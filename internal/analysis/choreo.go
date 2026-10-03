package analysis

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/ncastellani/DMXController/internal/library"
	"github.com/ncastellani/DMXController/internal/show"
)

// Tetos de segurança para os acentos gerados (alinhados com limits do console).
const (
	maxAccentBeats = 4
	minAccentGap   = 20.0 // segundos entre acentos
)

// sectionChoice é a luz escolhida para uma parte: um preset ou, no modo
// livre, um efeito criado para a música.
type sectionChoice struct {
	Index  int        `json:"indice"`
	Name   string     `json:"nome"`
	Preset string     `json:"preset"`
	Free   *llmEffect `json:"efeito,omitempty"` // como o LLM descreve o efeito
	Reason string     `json:"motivo"`

	Effect *show.Effect `json:"-"` // efeito já conferido (vale no lugar do preset)
}

// llmEffect é um efeito do modo livre na resposta do LLM.
type llmEffect struct {
	Model  string     `json:"modelo"` // modo de pista da biblioteca ("pista:club") usado como efeito; "cores" troca a paleta dele
	Type   string     `json:"tipo"`
	Colors []string   `json:"cores"`
	Steps  [][]string `json:"passos"` // sequence
	Fade   bool       `json:"fade"`
	Beats  float64    `json:"batidas"`
	Level  float64    `json:"intensidade"`
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
	Score     *Mood           `json:"clima_pontos"` // clima da música, na escala dos efeitos da biblioteca
	Sections  []sectionChoice `json:"secoes"`
	Accents   []accent        `json:"acentos"`
}

// ruleChoices decide sem LLM: a agitação da sequência acompanha a energia
// e a batida da parte (drive); partes repetidas (mesmo grupo) repetem o preset, mas partes
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
		energies = append(energies, drive(s))
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
			case drive(s) < lo:
				tier = "calma"
			case drive(s) < hi:
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

// Paletas (4 cores) das sequências geradas pelas regras, por temperatura.
var rulePalettes = map[string][][4]string{
	TempWarm: {{"#ff0000", "#ff6400", "#ff0a2d", "#ffb400"}, {"#ff3700", "#ffb400", "#ff0000", "#ff5000"}, {"#ff0a2d", "#ff5000", "#b4000e", "#ff6400"}},
	TempCold: {{"#0000ff", "#00ffff", "#7800ff", "#00ff78"}, {"#7800ff", "#0064ff", "#00ffff", "#5a00c8"}, {"#00ff78", "#00ffff", "#0000ff", "#8cff00"}},
	TempAuto: {{"#ff0064", "#00ffff", "#7800ff", "#0000ff"}, {"#ff3700", "#ffb400", "#ff0a2d", "#ff00aa"}, {"#7800ff", "#00ffff", "#ff2878", "#0064ff"}, {"#ff0000", "#0000ff", "#ff00ff", "#00ffff"}},
}

// ruleSequence monta uma sequência com a paleta p, em "frases" como as dos
// presets de pista (rodízio de três cores, dois tons alternando, um canhão por
// vez, todos iguais, um toque de branco). tier: 0 calma (lenta, em fade),
// 1 média, 2 agitada.
func ruleSequence(p [4]string, tier int) *show.Effect {
	const off, white = "#000000", "#ffffff"
	a, b, c, d := p[0], p[1], p[2], p[3]
	rot := func(x, y, z string) [][]string { return [][]string{{x, y, z}, {z, x, y}, {y, z, x}} }
	alt := func(x, y string) [][]string { return [][]string{{x, y, x}, {y, x, y}} }
	switch tier {
	case 0: // um canhão muda de cada vez, como o CHILL
		return &show.Effect{Type: show.FXSequence, Beats: 4, Fade: true, Level: 0.6,
			Steps: [][]string{{a, b, c}, {a, b, d}, {c, b, d}, {c, a, d}, {c, a, b}, {d, a, b}}}
	case 1:
		return &show.Effect{Type: show.FXSequence, Beats: 2,
			Steps: slices.Concat(rot(a, b, c), alt(a, d), rot(b, d, c), alt(c, a))}
	}
	return &show.Effect{Type: show.FXSequence, Beats: 1, Steps: slices.Concat(
		rot(a, b, c), alt(d, a),
		[][]string{{b, off, off}, {off, b, off}, {off, off, b}},
		[][]string{{c, c, c}, {d, d, d}},
		rot(d, c, a), alt(b, c),
		[][]string{{white, white, white}},
	)}
}

// ruleEffects troca os presets das regras por sequências do modo livre: cada
// grupo de partes ganha uma paleta e o ritmo acompanha a energia. Com a
// biblioteca, cada grupo (em cada faixa de energia) fica com o efeito dela de
// clima mais parecido (o da música, song, quando a IA o avaliou), sem repetir
// enquanto houver opções; sem um que sirva, a sequência é montada aqui.
func ruleEffects(a *show.Analysis, temp string, secs []sectionChoice, lib *library.Library, song *Mood) {
	pals := rulePalettes[temp]
	picked := map[string]*show.Effect{}
	used := map[string]bool{}
	family := "" // temperatura do primeiro efeito escolhido
	var energies []float64
	for _, s := range a.Sections {
		energies = append(energies, drive(s))
	}
	slices.Sort(energies)
	lo, hi := energies[len(energies)/3], energies[2*len(energies)/3]
	groups := map[string]int{}
	for i, s := range a.Sections {
		g, ok := groups[s.Group]
		if !ok {
			g = len(groups)
			groups[s.Group] = g
		}
		tier := 2
		switch {
		case drive(s) < lo:
			tier = 0
		case drive(s) < hi:
			tier = 1
		}
		key := fmt.Sprintf("%s/%d", s.Group, tier)
		e := picked[key]
		if e == nil && lib != nil {
			if le := pickEffect(lib, []float64{0.2, 0.5, 0.85}[tier], song, temp, family, used); le != nil {
				e, _ = LibraryEffect(le, a.BPM, nil, temp)
				used[le.ID] = true
				if family == "" {
					family = le.Temp
				}
			}
		}
		if e == nil {
			e = ruleSequence(pals[g%len(pals)], tier)
			e.Sync = show.SyncHits // troca a cada toque da música
			soften(e)
		}
		picked[key] = e
		secs[i].Preset, secs[i].Effect = "", e
	}
}

// groupsByEnergy devolve os grupos de partes do mais agitado ao mais calmo
// (volume e batida médios das partes do grupo).
func groupsByEnergy(a *show.Analysis) []string {
	sum, n := map[string]float64{}, map[string]float64{}
	var groups []string
	for _, s := range a.Sections {
		if n[s.Group] == 0 {
			groups = append(groups, s.Group)
		}
		sum[s.Group] += drive(s)
		n[s.Group]++
	}
	slices.SortStableFunc(groups, func(x, y string) int { return cmpF(sum[y]/n[y], sum[x]/n[x]) })
	return groups
}

// mixByEnergy monta o modo misto das regras: a metade mais agitada dos grupos
// de partes fica com os presets (os modos de pista tocam com os tempos e as
// rajadas deles) e a outra com os efeitos que já estão em secs.
func mixByEnergy(a *show.Analysis, secs, presets []sectionChoice) {
	groups := groupsByEnergy(a)
	top := map[string]bool{}
	for _, g := range groups[:len(groups)/2] {
		top[g] = true
	}
	for i, s := range a.Sections {
		if top[s.Group] {
			secs[i] = presets[i]
		}
	}
}

// balance garante a combinação do modo misto nas escolhas já validadas:
// presets e efeitos em pelo menos um terço das partes cada. Se faltar de um
// tipo, grupos inteiros de partes passam para ele (os mais agitados viram
// presets, os mais calmos viram efeitos), sem zerar o outro.
func balance(a *show.Analysis, secs, presets, effects []sectionChoice) {
	n := len(secs)
	if n < 2 {
		return
	}
	need := (n + 2) / 3
	count := func() (fx int) {
		for _, s := range secs {
			if s.Effect != nil {
				fx++
			}
		}
		return fx
	}
	move := func(groups []string, toEffect bool, from []sectionChoice, why string) {
		for _, g := range groups {
			fx := count()
			if toEffect && fx >= need || !toEffect && n-fx >= need {
				return
			}
			var idx []int
			for i, s := range a.Sections {
				if s.Group == g && (secs[i].Effect == nil) == toEffect {
					idx = append(idx, i)
				}
			}
			if left := map[bool]int{true: n - fx, false: fx}[toEffect] - len(idx); len(idx) == 0 || left < 1 {
				continue // o grupo já é desse tipo, ou levá-lo zeraria o outro
			}
			for _, i := range idx {
				name := secs[i].Name
				secs[i] = from[i]
				secs[i].Name, secs[i].Reason = name, why
			}
		}
	}
	groups := groupsByEnergy(a)
	move(groups, false, presets, "misto: preset na parte agitada, para combinar com os efeitos")
	slices.Reverse(groups)
	move(groups, true, effects, "misto: efeito na parte calma, para combinar com os presets")
}

// colorTempOf classifica uma cor pelo matiz: quente (vermelho a amarelo),
// fria (verde a violeta) ou "" (rosa, magenta, branco e tons pouco saturados).
func colorTempOf(r, g, b int) string {
	hi, lo := max(r, g, b), min(r, g, b)
	if hi == 0 || float64(hi-lo)/float64(hi) < 0.25 {
		return ""
	}
	var h float64
	d := float64(hi - lo)
	switch hi {
	case r:
		h = math.Mod(float64(g-b)/d+6, 6)
	case g:
		h = float64(b-r)/d + 2
	default:
		h = float64(r-g)/d + 4
	}
	switch h *= 60; {
	case h < 70 || h >= 345:
		return TempWarm
	case h >= 80 && h < 290:
		return TempCold
	}
	return ""
}

// cleanEffect confere um efeito do LLM (que passa a trocar de passo nos
// toques da música): tipo e cores válidos, cores acesas o
// bastante para aparecer no palco e dentro da temperatura pedida. Uma
// sequência que é quase toda apagada ou um pisca-pisca é recusada; nas
// outras, os extremos (branco total, apagado, tons escuros) saem (soften).
func cleanEffect(f *llmEffect, temp string) (*show.Effect, bool) {
	e := &show.Effect{Type: strings.ToLower(strings.TrimSpace(f.Type)), Colors: slices.Clone(f.Colors), Beats: f.Beats, Level: f.Level, Fade: f.Fade}
	for _, st := range f.Steps {
		e.Steps = append(e.Steps, slices.Clone(st))
	}
	if e.Clean() != nil {
		return nil, false
	}
	bright := func(c string) (ok, inTemp bool) {
		var r, g, b int
		fmt.Sscanf(c, "#%02x%02x%02x", &r, &g, &b)
		t := colorTempOf(r, g, b)
		return max(r, g, b) >= 60, temp != TempWarm && temp != TempCold || t == "" || t == temp
	}
	for _, c := range e.Colors {
		if ok, inTemp := bright(c); !ok || !inTemp {
			return nil, false
		}
	}
	lit, dark := 0, 0
	for _, st := range e.Steps {
		n := 0
		for _, c := range st {
			ok, inTemp := bright(c)
			if ok && !inTemp {
				return nil, false
			}
			if ok {
				n++
			}
		}
		lit += n
		if n == 0 {
			dark++
		}
	}
	if len(e.Steps) > 0 && (2*lit < 3*len(e.Steps) || 4*dark > len(e.Steps)) {
		return nil, false
	}
	if e.Level > 0 && e.Level < 0.4 {
		e.Level = 0.4
	}
	if e.Type != show.FXFixed {
		e.Sync = show.SyncHits // no modo livre, o passo troca a cada toque da música
	}
	if !soften(e) {
		return nil, false
	}
	return e, true
}

// minBright é o brilho mínimo (maior canal, de 255) de uma cor num efeito
// gerado: abaixo disso o canhão fica quase apagado e some no palco.
const minBright = 128

// soften tira os extremos de um efeito gerado (pedido da usuária: branco
// total e escuro "ficam sem graça no show"; trocando a cada toque, um passo
// desses dura perto de meio segundo, não é o flash rápido do preset
// original). Saem os passos com os 3 canhões brancos e os com os 3 apagados
// ou quase; um tom escuro sobe até minBright, mantendo a cor; e um canhão
// apagado num passo aceso fica com a cor mais forte do passo em minBright (a
// corrida de um canhão por vez continua aparecendo, sobre um fundo da mesma
// cor). Devolve false se não sobra uma sequência.
func soften(e *show.Effect) bool {
	type rgb struct{ r, g, b int }
	parse := func(s string) (c rgb) {
		fmt.Sscanf(s, "#%02x%02x%02x", &c.r, &c.g, &c.b)
		return c
	}
	top := func(c rgb) int { return max(c.r, c.g, c.b) }
	lift := func(c rgb) string { // a mesma cor, com o maior canal em minBright
		k := float64(minBright) / float64(top(c))
		return fmt.Sprintf("#%02x%02x%02x", int(float64(c.r)*k+0.5), int(float64(c.g)*k+0.5), int(float64(c.b)*k+0.5))
	}
	const dark = 40 // abaixo disso não dá para dizer que cor é: conta como apagado
	for i, s := range e.Colors {
		if c := parse(s); top(c) >= dark && top(c) < minBright {
			e.Colors[i] = lift(c)
		}
	}
	if e.Type != show.FXSequence {
		return true
	}
	// O branco é dos picos (o realce no canhão do centro): um branco solto
	// numa sequência colorida viraria um flash sem pico nenhum. Só fica nas
	// sequências em que ele é uma das cores do desenho (um quarto dos tons ou
	// mais, como VERMELHO BRANCO).
	whites, tones := 0, 0
	for _, st := range e.Steps {
		for _, s := range st {
			tones++
			if parse(s) == (rgb{255, 255, 255}) {
				whites++
			}
		}
	}
	noWhite := 4*whites < tones
	var steps [][]string
	for _, st := range e.Steps {
		if noWhite {
			color := rgb{}
			for _, s := range st {
				if c := parse(s); c != (rgb{255, 255, 255}) && top(c) > top(color) {
					color = c
				}
			}
			st = slices.Clone(st)
			for i, s := range st {
				if parse(s) == (rgb{255, 255, 255}) && top(color) >= dark {
					st[i] = fmt.Sprintf("#%02x%02x%02x", color.r, color.g, color.b)
				}
			}
		}
		white, best := 0, rgb{}
		for _, s := range st {
			c := parse(s)
			if c == (rgb{255, 255, 255}) {
				white++
			}
			if top(c) > top(best) {
				best = c
			}
		}
		if white == len(st) || top(best) < dark {
			continue
		}
		out := make([]string, len(st))
		for i, s := range st {
			switch c := parse(s); {
			case top(c) < dark:
				out[i] = lift(best)
			case top(c) < minBright:
				out[i] = lift(c)
			default:
				out[i] = s
			}
		}
		steps = append(steps, out)
	}
	e.Steps = steps
	return len(steps) >= 2
}

// validate mantém só as escolhas válidas do LLM; o que faltar vem das regras.
// As partes ficam com presets da camada base na temperatura pedida. Acentos
// são encaixados na batida mais próxima, espaçados e limitados. No modo livre
// (free), a parte pode vir com um efeito no lugar do preset.
func validate(llm *choices, a *show.Analysis, cat []Preset, fallback choices, temp string, free bool, lib *library.Library) ([]sectionChoice, []accent) {
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
			var fx *show.Effect
			if free && c.Free != nil && c.Free.Model != "" { // modo de pista da biblioteca como efeito, com as cores dele ou novas
				if le := lib.Effect(c.Free.Model); le != nil {
					fx, _ = LibraryEffect(le, a.BPM, c.Free.Colors, temp)
				}
			} else if free && c.Free != nil {
				fx, _ = cleanEffect(c.Free, temp)
			}
			if p, ok := byID[c.Preset]; fx != nil {
				c.Preset = ""
			} else if !ok || p.Layer != "base" || !p.Fits(temp) {
				continue
			}
			name := c.Name
			if name == "" {
				name = secs[c.Index].Name
			}
			secs[c.Index] = sectionChoice{Index: c.Index, Name: trunc(name, 40), Preset: c.Preset, Effect: fx, Reason: trunc(c.Reason, 200)}
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

// build monta a timeline final.
func build(title, file string, a show.Analysis, cat []Preset, secs []sectionChoice, accents []accent, llm *choices, model string, opt Options) *show.Timeline {
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
	bases, effects := 0, 0
	for i, c := range secs {
		if c.Effect != nil {
			effects++
			bases++
			t.Cues = append(t.Cues, show.Cue{T: t.Analysis.Sections[i].Start, Layer: "base", Kind: "effect", Effect: c.Effect, Name: show.EffectName(c.Effect), Note: c.Reason})
			continue
		}
		if c.Preset == "" {
			continue
		}
		p := byID[c.Preset]
		bases++
		t.Cues = append(t.Cues, show.Cue{T: t.Analysis.Sections[i].Start, Layer: "base", Kind: "preset", Preset: p.ID, Name: p.Name, Rate: lockRate(p, a.BPM), Note: c.Reason})
	}
	// A luz vai até o som acabar, apagando aos poucos na cauda final (pedido
	// da usuária: "luz até o fim enquanto ainda houver som"). Sem essa medida
	// (análise antiga), apaga quando a energia some.
	end, fade := a.SoundEnd, a.FadeOut
	if end <= 0 {
		end = a.Duration
		for i := len(a.Energy.Values) - 1; i >= 0; i-- {
			if a.Energy.Values[i] > 0.02 {
				end = math.Min(end, float64(i+1)*a.Energy.Hop)
				break
			}
		}
	}
	if fade < 1 { // fim seco: apaga de uma vez
		fade = 0
	}
	for _, ac := range accents {
		p := byID[ac.Preset]
		t.Cues = append(t.Cues, show.Cue{T: round(ac.Time, 3), Layer: "flash", Kind: "preset", Preset: p.ID, Name: p.Name, Duration: round(ac.Beats*beat, 3), Note: ac.Reason})
	}
	t.Cues = append(t.Cues, show.Cue{T: round(end, 3), Layer: "base", Kind: "blackout", Fade: round(fade, 1), Note: "fim"})
	show.SortCues(t.Cues)

	intensity := 0.0
	for _, s := range a.Sections {
		intensity += s.Energy / float64(len(a.Sections))
	}
	palette := []string{}
	var mood *Mood
	if llm != nil {
		if llm.Intensity != nil {
			intensity = math.Max(0, math.Min(1, *llm.Intensity))
		}
		if llm.Palette != nil {
			palette = llm.Palette
		}
		if llm.Score != nil {
			llm.Score.Clamp()
			mood = llm.Score
		}
		t.AI = &struct {
			Genero    string  `json:"genero"`
			Clima     string  `json:"clima"`
			Descricao string  `json:"descricao"`
			BPM       float64 `json:"bpm_percebido"`
		}{llm.Genre, llm.Mood, llm.Desc, llm.BPM}
	}
	mode := "presets"
	switch {
	case effects == bases && effects > 0:
		mode = "freestyle"
	case effects > 0:
		mode = "misto"
	}
	pers := map[string]any{"mode": mode, "intensity": round(intensity, 2), "palette": palette}
	if mood != nil {
		pers["mood"] = map[string]float64{"energy": mood.Energy, "joy": mood.Joy, "tension": mood.Tension, "romance": mood.Romance}
	}
	t.Personality, _ = json.Marshal(pers)
	gen := map[string]any{"at": time.Now().UTC().Format(time.RFC3339), "by": "DMXController", "temperature": opt.temp()}
	if opt.Free || opt.Mixed {
		gen["free"] = true // efeitos criados para a música
	}
	if opt.Mixed {
		gen["mix"] = true // modo misto pedido
	}
	if a.Drums {
		gen["drums"] = true // a batida foi ouvida na bateria separada
	}
	if model != "" {
		gen["model"] = model
	}
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
		pulse := fmt.Sprintf("batida %.2f", s.Pulse)
		if a.Drums && s.Pulse < 0.05 {
			pulse = "sem bateria"
		}
		secs += fmt.Sprintf("  %d: %.1fs–%.1fs, %d compassos, energia %.2f, %s, grupo %s\n", i, s.Start, s.End, s.Bars, s.Energy, pulse, s.Group)
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
