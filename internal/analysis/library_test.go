package analysis

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/show"
)

// TestLibraryEffect: um modo de pista vira efeito do modo livre com os mesmos
// passos (a FLASH, pisca-pisca, não), como está ou com outra paleta.
func TestLibraryEffect(t *testing.T) {
	lib := console(t).Library
	if n := len(usable(lib)); len(lib.Effects) != 32 || n != 31 || Usable(lib.Effect("pista:flash")) {
		t.Fatalf("%d modos de pista, %d servem de efeito; quero 32 e 31 (a FLASH fora)", len(lib.Effects), n)
	}
	club := lib.Effect("qlc:43")                                                                  // o id antigo ainda vale
	if club == nil || club.ID != "pista:club" || club.Temp != TempCold || len(club.Steps) != 16 { // sem as rajadas de branco/apagado, tiradas a pedido
		t.Fatalf("CLUB: %+v", club)
	}
	if pal := effectPalette(club); len(pal) != 5 || pal[0] != "#0000a0" || !strings.HasPrefix(effectPattern(club), "ABC CAB BCA DCD CDC ADB BAD CBC BCB ") {
		t.Errorf("CLUB: paleta %v, passos %s", pal, effectPattern(club))
	}
	if effectPalette(lib.Effect("pista:contramao")) != nil { // degradê de 17 tons: não troca de paleta
		t.Errorf("CONTRAMÃO não deveria aceitar cores novas")
	}

	// Como está: troca nos toques, com o nome do efeito.
	e, ok := LibraryEffect(club, 126, nil, TempAuto)
	if !ok || e.Type != show.FXSequence || e.Sync != show.SyncHits || len(e.Steps) != 16 || show.EffectName(e) != "EFEITO | CLUB" {
		t.Fatalf("CLUB como efeito: %+v", e)
	}
	if _, ok := LibraryEffect(club, 126, nil, TempWarm); ok {
		t.Errorf("CLUB (frio) aceito numa música quente com as cores originais")
	}
	// Sem extremos: nenhum passo de branco total ou apagado.
	for _, st := range e.Steps {
		if strings.Join(st, " ") == "#ffffff #ffffff #ffffff" || strings.Join(st, " ") == "#000000 #000000 #000000" {
			t.Errorf("CLUB como efeito ainda tem um passo extremo: %v", st)
		}
	}
	// Com cores novas: o desenho fica e a paleta muda.
	warm := []string{"#ff0000", "#ff6400", "#ffb400", "#ff0a2d", "#c80000"}
	e, ok = LibraryEffect(club, 126, warm, TempWarm)
	if !ok || e.Steps[0][0] != "#ff0000" || e.Steps[1][0] != "#ffb400" || len(e.Steps) != 16 || e.Steps[7][0] != "#ffb400" || e.Name != "CLUB (cores novas)" {
		t.Fatalf("CLUB com cores quentes: %+v", e)
	}
	if _, ok := LibraryEffect(club, 126, []string{"#ff0000", "#0000ff"}, TempWarm); ok {
		t.Errorf("azul aceito numa música quente")
	}
	if e, ok := LibraryEffect(lib.Effect("pista:contramao"), 126, warm, TempWarm); !ok || e.Name != "CONTRAMÃO" {
		t.Errorf("CONTRAMÃO ignora as cores novas: %+v", e)
	}
	// Tons escuros sobem até o brilho mínimo, na mesma cor (PULSO QUENTE:
	// vermelho #c80000 com dimmer 100 emite #4e0000).
	if e, _ := LibraryEffect(lib.Effect("pista:pulso-quente"), 126, nil, TempAuto); e.Steps[0][0] != "#800000" || e.Steps[3][0] != "#c80000" {
		t.Errorf("PULSO QUENTE, 1º passo: %v", e.Steps[0])
	}
	// Lento (CHILL, passos de 2,5 a 3 s em fade): segue o passo original, não os toques.
	if e, ok := LibraryEffect(lib.Effect("pista:chill"), 120, nil, TempAuto); !ok || e.Sync != "" || e.Beats != 4 || !e.Fade {
		t.Errorf("CHILL: %+v", e)
	}
	// Todos compilam (MinEffectStep vale ao compilar: nenhum passa de 3 trocas por segundo).
	a := songAnalysis()
	tl := &show.Timeline{Analysis: a}
	for i, le := range usable(lib) {
		e, _ := LibraryEffect(le, a.BPM, nil, TempAuto)
		tl.Cues = append(tl.Cues, show.Cue{T: float64(i), Layer: "base", Kind: "effect", Effect: e})
	}
	if _, err := show.Compile(tl, show.NewPresets(lib, positions(t)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if tl.Cues[0].Name != "EFEITO | AZUL ROSA" {
		t.Errorf("nome da troca: %q", tl.Cues[0].Name)
	}
}

// TestLibraryChoices: as regras do modo livre e as escolhas do LLM usam a
// biblioteca.
func TestLibraryChoices(t *testing.T) {
	c := console(t)
	cat, lib := Catalog(c), c.Library
	a := songAnalysis()

	// Regras: modos de pista na temperatura, a energia do efeito acompanha a
	// da parte e grupos diferentes não repetem o efeito.
	for i := range lib.Effects {
		lib.Effects[i].Mood = nil
	}
	byName := func(e *show.Effect) string { return "pista:" + strings.ReplaceAll(strings.ToLower(e.Name), " ", "-") }
	fb := ruleChoices(&a, cat, TempWarm)
	ruleEffects(&a, TempWarm, fb.Sections, lib, nil)
	names := map[string]bool{}
	for i, s := range fb.Sections {
		if s.Effect == nil || s.Effect.Name == "" || s.Preset != "" {
			t.Fatalf("parte %d sem efeito da biblioteca: %+v", i, s)
		}
		if le := lib.Effect(byName(s.Effect)); le == nil || le.Temp != TempWarm {
			t.Errorf("parte %d: %q não é um modo de pista quente", i, s.Effect.Name)
		}
		names[s.Effect.Name] = true
	}
	if len(names) != 6 {
		t.Errorf("quero 6 efeitos diferentes (6 partes, cada uma num grupo/energia), veio %v", names)
	}
	calm, hot := lib.Effect(byName(fb.Sections[0].Effect)), lib.Effect(byName(fb.Sections[4].Effect))
	if calm.Agitacao != "calma" || hot.Agitacao != "agitada" {
		t.Errorf("energia: parte calma com %s (%s), agitada com %s (%s)", calm.Name, calm.Agitacao, hot.Name, hot.Agitacao)
	}
	// O clima da música desempata entre efeitos de energia parecida.
	lib.Effect("pista:golden").Mood = &Mood{Energy: 0.5, Joy: 0.9, Romance: 0.9}
	lib.Effect("pista:quente").Mood = &Mood{Energy: 0.5, Joy: 0.1, Tension: 0.9}
	if e := pickEffect(lib, 0.5, &Mood{Joy: 0.9, Romance: 0.8}, TempWarm, "", nil); e.ID != "pista:golden" {
		t.Errorf("música alegre e romântica: %s", e.ID)
	}
	if e := pickEffect(lib, 0.5, &Mood{Joy: 0.1, Tension: 1}, TempWarm, "", nil); e.ID != "pista:quente" {
		t.Errorf("música tensa: %s", e.ID)
	}
	if e := pickEffect(lib, 0.5, &Mood{Joy: 0.9, Romance: 0.8}, TempWarm, "", map[string]bool{"pista:golden": true}); e.ID == "pista:golden" {
		t.Errorf("repetiu o efeito já usado")
	}

	// LLM: modo de pista como efeito (como está e com cores novas), como
	// preset (toca o original) e um que não existe.
	llm := &choices{Sections: []sectionChoice{
		{Index: 0, Name: "intro", Free: &llmEffect{Model: "pista:golden"}},
		{Index: 1, Name: "verso", Free: &llmEffect{Model: "pista:club", Colors: []string{"#ff0000", "#ff6400", "#ffb400", "#ff0a2d", "#c80000"}}},
		{Index: 2, Name: "refrão", Preset: "pista:fogo"},
		{Index: 3, Name: "verso", Free: &llmEffect{Model: "pista:club"}},       // frio numa música quente: fica a regra
		{Index: 4, Name: "refrão", Free: &llmEffect{Model: "pista:naoexiste"}}, // fica a regra
		{Index: 5, Name: "final", Free: &llmEffect{Model: "pista:flash"}},      // pisca-pisca não vira efeito: fica a regra
	}}
	secs, _ := validate(llm, &a, cat, fb, TempWarm, true, lib)
	if e := secs[0].Effect; e == nil || e.Name != "GOLDEN" {
		t.Errorf("parte 0: %+v", e)
	}
	if e := secs[1].Effect; e == nil || e.Name != "CLUB (cores novas)" || e.Steps[0][2] != "#ffb400" {
		t.Errorf("parte 1: %+v", e)
	}
	if secs[2].Effect != nil || secs[2].Preset != "pista:fogo" {
		t.Errorf("parte 2: %+v", secs[2])
	}
	for _, i := range []int{3, 4, 5} {
		if secs[i].Effect != fb.Sections[i].Effect {
			t.Errorf("parte %d: recusado deveria ficar com a regra: %+v", i, secs[i].Effect)
		}
	}
	// Fora do modo livre, o efeito não entra.
	if plain, _ := validate(llm, &a, cat, ruleChoices(&a, cat, TempWarm), TempWarm, false, lib); plain[0].Effect != nil || plain[1].Effect != nil {
		t.Errorf("efeito da biblioteca fora do modo livre: %+v", plain[0])
	}
	// O prompt mostra o desenho e a paleta, sem a FLASH.
	if d := describeLibrary(lib, TempWarm); !strings.Contains(d, "pista:club | CLUB") || !strings.Contains(d, "A=#0000a0") || !strings.Contains(d, "só com cores novas") || strings.Contains(d, "pista:flash") {
		t.Errorf("descrição da biblioteca:\n%s", d)
	}
	if p := paletteText(lib, TempWarm); !strings.Contains(p, "ÂMBAR #ff2d00") || strings.Contains(p, "CIANO") || !strings.Contains(p, "ROSA #ff2878") || !strings.Contains(p, "BORDÔ #b4000e") {
		t.Errorf("paleta quente: %s", p)
	}
	llm.Score = &Mood{Energy: 1.4, Joy: 0.8}
	tl := build("teste", "teste.wav", a, cat, secs, nil, llm, "modelo", Options{Free: true, Temperature: TempWarm})
	if tl.Cues[0].Name != "EFEITO | GOLDEN" || !strings.Contains(string(tl.Personality), `"mood":{"energy":1,"joy":0.8`) {
		t.Errorf("timeline: %q, %s", tl.Cues[0].Name, tl.Personality)
	}
}

// TestSoften: os efeitos gerados não têm branco total, apagado nem tons
// escuros; o desenho (quem está aceso em cada passo) continua.
func TestSoften(t *testing.T) {
	e := &show.Effect{Type: show.FXSequence, Steps: [][]string{
		{"#7800ff", "#000000", "#000000"}, // um canhão por vez: os outros ficam na mesma cor, fechada
		{"#000000", "#7800ff", "#000000"},
		{"#ffffff", "#ffffff", "#ffffff"}, // branco total: sai
		{"#000000", "#000000", "#000000"}, // apagado: sai
		{"#190400", "#140000", "#000000"}, // quase apagado: sai
		{"#ff0000", "#ffffff", "#ff0000"}, // um branco solto vira a cor do passo (o branco é dos picos)
		{"#640000", "#ff5000", "#3c1300"}, // tons escuros sobem, sem mudar de cor
	}}
	if !soften(e) {
		t.Fatal("sequência recusada")
	}
	want := [][]string{
		{"#7800ff", "#3c0080", "#3c0080"},
		{"#3c0080", "#7800ff", "#3c0080"},
		{"#ff0000", "#ff0000", "#ff0000"},
		{"#800000", "#ff5000", "#802900"},
	}
	if len(e.Steps) != len(want) {
		t.Fatalf("passos: %v", e.Steps)
	}
	for i := range want {
		if strings.Join(e.Steps[i], " ") != strings.Join(want[i], " ") {
			t.Errorf("passo %d: %v, quero %v", i, e.Steps[i], want[i])
		}
	}
	if soften(&show.Effect{Type: show.FXSequence, Steps: [][]string{{"#ffffff", "#ffffff", "#ffffff"}, {"#000000", "#000000", "#000000"}, {"#ff0000", "#ff0000", "#ff0000"}}}) {
		t.Errorf("sobrou um passo só: não é mais uma sequência")
	}
	// Onde o branco é uma das cores do desenho (um quarto dos tons ou mais), fica.
	themed := &show.Effect{Type: show.FXSequence, Steps: [][]string{{"#ff0000", "#ffffff", "#ff0000"}, {"#ffffff", "#ff0000", "#ffffff"}}}
	if !soften(themed) || themed.Steps[0][1] != "#ffffff" || themed.Steps[1][0] != "#ffffff" {
		t.Errorf("sequência de vermelho e branco perdeu o branco: %v", themed.Steps)
	}
	// Nenhum efeito da biblioteca nem das regras sai com extremos.
	lib := console(t).Library
	check := func(name string, e *show.Effect) {
		for i, st := range e.Steps {
			white := 0
			for _, c := range st {
				var r, g, b int
				fmt.Sscanf(c, "#%02x%02x%02x", &r, &g, &b)
				if max(r, g, b) < minBright {
					t.Errorf("%s, passo %d: %s é escuro demais", name, i+1, c)
				}
				if c == "#ffffff" {
					white++
				}
			}
			if white == 3 {
				t.Errorf("%s, passo %d: branco total", name, i+1)
			}
		}
	}
	for _, le := range usable(lib) {
		e, _ := LibraryEffect(le, 120, nil, TempAuto)
		check(le.ID, e)
	}
	a := songAnalysis()
	fb := ruleChoices(&a, Catalog(console(t)), TempAuto)
	ruleEffects(&a, TempAuto, fb.Sections, nil, nil)
	for i, s := range fb.Sections {
		check(fmt.Sprintf("regras, parte %d", i), s.Effect)
	}
}

// TestMixedMode: o modo misto combina presets e efeitos, cada um em pelo
// menos um terço das partes, pelas regras e depois das escolhas do LLM.
func TestMixedMode(t *testing.T) {
	c := console(t)
	cat, lib := Catalog(c), c.Library
	a := songAnalysis() // energias 0,1 0,4 0,9 0,5 0,95 0,2 nos grupos A B C A B C
	count := func(secs []sectionChoice) (fx, pr int) {
		for i, s := range secs {
			switch {
			case s.Effect != nil && s.Preset == "":
				fx++
			case s.Effect == nil && s.Preset != "":
				pr++
			default:
				t.Fatalf("parte %d sem luz ou com as duas: %+v", i, s)
			}
		}
		return fx, pr
	}
	fallbacks := func() (mixed, presets, effects []sectionChoice) {
		fb := ruleChoices(&a, cat, TempAuto)
		presets = append(presets, fb.Sections...)
		ruleEffects(&a, TempAuto, fb.Sections, lib, nil)
		effects = append(effects, fb.Sections...)
		mixByEnergy(&a, fb.Sections, presets)
		return fb.Sections, presets, effects
	}

	// Regras: o grupo mais agitado (B: 0,4 e 0,95) fica com presets; os outros, com efeitos.
	mixed, presets, effects := fallbacks()
	if fx, pr := count(mixed); fx != 4 || pr != 2 || mixed[1].Preset == "" || mixed[4].Preset == "" {
		t.Fatalf("regras: %d efeitos e %d presets: %+v", fx, pr, mixed)
	}

	// A IA só mandou efeitos: os grupos mais agitados passam a presets.
	all := &choices{}
	for i := range a.Sections {
		all.Sections = append(all.Sections, sectionChoice{Index: i, Name: "parte", Free: &llmEffect{Type: "sequence", Steps: [][]string{{"#ff0064", "#00ffff", "#7800ff"}, {"#7800ff", "#ff0064", "#00ffff"}}}})
	}
	secs, _ := validate(all, &a, cat, choices{Sections: effects}, TempAuto, true, lib)
	if fx, _ := count(secs); fx != 6 {
		t.Fatalf("a IA deveria ter ficado só com efeitos: %d", fx)
	}
	balance(&a, secs, presets, effects)
	if fx, pr := count(secs); pr < 2 || fx < 2 || secs[4].Preset == "" || secs[4].Name != "parte" || !strings.HasPrefix(secs[4].Reason, "misto") {
		t.Errorf("só efeitos → %d efeitos e %d presets: %+v", fx, pr, secs[4])
	}

	// A IA só mandou presets: os grupos mais calmos passam a efeitos.
	all = &choices{}
	for i := range a.Sections {
		all.Sections = append(all.Sections, sectionChoice{Index: i, Name: "parte", Preset: "pista:pop"})
	}
	secs, _ = validate(all, &a, cat, choices{Sections: effects}, TempAuto, true, lib)
	balance(&a, secs, presets, effects)
	if fx, pr := count(secs); pr < 2 || fx < 2 || secs[0].Effect == nil || secs[4].Preset != "pista:pop" {
		t.Errorf("só presets → %d efeitos e %d presets", fx, pr)
	}

	// Já combinada: nada muda.
	before := append([]sectionChoice(nil), mixed...)
	balance(&a, mixed, presets, effects)
	for i := range mixed {
		if mixed[i].Preset != before[i].Preset || mixed[i].Effect != before[i].Effect {
			t.Errorf("parte %d mudou sem precisar", i)
		}
	}
	tl := build("teste", "teste.wav", a, cat, mixed, nil, nil, "", Options{Mixed: true})
	if !strings.Contains(string(tl.Personality), `"misto"`) || !strings.Contains(string(tl.Generated), `"mix":true`) || !strings.Contains(string(tl.Generated), `"free":true`) {
		t.Errorf("personality %s, generated %s", tl.Personality, tl.Generated)
	}
	if _, err := show.Compile(tl, show.NewPresets(lib, positions(t)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if r := partRule(prompt{free: true, mixed: true, lib: lib}); !strings.Contains(r, "MODO MISTO") || strings.Contains(partRule(prompt{free: true, lib: lib}), "MODO MISTO") {
		t.Errorf("regra do modo misto no pedido à IA: %v", strings.Contains(r, "MODO MISTO"))
	}
}
