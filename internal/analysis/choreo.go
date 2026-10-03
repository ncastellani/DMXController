package analysis

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"luzes/internal/show"
)

// Tetos de segurança para os acentos gerados (alinhados com limits do console).
const (
	maxAccentBeats = 4
	minAccentGap   = 20.0 // segundos entre acentos
)

// sectionChoice é o preset escolhido para uma parte.
type sectionChoice struct {
	Index  int    `json:"indice"`
	Name   string `json:"nome"`
	Preset string `json:"preset"`
	Lyric  string `json:"letra,omitempty"` // trecho da letra que cai na parte
	Reason string `json:"motivo"`
}

// colorChoice é a luz de uma cor citada na letra: uma cena de cor fixa da
// biblioteca ou uma cor nova. Com a versão cantada, ID é a cor citada (o
// instante vem da transcrição); só com a letra colada, Part é a parte.
type colorChoice struct {
	ID     string    `json:"id,omitempty"`
	Part   *int      `json:"parte,omitempty"`
	Word   string    `json:"palavra,omitempty"`
	Preset string    `json:"preset,omitempty"`
	Color  *newColor `json:"cor,omitempty"`
	Reason string    `json:"motivo,omitempty"`
}

// colorCue é uma cor citada já no tempo: acende em T (a palavra, ou o início
// da parte) e fica alguns compassos, até perto de Until (fim da frase; 0 =
// sem frase).
type colorCue struct {
	T, Until float64
	Preset   string // "qlc:NN" ou "cor:#rrggbb"
	Name     string
	Note     string
}

// Quanto dura a cor citada: até o fim da frase, entre 2 e 4 compassos; sem a
// versão cantada (não se sabe a frase), 4.
const (
	colorMinBars = 2
	colorMaxBars = 4
)

// newColor é uma cor criada para a música ("LILÁS", "#c8a2c8").
type newColor struct {
	Name string `json:"nome"`
	RGB  string `json:"rgb"`
}

// accent é um flash curto num momento de impacto.
type accent struct {
	Time   float64 `json:"tempo"`
	Preset string  `json:"preset"`
	Beats  float64 `json:"duracao_batidas"`
	Reason string  `json:"motivo"`
}

// choices é o que o LLM (ou as regras) decide.
type choices struct {
	Genre     string          `json:"genero"`
	Mood      string          `json:"clima"`
	Desc      string          `json:"descricao"`
	BPM       float64         `json:"bpm_percebido"`
	Intensity *float64        `json:"intensidade"`
	Palette   []string        `json:"paleta"`
	Sections  []sectionChoice `json:"secoes"`
	Accents   []accent        `json:"acentos"`
	Colors    []colorChoice   `json:"cores_citadas"`
}

// ruleChoices decide sem LLM: a agitação da sequência acompanha a energia
// da parte; partes repetidas (mesmo grupo) repetem o preset, mas partes
// vizinhas nunca ficam com o mesmo. Com temperatura (quente/fria), prefere
// os presets dela e só usa mistos/neutros quando faltam opções.
func ruleChoices(a *show.Analysis, cat []Preset, temp string) choices {
	type ranked struct{ exact, other []Preset }
	tiers := map[string]*ranked{}
	for _, t := range []string{"calma", "media", "agitada"} {
		tiers[t] = &ranked{}
	}
	for _, p := range cat {
		if p.Kind != "sequencia" || p.Layer != "base" || !p.Fits(temp) || tiers[p.Agitacao] == nil {
			continue
		}
		r := tiers[p.Agitacao]
		if temp == TempWarm || temp == TempCold {
			if p.Temp != temp {
				r.other = append(r.other, p)
				continue
			}
		}
		r.exact = append(r.exact, p)
	}
	secs := a.Sections
	var energies []float64
	for _, s := range secs {
		energies = append(energies, s.Energy)
	}
	slices.Sort(energies)
	lo, hi := energies[len(energies)/3], energies[2*len(energies)/3]

	var out choices
	byGroup := map[string]string{}
	used := map[string]bool{}
	prev := ""
	for i, s := range secs {
		pid, ok := byGroup[s.Group]
		if !ok || pid == prev {
			tier := "agitada"
			switch {
			case s.Energy < lo:
				tier = "calma"
			case s.Energy < hi:
				tier = "media"
			}
			// Ordem de preferência: da temperatura e ainda não usado; misto
			// ainda não usado; qualquer um diferente do anterior.
			r := tiers[tier]
			filter := func(list []Preset, fresh bool) (pool []Preset) {
				for _, p := range list {
					if p.ID != prev && (!fresh || !used[p.ID]) {
						pool = append(pool, p)
					}
				}
				return pool
			}
			var pool []Preset
			for _, try := range [][]Preset{filter(r.exact, true), filter(r.other, true), filter(append(slices.Clone(r.exact), r.other...), false)} {
				if len(try) > 0 {
					pool = try
					break
				}
			}
			if len(pool) > 0 {
				pid = pool[(i*7)%len(pool)].ID // determinístico, variado
			}
			if _, ok := byGroup[s.Group]; !ok {
				byGroup[s.Group] = pid
			}
			used[pid] = true
		}
		prev = pid
		name := "parte " + s.Group
		switch i {
		case 0:
			name = "intro"
		case len(secs) - 1:
			name = "final"
		}
		out.Sections = append(out.Sections, sectionChoice{Index: i, Name: name, Preset: pid, Reason: "regra: energia da parte"})
	}

	// Acento: subida forte de energia → fritação branca (não a mais agressiva).
	flash := ""
	for _, p := range cat {
		if p.Layer == "flash" && p.Fits(temp) && (flash == "" || p.Name == "FRITAÇÃO | BRANCO") {
			flash = p.ID
		}
	}
	for i := 1; i < len(secs) && flash != ""; i++ {
		if secs[i].Energy-secs[i-1].Energy >= 0.25 {
			out.Accents = append(out.Accents, accent{Time: secs[i].Start, Preset: flash, Beats: 1, Reason: "regra: subida de energia"})
		}
	}
	return out
}

var hexColor = regexp.MustCompile(`^#?([0-9a-fA-F]{6})$`)

// cleanColor confere uma cor nova do LLM: nome curto em maiúsculas e RGB
// "#rrggbb" aceso o bastante para aparecer no palco.
func cleanColor(c *newColor) (*newColor, bool) {
	m := hexColor.FindStringSubmatch(strings.TrimSpace(c.RGB))
	name := strings.ToUpper(strings.TrimSpace(c.Name))
	if m == nil || name == "" {
		return nil, false
	}
	var r, g, b int
	fmt.Sscanf(m[1], "%02x%02x%02x", &r, &g, &b)
	if max(r, g, b) < 60 {
		return nil, false
	}
	return &newColor{Name: trunc(name, 30), RGB: "#" + strings.ToLower(m[1])}, true
}

// validate mantém só as escolhas válidas do LLM; o que faltar vem das regras.
// As partes ficam com presets da camada base na temperatura pedida. Acentos
// são encaixados na batida mais próxima, espaçados e limitados.
func validate(llm *choices, a *show.Analysis, cat []Preset, fallback choices, temp string) ([]sectionChoice, []accent) {
	byID := map[string]Preset{}
	for _, p := range cat {
		byID[p.ID] = p
	}
	secs := slices.Clone(fallback.Sections)
	accents := fallback.Accents
	if llm != nil {
		for _, c := range llm.Sections {
			if c.Index < 0 || c.Index >= len(secs) {
				continue
			}
			p, ok := byID[c.Preset]
			if !ok || p.Layer != "base" || !p.Fits(temp) {
				continue
			}
			name := c.Name
			if name == "" {
				name = secs[c.Index].Name
			}
			reason := trunc(c.Reason, 200)
			if l := strings.Join(strings.Fields(c.Lyric), " "); l != "" {
				reason += " · “" + trunc(l, 80) + "”"
			}
			secs[c.Index] = sectionChoice{Index: c.Index, Name: trunc(name, 40), Preset: c.Preset, Reason: reason}
		}
		accents = llm.Accents
	}
	slices.SortFunc(accents, func(x, y accent) int { return cmpF(x.Time, y.Time) })
	var out []accent
	for _, ac := range accents {
		if p, ok := byID[ac.Preset]; !ok || p.Layer != "flash" || !p.Fits(temp) || len(a.Beats) == 0 {
			continue
		}
		t := nearest(a.Beats, ac.Time)
		if len(out) > 0 && t-out[len(out)-1].Time < minAccentGap {
			continue
		}
		ac.Time, ac.Beats = t, math.Max(1, math.Min(maxAccentBeats, math.Round(ac.Beats)))
		ac.Reason = trunc(ac.Reason, 200)
		out = append(out, ac)
	}
	return secs, out
}

// validateColors põe no tempo as cores citadas que o LLM escolheu: com a
// versão cantada, no instante da palavra (s); só com a letra colada, no
// início da parte. Valem as cenas de cor fixa (de qualquer temperatura: a cor
// citada vale mais que a temperatura) e as cores novas.
func validateColors(llm *choices, a *show.Analysis, cat []Preset, s *sung) []colorCue {
	if llm == nil {
		return nil
	}
	byID := map[string]Preset{}
	for _, p := range cat {
		byID[p.ID] = p
	}
	mentions := map[string]mention{}
	if s != nil {
		for _, m := range s.Colors {
			mentions[m.ID] = m
		}
	}
	var out []colorCue
	for _, c := range llm.Colors {
		cc := colorCue{}
		word := c.Word
		switch {
		case s != nil:
			m, ok := mentions[c.ID]
			if !ok {
				continue
			}
			delete(mentions, c.ID) // uma luz por cor citada
			line := s.Lines[m.Line]
			cc.T, cc.Until, word = m.Time, line.End, m.Word
			cc.Note = "letra: “" + trunc(line.Text, 80) + "”"
		case c.Part != nil && *c.Part >= 0 && *c.Part < len(a.Sections):
			cc.T = a.Sections[*c.Part].Start
			cc.Note = "letra"
		default:
			continue
		}
		if p, ok := byID[c.Preset]; ok && p.Layer == "base" && p.Kind == "cena" && p.Agitacao == "fixa" {
			cc.Preset, cc.Name = p.ID, p.Name
		} else if col, ok := cleanColorPtr(c.Color); ok {
			cc.Preset, cc.Name = show.ColorPrefix+col.RGB, "COR | "+col.Name
		} else {
			continue
		}
		if w := strings.TrimSpace(word); w != "" && s == nil {
			cc.Note += ": “" + trunc(w, 30) + "”"
		}
		if r := strings.TrimSpace(c.Reason); r != "" {
			cc.Note += " · " + trunc(r, 120)
		}
		out = append(out, cc)
	}
	slices.SortFunc(out, func(x, y colorCue) int { return cmpF(x.T, y.T) })
	return out
}

// nearest devolve o valor de xs (não vazio) mais perto de t.
func nearest(xs []float64, t float64) float64 {
	best := xs[0]
	for _, x := range xs {
		if math.Abs(x-t) < math.Abs(best-t) {
			best = x
		}
	}
	return best
}

func cleanColorPtr(c *newColor) (*newColor, bool) {
	if c == nil {
		return nil, false
	}
	return cleanColor(c)
}

// lockRate devolve o fator de velocidade para os passos da sequência caírem
// na batida (a subdivisão mais próxima: 1/2, 1, 2 ou 4 batidas).
func lockRate(p Preset, bpm float64) float64 {
	if p.Kind != "sequencia" || p.Fade || bpm <= 0 {
		return 1
	}
	beat := 60000 / bpm
	best := beat
	for _, m := range []float64{0.5, 1, 2, 4} {
		if math.Abs(math.Log(p.StepMs/(beat*m))) < math.Abs(math.Log(p.StepMs/best)) {
			best = beat * m
		}
	}
	return round(math.Max(0.5, math.Min(2, p.StepMs/best)), 4)
}

// numberSections enumera as partes que se repetem ("refrão 1", "refrão 2"...),
// na ordem da música. Partes vizinhas com o mesmo nome são uma só (a
// análise às vezes divide um verso em dois trechos) e levam o mesmo número;
// nomes que aparecem uma vez só ("intro", "final") ficam sem número. Um
// número que a IA já tenha posto é refeito.
func numberSections(secs []show.Section) {
	base := make([]string, len(secs))
	runs := map[string]int{}
	for i, s := range secs {
		base[i] = strings.TrimSpace(strings.TrimRightFunc(strings.TrimSpace(s.Name), func(r rune) bool { return r >= '0' && r <= '9' }))
		if base[i] != "" && (i == 0 || !strings.EqualFold(base[i], base[i-1])) {
			runs[strings.ToLower(base[i])]++
		}
	}
	seen := map[string]int{}
	for i := range secs {
		key := strings.ToLower(base[i])
		if base[i] == "" || runs[key] < 2 {
			continue
		}
		if i == 0 || !strings.EqualFold(base[i], base[i-1]) {
			seen[key]++
		}
		secs[i].Name = fmt.Sprintf("%s %d", base[i], seen[key])
	}
}

// placeColors encaixa as cores citadas entre as trocas das partes (base, em
// ordem). Cada cor acende na batida mais perto da palavra (no início da
// parte, se cair a menos de uma batida dele; a troca da parte sai) e fica até
// o fim da frase, de 2 a 4 compassos; depois volta o preset da parte. Uma
// cor a menos de 2 batidas da anterior fica de fora (troca rápida demais), e
// nada volta a acender depois de songEnd.
func placeColors(a *show.Analysis, base []show.Cue, colors []colorCue, songEnd float64) []show.Cue {
	if len(colors) == 0 || len(a.Beats) == 0 || a.BPM <= 0 {
		return base
	}
	beat := 60 / a.BPM
	bar := 4 * beat
	partAt := func(t float64) int { // troca de parte valendo em t
		i := -1
		for j, c := range base {
			if c.T <= t+1e-3 {
				i = j
			}
		}
		return i
	}
	type placed struct {
		t, end float64
		cue    show.Cue
	}
	var ps []placed
	for _, c := range colors {
		t := nearest(a.Beats, c.T)
		for _, s := range a.Sections {
			if math.Abs(t-s.Start) < beat*1.01 {
				t = s.Start
				break
			}
		}
		if len(ps) > 0 && t-ps[len(ps)-1].t < 2*beat || t >= songEnd {
			continue
		}
		bars := float64(colorMaxBars)
		if c.Until > 0 {
			bars = math.Max(colorMinBars, math.Min(colorMaxBars, math.Ceil((c.Until-t)/bar-0.05)))
		}
		ps = append(ps, placed{t, nearest(a.Beats, t+bars*bar), show.Cue{T: round(t, 3), Layer: "base", Kind: "preset", Preset: c.Preset, Name: c.Name, Note: c.Note}})
	}
	var out []show.Cue
	for _, c := range base {
		if !slices.ContainsFunc(ps, func(p placed) bool { return math.Abs(p.t-c.T) < 1e-3 }) {
			out = append(out, c)
		}
	}
	for k, p := range ps {
		out = append(out, p.cue)
		if k+1 < len(ps) && ps[k+1].t <= p.end || p.end >= songEnd-beat {
			continue // a próxima cor ou o fim da música vêm antes
		}
		j := partAt(p.end)
		if j < 0 || base[j].T > p.t+1e-3 {
			continue // a parte seguinte já tira a cor
		}
		back := base[j]
		back.T, back.Note = round(p.end, 3), "volta depois da cor"
		out = append(out, back)
	}
	return out
}

// build monta a timeline final.
func build(title, file string, a show.Analysis, cat []Preset, secs []sectionChoice, accents []accent, colors []colorCue, sg *sung, llm *choices, model string, opt Options, extra map[string]any) *show.Timeline {
	byID := map[string]Preset{}
	for _, p := range cat {
		byID[p.ID] = p
	}
	beat := 60 / a.BPM
	t := &show.Timeline{Version: 1, Analysis: a}
	t.Song.Title, t.Song.File, t.Song.Duration = title, file, a.Duration

	for i, c := range secs {
		t.Analysis.Sections[i].Name = c.Name
	}
	numberSections(t.Analysis.Sections)
	var base []show.Cue
	for i, c := range secs {
		if c.Preset == "" {
			continue
		}
		p := byID[c.Preset]
		base = append(base, show.Cue{T: t.Analysis.Sections[i].Start, Layer: "base", Kind: "preset", Preset: p.ID, Name: p.Name, Rate: lockRate(p, a.BPM), Note: c.Reason})
	}
	// Apaga no fim do som (quando a energia some).
	end := a.Duration
	for i := len(a.Energy.Values) - 1; i >= 0; i-- {
		if a.Energy.Values[i] > 0.02 {
			end = math.Min(end, float64(i+1)*a.Energy.Hop)
			break
		}
	}
	t.Cues = placeColors(&t.Analysis, base, colors, end)
	for _, ac := range accents {
		p := byID[ac.Preset]
		t.Cues = append(t.Cues, show.Cue{T: round(ac.Time, 3), Layer: "flash", Kind: "preset", Preset: p.ID, Name: p.Name, Duration: round(ac.Beats*beat, 3), Note: ac.Reason})
	}
	t.Cues = append(t.Cues, show.Cue{T: round(end, 3), Layer: "base", Kind: "blackout", Note: "fim"})
	if sg != nil {
		t.Lyrics = sg.Lines
	}
	show.SortCues(t.Cues)

	intensity := 0.0
	for _, s := range a.Sections {
		intensity += s.Energy / float64(len(a.Sections))
	}
	palette := []string{}
	if llm != nil {
		if llm.Intensity != nil {
			intensity = math.Max(0, math.Min(1, *llm.Intensity))
		}
		if llm.Palette != nil {
			palette = llm.Palette
		}
		t.AI = &struct {
			Genero    string  `json:"genero"`
			Clima     string  `json:"clima"`
			Descricao string  `json:"descricao"`
			BPM       float64 `json:"bpm_percebido"`
		}{llm.Genre, llm.Mood, llm.Desc, llm.BPM}
	}
	t.Personality, _ = json.Marshal(map[string]any{"mode": "presets", "intensity": round(intensity, 2), "palette": palette})
	gen := map[string]any{"at": time.Now().UTC().Format(time.RFC3339), "by": "luzes", "temperature": opt.temp()}
	if model != "" {
		gen["model"] = model
	}
	if llm != nil && strings.TrimSpace(opt.Lyrics) != "" {
		gen["lyrics"] = true // a IA usou a letra colada
	}
	maps.Copy(gen, extra)
	t.Generated, _ = json.Marshal(gen)
	return t
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// describe resume o catálogo e as partes para o prompt.
func describe(a *show.Analysis, cat []Preset) (secs, presets string) {
	for i, s := range a.Sections {
		secs += fmt.Sprintf("  %d: %.1fs–%.1fs, %d compassos, energia %.2f, grupo %s\n", i, s.Start, s.End, s.Bars, s.Energy, s.Group)
	}
	for _, p := range cat {
		line := fmt.Sprintf("  %s | %s | %s | camada %s | %s", p.ID, p.Caption, p.Group, p.Layer, p.Kind)
		if p.Agitacao != "" {
			line += " | agitação " + p.Agitacao
		}
		if p.Kind == "sequencia" {
			line += fmt.Sprintf(" | passo ~%.0f ms", p.StepMs)
		}
		line += " | cores: " + joinOr(p.Colors, "-") + " | temperatura " + p.Temp
		if p.Hex != "" {
			line += " | rgb " + p.Hex
		}
		presets += line + "\n"
	}
	return secs, presets
}

func joinOr(s []string, empty string) string {
	if len(s) == 0 {
		return empty
	}
	out := s[0]
	for _, x := range s[1:] {
		out += ", " + x
	}
	return out
}

// RateFor devolve o encaixe na batida (rate) do preset id, ou 1 se não for
// uma sequência do catálogo.
func RateFor(cat []Preset, id string, bpm float64) float64 {
	for _, p := range cat {
		if p.ID == id {
			return lockRate(p, bpm)
		}
	}
	return 1
}
