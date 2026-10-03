package analysis

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
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

func catalog(t *testing.T) []Preset {
	t.Helper()
	c, err := control.LoadConsole("../../config/console.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	rig, err := control.NewRig(fixtures, c, filepath.Join(t.TempDir(), "patch.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pos []*fixture.Fixture
	for i := range rig.Fixtures {
		pos = append(pos, rig.Logical(i))
	}
	return Catalog(c, pos)
}

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
		case "qlc:55": // PISTA | FLASH: passos de 100 ms piscando
			if p.Agitacao != "agitada" || p.Kind != "sequencia" {
				t.Errorf("PISTA | FLASH: %+v", p)
			}
		case "qlc:47": // PISTA | CHILL: fades lentos
			if p.Agitacao != "calma" || !p.Fade {
				t.Errorf("PISTA | CHILL: %+v", p)
			}
		case "qlc:94": // FRITAÇÃO | BRANCO MÁXIMO
			if p.Layer != "flash" || p.Agitacao != "strobo" {
				t.Errorf("FRITAÇÃO | BRANCO MÁXIMO: %+v", p)
			}
		case "qlc:1": // TODOS | AZUL
			if len(p.Colors) != 1 || p.Colors[0] != "AZUL" || p.Temp != TempCold || p.Hex != "#0000ff" {
				t.Errorf("TODOS | AZUL: %+v", p)
			}
		case "qlc:10": // TODOS | VERDE LIMÃO: não é botão, mas está na biblioteca
			if !p.Library || p.Layer != "base" || p.Hex != "#8cff00" || p.Caption != "VERDE LIMÃO" {
				t.Errorf("VERDE LIMÃO: %+v", p)
			}
		case "qlc:43": // PISTA | CLUB: o único frio agitado
			if p.Temp != TempCold || p.Agitacao != "agitada" {
				t.Errorf("CLUB: %+v", p)
			}
		case "qlc:61": // PISTA | FOGO
			if p.Temp != TempWarm {
				t.Errorf("FOGO: %+v", p)
			}
		case "qlc:49": // PISTA | FESTA: de tudo um pouco
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
			{Index: 0, Name: "intro", Preset: "qlc:47"},
			{Index: 1, Name: "refrão", Preset: "qlc:94"}, // flash não serve de base: ignora
			{Index: 7, Name: "?", Preset: "qlc:41"},      // parte inexistente: ignora
		},
		Accents: []accent{
			{Time: 40.2, Preset: "qlc:100", Beats: 9},  // encaixa em 40,0 e limita a 4 batidas
			{Time: 45, Preset: "qlc:100", Beats: 1},    // a menos de 20 s do anterior: descarta
			{Time: 80.1, Preset: "qlc:41", Beats: 1},   // não é flash: descarta
			{Time: 100.26, Preset: "qlc:99", Beats: 1}, // ok, em 100,5? não: 100,0 ou 100,5 → mais perto 100,5
		},
	}
	secs, acc := validate(llm, &a, cat, fallback, TempAuto)
	if secs[0].Preset != "qlc:47" || secs[1].Preset != fallback.Sections[1].Preset {
		t.Fatalf("partes: %+v", secs)
	}
	if len(acc) != 2 || acc[0].Time != 40 || acc[0].Beats != 4 || acc[1].Time != 100.5 {
		t.Fatalf("acentos: %+v", acc)
	}
	tl := build("teste", "teste.wav", a, cat, secs, acc, nil, nil, llm, "modelo", Options{}, nil)
	last := tl.Cues[len(tl.Cues)-1]
	if last.Kind != "blackout" || tl.Cues[0].Preset != "qlc:47" || tl.Analysis.Sections[0].Name != "intro" {
		t.Fatalf("timeline: %+v", tl.Cues)
	}
	for _, c := range tl.Cues {
		if c.Layer == "flash" && c.Duration > 2.0001 { // 4 batidas a 120 BPM
			t.Fatalf("flash longo demais: %+v", c)
		}
	}
	// A timeline gerada precisa compilar no player.
	ws, _ := control.LoadConsole("../../config/console.yaml")
	if _, err := show.Compile(tl, show.NewPresets(ws.Workspace, nil), 10*time.Second); err != nil {
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
	secs, _ := validate(&choices{Sections: []sectionChoice{{Index: 0, Preset: "qlc:61"}, {Index: 1, Preset: "qlc:43"}}}, &a, cat, fb, TempCold)
	if secs[0].Preset != fb.Sections[0].Preset || secs[1].Preset != "qlc:43" {
		t.Fatalf("validação fria: %+v", secs[:2])
	}
}

// TestLyricsColors: só com a letra colada, a cor citada acende no início da
// parte por 4 compassos e depois volta a sequência da parte.
func TestLyricsColors(t *testing.T) {
	cat := catalog(t)
	a := songAnalysis()
	fb := ruleChoices(&a, cat, TempWarm)
	one, two, nine := 1, 2, 9
	llm := &choices{
		Sections: []sectionChoice{
			{Index: 0, Name: "verso", Preset: "qlc:10", Lyric: "teu  vestido\nverde-limão", Reason: "a letra cita"}, // cor fria fixa não serve de parte numa música quente
		},
		Colors: []colorChoice{
			{Part: &one, Word: "lilás", Color: &newColor{Name: "lilás", RGB: "C8A2C8"}, Reason: "lilás na letra"},
			{Part: &two, Word: "verde-limão", Preset: "qlc:10"},          // cor fixa fria vale: está na letra
			{Part: &two, Color: &newColor{Name: "breu", RGB: "#050505"}}, // escura demais: ignora
			{Part: &nine, Color: &newColor{Name: "x", RGB: "#ff0000"}},   // parte inexistente
			{Part: &one, Preset: fb.Sections[0].Preset},                  // sequência não é cor fixa
		},
	}
	secs, _ := validate(llm, &a, cat, fb, TempWarm)
	if secs[0].Preset != fb.Sections[0].Preset {
		t.Fatalf("parte 0: %+v", secs[0])
	}
	colors := validateColors(llm, &a, cat, nil)
	if len(colors) != 2 || colors[0].T != 40 || colors[0].Preset != "cor:#c8a2c8" || colors[0].Name != "COR | LILÁS" ||
		colors[0].Note != "letra: “lilás” · lilás na letra" || colors[1].T != 80 || colors[1].Preset != "qlc:10" {
		t.Fatalf("cores: %+v", colors)
	}

	tl := build("teste", "teste.wav", a, cat, secs, nil, colors, nil, llm, "", Options{Temperature: TempWarm, Lyrics: "..."}, nil)
	at := func(t float64) []show.Cue {
		var out []show.Cue
		for _, c := range tl.Cues {
			if c.T == t && c.Layer == "base" {
				out = append(out, c)
			}
		}
		return out
	}
	// A cor substitui a troca da parte (40 s) e a parte volta 4 compassos depois.
	if c := at(40); len(c) != 1 || c[0].Preset != "cor:#c8a2c8" {
		t.Fatalf("40 s: %+v", c)
	}
	if c := at(48); len(c) != 1 || c[0].Preset != fb.Sections[1].Preset || c[0].Note != "volta depois da cor" {
		t.Fatalf("48 s: %+v", c)
	}
	if !strings.Contains(string(tl.Generated), `"temperature":"quente"`) || !strings.Contains(string(tl.Generated), `"lyrics":true`) {
		t.Fatalf("generated: %s", tl.Generated)
	}
	ws, _ := control.LoadConsole("../../config/console.yaml")
	if _, err := show.Compile(tl, show.NewPresets(ws.Workspace, positions(t)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

// TestSungColors: com a versão cantada, a cor acende na batida da palavra e
// fica até o fim da frase (2 a 4 compassos).
func TestSungColors(t *testing.T) {
	cat := catalog(t)
	a := songAnalysis() // 120 BPM: batida 0,5 s, compasso 2 s; partes de 40 s
	fb := ruleChoices(&a, cat, TempAuto)
	s := &sung{
		Lines: []show.Lyric{
			{T: 39.8, End: 41, Text: "verde-limão"},
			{T: 59, End: 63, Text: "o céu ficou azul e lilás"},
			{T: 236, End: 239, Text: "vermelho no fim"},
		},
		Colors: []mention{
			{ID: "c0", Time: 40.3, Word: "verde-limão", Line: 0}, // perto do início da parte 1 (40 s): vai para lá
			{ID: "c1", Time: 61.3, Word: "azul", Line: 1},        // batida 61,5; frase acaba em 63 → 2 compassos
			{ID: "c2", Time: 62, Word: "lilás", Line: 1},         // a menos de 2 batidas da anterior: fica de fora
			{ID: "c3", Time: 237, Word: "vermelho", Line: 2},     // no fim: não volta a parte depois
		},
	}
	llm := &choices{Colors: []colorChoice{
		{ID: "c0", Preset: "qlc:10"},
		{ID: "c1", Color: &newColor{Name: "azul céu", RGB: "#3399ff"}},
		{ID: "c2", Color: &newColor{Name: "lilás", RGB: "#c8a2c8"}},
		{ID: "c3", Color: &newColor{Name: "vermelho", RGB: "#ff0000"}},
		{ID: "c3", Preset: "qlc:10"}, // repetida: vale a primeira
		{ID: "c9", Preset: "qlc:10"}, // não existe
	}}
	colors := validateColors(llm, &a, cat, s)
	if len(colors) != 4 || colors[1].Until != 63 || colors[1].Note != "letra: “o céu ficou azul e lilás”" {
		t.Fatalf("cores: %+v", colors)
	}
	secs, _ := validate(nil, &a, cat, fb, TempAuto)
	tl := build("teste", "teste.wav", a, cat, secs, nil, colors, s, llm, "", Options{}, map[string]any{"voice": true})
	var got []string
	for _, c := range tl.Cues {
		if c.Layer == "base" && c.T >= 39 && c.T <= 240 && c.T != 120 && c.T != 160 && c.T != 200 {
			got = append(got, fmt.Sprintf("%g %s", c.T, c.Name))
		}
	}
	name := func(id string) string {
		for _, x := range cat {
			if x.ID == id {
				return x.Name
			}
		}
		return "?"
	}
	p := func(i int) string { return name(fb.Sections[i].Preset) }
	want := []string{
		"40 " + name("qlc:10"), // a cor no lugar da parte (e da troca dela)
		"44 " + p(1),           // 2 compassos (frase curta) e volta a parte
		"61.5 COR | AZUL CÉU",  // na batida da palavra
		"65.5 " + p(1),         // fim da frase (63) → 2 compassos
		"80 " + p(2),           // parte seguinte
		"237 COR | VERMELHO",   // sem volta: a música acaba antes
		"240 ",                 // apagão do fim
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("trocas:\n%s\nquero:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(tl.Lyrics) != 3 || !strings.Contains(string(tl.Generated), `"voice":true`) {
		t.Fatalf("letra %+v, generated %s", tl.Lyrics, tl.Generated)
	}
	ws, _ := control.LoadConsole("../../config/console.yaml")
	if _, err := show.Compile(tl, show.NewPresets(ws.Workspace, positions(t)), 10*time.Second); err != nil {
		t.Fatal(err)
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
