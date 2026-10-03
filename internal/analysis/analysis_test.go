package analysis

import (
	"context"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/show"
)

func TestTempoAndBeatsOfClickTrack(t *testing.T) {
	for _, bpm := range []float64{100, 128} {
		y := Mono(audio.Clicks(bpm, 40*time.Second))
		a := Analyze(y)
		if math.Abs(a.BPM-bpm) > 0.5 {
			t.Errorf("%v BPM: estimou %.2f", bpm, a.BPM)
		}
		want := 40 * bpm / 60
		if n := float64(len(a.Beats)); math.Abs(n-want) > 4 {
			t.Errorf("%v BPM: %d batidas, esperava ~%.0f", bpm, len(a.Beats), want)
		}
		// Batidas caem nos cliques (a cada 60/bpm s, a partir de 0).
		period := 60 / bpm
		for _, b := range a.Beats[2 : len(a.Beats)-2] {
			if off := math.Abs(b/period - math.Round(b/period)); off*period > 0.05 {
				t.Fatalf("%v BPM: batida em %.3fs fora dos cliques", bpm, b)
			}
		}
	}
}

// song gera uma "música" a 22.050 Hz: acorde contínuo + bumbo a 120 BPM,
// com o trecho do meio mais forte e noutro acorde (como verso → refrão).
func song(parts []float64) []float64 {
	const partLen = 32 * SR
	y := make([]float64, 0, len(parts)*partLen)
	chords := [][]float64{{220, 277.2, 329.6}, {261.6, 329.6, 392}} // Lá maior, Dó maior
	for p, amp := range parts {
		chord := chords[0]
		if amp > 0.2 {
			chord = chords[1]
		}
		for i := 0; i < partLen; i++ {
			t := float64(i) / SR
			var v float64
			for _, f := range chord {
				v += math.Sin(2 * math.Pi * f * t)
			}
			v *= amp / 3
			if beat := math.Mod(float64(p*partLen+i)/SR, 0.5); beat < 0.04 {
				v += 0.6 * math.Sin(2*math.Pi*60*beat) * math.Exp(-beat*60)
			}
			y = append(y, v)
		}
	}
	return y
}

func TestSectionsFollowEnergy(t *testing.T) {
	a := Analyze(song([]float64{0.05, 0.5, 0.05}))
	if math.Abs(a.BPM-120) > 0.5 {
		t.Errorf("BPM %.2f", a.BPM)
	}
	var cuts []float64
	for _, s := range a.Sections[1:] {
		cuts = append(cuts, s.Start)
	}
	near := func(x float64) bool {
		for _, c := range cuts {
			if math.Abs(c-x) < 2.5 {
				return true
			}
		}
		return false
	}
	if !near(32) || !near(64) {
		t.Fatalf("limites %v, esperava perto de 32 s e 64 s", cuts)
	}
	var loudest show.Section
	for _, s := range a.Sections {
		if s.Energy > loudest.Energy {
			loudest = s
		}
	}
	if loudest.Start < 30 || loudest.End > 66 {
		t.Fatalf("parte mais forte %.1f–%.1f, esperava o meio", loudest.Start, loudest.End)
	}
	// Partes 1 e 3 são iguais: devem receber a mesma letra.
	first, last := a.Sections[0], a.Sections[len(a.Sections)-1]
	if first.Group != last.Group || first.Group == loudest.Group {
		t.Fatalf("grupos: %+v", a.Sections)
	}
}

// console carrega o console e a biblioteca do repositório.
func console(t *testing.T) *control.Console {
	t.Helper()
	c, err := control.LoadConsole("../../config/console.yaml", positions(t))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func catalog(t *testing.T) []Preset { return Catalog(console(t)) }

func positions(t *testing.T) []*fixture.Fixture {
	t.Helper()
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func TestCatalog(t *testing.T) {
	cat := catalog(t)
	if len(cat) != 55+17 { // 55 botões + 17 cores "TODOS | X" fora do console
		t.Fatalf("%d presets, quero 72", len(cat))
	}
	tiers := map[string]int{}
	for _, p := range cat {
		tiers[p.Agitacao]++
		switch p.ID {
		case "pista:flash": // PISTA | FLASH: passos de 100 ms piscando
			if p.Agitacao != "agitada" || p.Kind != "sequencia" {
				t.Errorf("PISTA | FLASH: %+v", p)
			}
		case "pista:chill": // PISTA | CHILL: fades lentos
			if p.Agitacao != "calma" || !p.Fade {
				t.Errorf("PISTA | CHILL: %+v", p)
			}
		case "fritacao:branco-maximo": // FRITAÇÃO | BRANCO MÁXIMO
			if p.Layer != "flash" || p.Agitacao != "strobo" {
				t.Errorf("FRITAÇÃO | BRANCO MÁXIMO: %+v", p)
			}
		case "todos:azul": // TODOS | AZUL
			if len(p.Colors) != 1 || p.Colors[0] != "AZUL" || p.Temp != TempCold || p.Hex != "#0000ff" {
				t.Errorf("TODOS | AZUL: %+v", p)
			}
		case "todos:verde-limao": // TODOS | VERDE LIMÃO: não é botão, mas está na biblioteca
			if !p.Library || p.Layer != "base" || p.Hex != "#8cff00" || p.Caption != "VERDE LIMÃO" {
				t.Errorf("VERDE LIMÃO: %+v", p)
			}
		case "pista:club": // PISTA | CLUB: o único frio agitado
			if p.Temp != TempCold || p.Agitacao != "agitada" {
				t.Errorf("CLUB: %+v", p)
			}
		case "pista:fogo": // PISTA | FOGO
			if p.Temp != TempWarm {
				t.Errorf("FOGO: %+v", p)
			}
		case "pista:festa": // PISTA | FESTA: de tudo um pouco
			if p.Temp != TempMixed {
				t.Errorf("FESTA: %+v", p)
			}
		case "qlc:2": // TODOS | BLACKOUT não é cor
			t.Errorf("blackout no catálogo: %+v", p)
		}
	}
	if tiers["calma"] == 0 || tiers["media"] == 0 || tiers["agitada"] == 0 {
		t.Fatalf("agitação: %v", tiers)
	}
}

func TestValidateAndBuild(t *testing.T) {
	cat := catalog(t)
	a := show.Analysis{Duration: 120, BPM: 120}
	for b := 0.5; b < 120; b += 0.5 {
		a.Beats = append(a.Beats, b)
	}
	a.Energy.Hop = 0.5
	for i := 0; i < 240; i++ {
		a.Energy.Values = append(a.Energy.Values, 0.5)
	}
	a.Sections = []show.Section{{Start: 0, End: 40, Energy: 0.1, Group: "A"}, {Start: 40, End: 80, Energy: 0.8, Group: "B"}, {Start: 80, End: 120, Energy: 0.4, Group: "C"}}
	fallback := ruleChoices(&a, cat, TempAuto)
	llm := &choices{
		Sections: []sectionChoice{
			{Index: 0, Name: "intro", Preset: "pista:chill"},
			{Index: 1, Name: "refrão", Preset: "fritacao:branco-maximo"}, // flash não serve de base: ignora
			{Index: 7, Name: "?", Preset: "pista:pop"},                   // parte inexistente: ignora
		},
		Accents: []accent{
			{Time: 40.2, Preset: "fritacao:branco", Beats: 9},  // encaixa em 40,0 e limita a 4 batidas
			{Time: 45, Preset: "fritacao:branco", Beats: 1},    // a menos de 20 s do anterior: descarta
			{Time: 80.1, Preset: "pista:pop", Beats: 1},        // não é flash: descarta
			{Time: 100.26, Preset: "fritacao:ciano", Beats: 1}, // ok, em 100,5? não: 100,0 ou 100,5 → mais perto 100,5
		},
	}
	secs, acc := validate(llm, &a, cat, fallback, TempAuto, false, nil)
	if secs[0].Preset != "pista:chill" || secs[1].Preset != fallback.Sections[1].Preset {
		t.Fatalf("partes: %+v", secs)
	}
	if len(acc) != 2 || acc[0].Time != 40 || acc[0].Beats != 4 || acc[1].Time != 100.5 {
		t.Fatalf("acentos: %+v", acc)
	}
	tl := build("teste", "teste.wav", a, cat, secs, acc, llm, "modelo", Options{})
	last := tl.Cues[len(tl.Cues)-1]
	if last.Kind != "blackout" || tl.Cues[0].Preset != "pista:chill" || tl.Analysis.Sections[0].Name != "intro" {
		t.Fatalf("timeline: %+v", tl.Cues)
	}
	for _, c := range tl.Cues {
		if c.Layer == "flash" && c.Duration > 2.0001 { // 4 batidas a 120 BPM
			t.Fatalf("flash longo demais: %+v", c)
		}
	}
	// A timeline gerada precisa compilar no player.
	if _, err := show.Compile(tl, show.NewPresets(console(t).Library, positions(t)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

// songAnalysis é uma música de 6 partes com energias variadas.
func songAnalysis() show.Analysis {
	a := show.Analysis{Duration: 240, BPM: 120}
	for b := 0.5; b < 240; b += 0.5 {
		a.Beats = append(a.Beats, b)
	}
	a.Energy.Hop = 0.5
	for i := 0; i < 480; i++ {
		a.Energy.Values = append(a.Energy.Values, 0.5)
	}
	for i, e := range []float64{0.1, 0.4, 0.9, 0.5, 0.95, 0.2} {
		a.Sections = append(a.Sections, show.Section{Start: float64(i) * 40, End: float64(i+1) * 40, Energy: e, Group: string(rune('A' + i%3))})
	}
	return a
}

func TestTemperatureRules(t *testing.T) {
	cat := catalog(t)
	byID := map[string]Preset{}
	for _, p := range cat {
		byID[p.ID] = p
	}
	a := songAnalysis()
	for _, temp := range []string{TempWarm, TempCold} {
		c := ruleChoices(&a, cat, temp)
		exact := 0
		for _, s := range c.Sections {
			p := byID[s.Preset]
			if !p.Fits(temp) {
				t.Errorf("%s: parte %d com %s (%s)", temp, s.Index, p.Name, p.Temp)
			}
			if p.Temp == temp {
				exact++
			}
		}
		if exact < len(c.Sections)/2 {
			t.Errorf("%s: só %d partes na temperatura pedida", temp, exact)
		}
		for _, ac := range c.Accents {
			if !byID[ac.Preset].Fits(temp) {
				t.Errorf("%s: acento %s", temp, byID[ac.Preset].Name)
			}
		}
	}
	// O LLM pedindo um preset quente numa música fria é ignorado (fica a regra).
	fb := ruleChoices(&a, cat, TempCold)
	secs, _ := validate(&choices{Sections: []sectionChoice{{Index: 0, Preset: "pista:fogo"}, {Index: 1, Preset: "pista:club"}}}, &a, cat, fb, TempCold, false, nil)
	if secs[0].Preset != fb.Sections[0].Preset || secs[1].Preset != "pista:club" {
		t.Fatalf("validação fria: %+v", secs[:2])
	}
}

func TestLockRate(t *testing.T) {
	seq := Preset{Kind: "sequencia", StepMs: 400}
	if r := lockRate(seq, 150); math.Abs(r-1) > 1e-9 { // 400 ms = 1 batida a 150 BPM
		t.Fatalf("rate %v", r)
	}
	if r := lockRate(seq, 120); math.Abs(r-0.8) > 1e-9 { // batida de 500 ms
		t.Fatalf("rate %v", r)
	}
	if r := lockRate(Preset{Kind: "sequencia", StepMs: 400, Fade: true}, 120); r != 1 {
		t.Fatal("sequência com fade não deve ser acelerada")
	}
}

// TestRealSong compara com a análise de referência do librosa.
func TestRealSong(t *testing.T) {
	const wav = "../../musicas/praficarcomigo.wav"
	if _, err := os.Stat(wav); err != nil {
		t.Skip("sem a faixa de teste")
	}
	if testing.Short() {
		t.Skip("-short")
	}
	tl, warn, err := Generate(context.Background(), wav, catalog(t), Options{})
	if err != nil || warn != "" {
		t.Fatal(err, warn)
	}
	a := tl.Analysis
	// 126 BPM: o librosa dizia 123,05 (passo de quadro); as batidas dele
	// alternavam 0,488/0,464 s — 447 batidas em 212,4 s = 126,0.
	if math.Abs(a.BPM-126) > 0.5 || math.Abs(float64(len(a.Beats))-536) > 3 || a.Downbeats[0] != 3.135 {
		t.Fatalf("BPM %.2f, %d batidas, primeiro tempo 1 em %v", a.BPM, len(a.Beats), a.Downbeats[0])
	}
	if a.Sections[0].End < 31 || a.Sections[0].End > 32.5 {
		t.Fatalf("a intro deveria acabar em ~31,7 s: %+v", a.Sections[0])
	}
}

func TestNumberSections(t *testing.T) {
	names := []string{"intro", "verso", "verso", "refrão", "Verso", "refrão 7", "ponte", "refrão", "final", ""}
	secs := make([]show.Section, len(names))
	for i, n := range names {
		secs[i].Name = n
	}
	numberSections(secs)
	var got []string
	for _, s := range secs {
		got = append(got, s.Name)
	}
	want := "intro|verso 1|verso 1|refrão 1|Verso 2|refrão 2|ponte|refrão 3|final|"
	if strings.Join(got, "|") != want {
		t.Fatalf("partes: %s\n   quero: %s", strings.Join(got, "|"), want)
	}
}

func TestFreeMode(t *testing.T) {
	cat := catalog(t)
	a := songAnalysis()
	fb := ruleChoices(&a, cat, TempWarm)
	ruleEffects(&a, TempWarm, fb.Sections, nil, nil)
	for i, s := range fb.Sections {
		if s.Effect == nil || s.Preset != "" || s.Effect.Clean() != nil {
			t.Fatalf("regras, parte %d: %+v", i, s)
		}
		if _, ok := cleanEffect(&llmEffect{Type: s.Effect.Type, Steps: s.Effect.Steps, Fade: s.Effect.Fade, Beats: s.Effect.Beats}, TempWarm); !ok {
			t.Errorf("regras, parte %d: efeito fora da temperatura quente: %+v", i, s.Effect)
		}
	}
	if calm, hot := fb.Sections[0].Effect, fb.Sections[2].Effect; calm.Sync != show.SyncHits || hot.Sync != show.SyncHits || calm.Beats != 4 || !calm.Fade || hot.Beats != 1 || hot.Fade || len(hot.Steps) < 12 {
		t.Errorf("a sequência não acompanha a energia: %+v (calma), %+v (agitada)", calm, hot)
	}
	if a, b := fb.Sections[0].Effect.Steps[0], fb.Sections[3].Effect.Steps[0]; a[0] != b[0] { // mesmo grupo
		t.Errorf("partes do mesmo grupo com paletas diferentes: %v e %v", a, b)
	}

	llm := &choices{Sections: []sectionChoice{
		{Index: 0, Name: "intro", Free: &llmEffect{Type: "FADE", Colors: []string{"FF2000", "#ffb000"}, Beats: 3.7, Level: 0.1}},
		{Index: 1, Name: "verso", Free: &llmEffect{Type: "chase", Colors: []string{"#ff0000", "#0000ff"}}},              // azul numa música quente: fica a regra
		{Index: 2, Name: "refrão", Preset: "pista:fogo"},                                                                // preset no meio dos efeitos (misto)
		{Index: 3, Name: "verso", Free: &llmEffect{Type: "laser", Colors: []string{"#ff0000"}}, Preset: "pista:quente"}, // efeito inválido: vale o preset
		{Index: 4, Name: "refrão", Free: &llmEffect{Type: "pulse", Colors: []string{"#200000"}}},                        // escuro demais: fica a regra
		{Index: 5, Name: "final", Free: &llmEffect{Type: "sequence", Beats: 2, Fade: true, Steps: [][]string{
			{"#FF0000", "#ff6400", "#ffb400"}, {"#ff6400"}, {"#000000", "#ff0000"}, {"#ffffff", "#ffffff", "#ffffff"}}}},
	}}
	secs, _ := validate(llm, &a, cat, fb, TempWarm, true, nil)
	if e := secs[0].Effect; e == nil || e.Type != show.FXFade || e.Colors[0] != "#ff2000" || e.Beats != 4 || e.Level != 0.4 || e.Sync != show.SyncHits {
		t.Errorf("parte 0: %+v", secs[0].Effect)
	}
	if secs[1].Effect != fb.Sections[1].Effect || secs[4].Effect != fb.Sections[4].Effect {
		t.Errorf("efeito recusado deveria deixar o das regras: %+v, %+v", secs[1].Effect, secs[4].Effect)
	}
	if secs[2].Effect != nil || secs[2].Preset != "pista:fogo" || secs[3].Effect != nil || secs[3].Preset != "pista:quente" {
		t.Errorf("presets: %+v, %+v", secs[2], secs[3])
	}
	if e := secs[5].Effect; e == nil || e.Type != show.FXSequence || !e.Fade || e.Beats != 2 || len(e.Steps) != 3 || // o passo todo branco sai
		e.Steps[0][0] != "#ff0000" || len(e.Steps[1]) != 3 || e.Steps[2][2] != "#800000" { // o canhão apagado fica com a cor do passo, fechada
		t.Errorf("parte 5: %+v", secs[5].Effect)
	}
	for name, steps := range map[string][][]string{
		"quase tudo apagado": {{"#ff0000", "#000000", "#000000"}, {"#000000", "#000000", "#000000"}, {"#000000", "#ff0000", "#000000"}, {"#000000", "#000000", "#000000"}},
		"pisca-pisca":        {{"#ffffff"}, {"#000000"}},
		"fria numa quente":   {{"#ff0000"}, {"#0000ff"}},
		"um passo só":        {{"#ff0000"}},
		"cor que não é cor":  {{"#ff0000"}, {"vermelho"}},
		"passo sem cor":      {{"#ff0000"}, {}},
	} {
		if e, ok := cleanEffect(&llmEffect{Type: "sequence", Steps: steps}, TempWarm); ok {
			t.Errorf("sequência %q deveria ser recusada: %+v", name, e)
		}
	}
	// Fora do modo livre, o efeito do LLM é ignorado.
	if plain, _ := validate(llm, &a, cat, ruleChoices(&a, cat, TempWarm), TempWarm, false, nil); plain[0].Effect != nil || plain[0].Preset == "" {
		t.Errorf("efeito aceito fora do modo livre: %+v", plain[0])
	}

	tl := build("teste", "teste.wav", a, cat, secs, nil, llm, "modelo", Options{Free: true, Temperature: TempWarm})
	if c := tl.Cues[0]; c.Kind != "effect" || c.Preset != "" || c.Name != "LIVRE | TRANSIÇÃO" || c.Effect == nil {
		t.Errorf("troca 0: %+v", c)
	}
	if c := tl.Cues[2]; c.Kind != "preset" || c.Preset != "pista:fogo" || c.Effect != nil {
		t.Errorf("troca 2: %+v", c)
	}
	if !strings.Contains(string(tl.Personality), `"misto"`) || !strings.Contains(string(tl.Generated), `"free":true`) {
		t.Errorf("personality %s, generated %s", tl.Personality, tl.Generated)
	}
	if _, err := show.Compile(tl, show.NewPresets(console(t).Library, positions(t)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

// TestPickHits: os toques saem nos picos da curva de ataques, nunca a menos de
// minHitGap um do outro (fica o mais forte), e um trecho parado ganha batidas
// da grade para a luz não congelar.
func TestPickHits(t *testing.T) {
	frame := func(sec float64) int { return int(sec * SR / hop) }
	env := make([]float64, frame(12))
	for _, p := range []struct{ t, v float64 }{{1, 1}, {1.5, 0.8}, {1.7, 0.3}, {2, 0.9}, {2.2, 1}, {3, 0.5}, {3.02, 0.03}} {
		env[frame(p.t)] = p.v
	}
	var beats []float64
	for b := 0.5; b < 12; b += 0.5 {
		beats = append(beats, b)
	}
	hits := pickHits(env, beats, 120)
	near := func(want float64) bool {
		return slices.ContainsFunc(hits, func(h float64) bool { return math.Abs(h-want) < 0.03 })
	}
	for _, want := range []float64{1, 1.5, 2.2, 3} {
		if !near(want) {
			t.Errorf("faltou o toque de %.1f s: %v", want, hits)
		}
	}
	if near(1.7) || near(2) {
		t.Errorf("toque fraco colado num forte deveria sair: %v", hits)
	}
	for i := 1; i < len(hits); i++ {
		if hits[i]-hits[i-1] < minHitGap-1e-3 {
			t.Errorf("toques colados: %.3f e %.3f", hits[i-1], hits[i])
		}
		if hits[i]-hits[i-1] > 2.01 {
			t.Errorf("trecho parado de %.1f s a %.1f s sem batidas da grade", hits[i-1], hits[i])
		}
	}
	if pickHits(make([]float64, 100), nil, 120) != nil {
		t.Error("silêncio não tem toques")
	}
}

func TestColorTempOf(t *testing.T) {
	for hex, want := range map[string]string{
		"ff0000": TempWarm, "ff6400": TempWarm, "ffb400": TempWarm, "ff0a2d": TempWarm,
		"0000ff": TempCold, "00ffff": TempCold, "8cff00": TempCold, "7800ff": TempCold, "00ff78": TempCold,
		"ff2878": "", "ff00ff": "", "ffffff": "", "ffe0d0": "",
	} {
		var r, g, b int
		fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
		if got := colorTempOf(r, g, b); got != want {
			t.Errorf("#%s: %q, esperava %q", hex, got, want)
		}
	}
}
