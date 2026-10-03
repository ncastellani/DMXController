package show

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/library"
)

func workspace(t *testing.T) *library.Library {
	t.Helper()
	lib, err := library.Load("../../config/biblioteca.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

// rig são os 3 canhões nos endereços lógicos 1, 8 e 15.
func rig(t *testing.T) []*fixture.Fixture {
	t.Helper()
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return fixtures
}

func timeline(cues ...Cue) *Timeline {
	t := &Timeline{Version: 1, Cues: cues}
	t.Song.Title, t.Song.File, t.Song.Duration = "teste", "teste.wav", 20
	t.Analysis.Sections = []Section{{Start: 0, End: 10, Name: "verso"}, {Start: 10, End: 20, Name: "refrão"}}
	return t
}

func at(p *Program, s float64) dmx.Frame {
	var f dmx.Frame
	p.Render(secs(s), &f)
	return f
}

func TestProgram(t *testing.T) {
	presets := NewPresets(workspace(t), rig(t))
	prog, err := Compile(timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:vermelho"},                  // TODOS | VERMELHO
		Cue{T: 10, Layer: "base", Kind: "preset", Preset: "todos:azul"},                     // TODOS | AZUL
		Cue{T: 12, Layer: "flash", Kind: "preset", Preset: "fritacao:branco", Duration: 60}, // FRITAÇÃO | BRANCO
		Cue{T: 18, Layer: "base", Kind: "blackout"},
	), presets, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f := at(prog, 5); f[1] != 255 || f[3] != 0 {
		t.Fatalf("t=5: deveria ser vermelho: %v", f[:7])
	}
	if f := at(prog, 11); f[1] != 0 || f[3] != 255 {
		t.Fatalf("t=11: deveria ser azul: %v", f[:7])
	}
	if f := at(prog, 13); f[5] != 50 || f[1] != 255 {
		t.Fatalf("t=13: deveria estar no flash (strobo): %v", f[:7])
	}
	// Flash limitado a 2 s (pedia 60).
	if f := at(prog, 14.5); f[5] != 0 || f[3] != 255 {
		t.Fatalf("t=14.5: flash deveria ter acabado (teto): %v", f[:7])
	}
	if f := at(prog, 19); f != (dmx.Frame{}) {
		t.Fatalf("t=19: deveria estar apagado: %v", f[:7])
	}
	cur, next := prog.Current(secs(11))
	if cur == nil || cur.Preset != "todos:azul" || next == nil || next.T != 12 {
		t.Fatalf("Current(11) = %+v, %+v", cur, next)
	}
}

func TestSequenceRate(t *testing.T) {
	presets := NewPresets(workspace(t), rig(t))
	// PISTA | FLASH (id 55): passos de 100 ms alternando branco e apagado.
	normal, _ := Compile(timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "pista:flash", Rate: 1}), presets, time.Second)
	fast, _ := Compile(timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "pista:flash", Rate: 2}), presets, time.Second)
	// Em 0,15 s: normal está no passo 1 (apagado); 2× está no passo 3 (apagado)... compara 0,05 vs 0,025.
	if at(normal, 0.15)[0] != 0 || at(normal, 0.05)[0] != 255 {
		t.Fatal("sequência normal fora do esperado")
	}
	if at(fast, 0.075)[0] != 0 {
		t.Fatal("rate 2 deveria chegar no passo apagado em 0,075 s")
	}
}

func TestCompileErrors(t *testing.T) {
	presets := NewPresets(workspace(t), rig(t))
	for _, c := range []Cue{
		{Layer: "base", Kind: "preset", Preset: "qlc:9999"},
		{Layer: "base", Kind: "preset", Preset: "user:x"},
		{Layer: "flash", Kind: "blackout"},
		{Layer: "lateral", Kind: "blackout"},
		{Layer: "base", Kind: "fumaça"},
	} {
		if _, err := Compile(timeline(c), presets, time.Second); err == nil {
			t.Errorf("cue %+v deveria dar erro", c)
		}
	}
}

func TestRealTimeline(t *testing.T) {
	tl, err := LoadTimeline("../../musicas/praficarcomigo.json")
	if os.IsNotExist(err) {
		t.Skip("sem musicas/praficarcomigo.json")
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compile(tl, NewPresets(workspace(t), rig(t)), 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

// TestShowClock usa o relógio interno (sem saída de som).
func TestShowClock(t *testing.T) {
	dir := t.TempDir()
	tl := timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:vermelho"}, Cue{T: 10, Layer: "base", Kind: "preset", Preset: "todos:azul"})
	data, _ := json.Marshal(tl)
	os.WriteFile(filepath.Join(dir, "01-teste.json"), data, 0o644)
	os.WriteFile(filepath.Join(dir, "teste.wav"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "02-quebrado.json"), []byte("{"), 0o644)

	eng := &engine.Engine{}
	var log []string
	sh := New(dir, NewPresets(workspace(t), rig(t)), eng, nil, time.Second, func(f string, a ...any) { log = append(log, f) })
	if err := sh.Scan(); err != nil {
		t.Fatal(err)
	}
	songs := sh.Songs()
	if len(songs) != 2 || songs[0].Title != "teste" || songs[1].Err == "" {
		t.Fatalf("setlist: %+v", songs)
	}
	if err := sh.Load(1); err == nil {
		t.Fatal("música quebrada não deveria carregar")
	}
	if err := sh.Load(0); err != nil {
		t.Fatal(err)
	}
	sh.Seek(11)
	var f dmx.Frame
	eng.Render(time.Now(), &f)
	if f != (dmx.Frame{}) {
		t.Fatalf("carregada e parada, a luz deveria ficar apagada: %v", f[:7])
	}
	sh.Play()
	time.Sleep(50 * time.Millisecond)
	st := sh.Status()
	if !st.Playing || st.Position < 11.04 || st.Section != "refrão" || !strings.Contains(st.Cue, "AZUL") {
		t.Fatalf("status tocando: %+v", st)
	}
	eng.Render(time.Now(), &f)
	if f[3] != 255 {
		t.Fatalf("tocando em 11 s deveria mostrar o azul: %v", f[:7])
	}
	sh.Pause()
	eng.Render(time.Now(), &f)
	if f != (dmx.Frame{}) {
		t.Fatalf("pausada, a luz deveria apagar: %v", f[:7])
	}
	sh.Stop()
	eng.Render(time.Now(), &f)
	if f != (dmx.Frame{}) || sh.Status().Position != 0 {
		t.Fatal("stop deveria apagar a luz do show e voltar ao início")
	}
}

// writeSong grava uma música curta com uma cor fixa (preset) do começo ao fim.
func writeSong(t *testing.T, dir, name string, dur float64, preset string) {
	t.Helper()
	tl := timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: preset})
	tl.Song.Title, tl.Song.File, tl.Song.Duration = name, name+".wav", dur
	tl.Analysis.Sections = []Section{{Start: 0, End: dur, Name: name}}
	data, _ := json.Marshal(tl)
	os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)
	os.WriteFile(filepath.Join(dir, name+".wav"), nil, 0o644)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("esperando: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestContinuousPlayback(t *testing.T) {
	dir := t.TempDir()
	writeSong(t, dir, "a", 0.4, "todos:vermelho") // TODOS | VERMELHO
	writeSong(t, dir, "b", 0.4, "todos:azul")     // TODOS | AZUL
	eng := &engine.Engine{}
	sh := New(dir, NewPresets(workspace(t), rig(t)), eng, nil, time.Second, func(string, ...any) {})
	changed := 0
	sh.OnChange(func() { changed++ })
	sh.Scan()
	if err := sh.SetLink("a.json", true, 200*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if songs := sh.Songs(); !songs[0].Link || songs[0].Gap != 200*time.Millisecond || songs[1].Link {
		t.Fatalf("emendas: %+v", songs)
	}
	sh.Load(0)
	waitFor(t, "b enfileirada", func() bool { return len(segs(sh)) == 2 })
	if seg := segs(sh)[1]; seg.start != 600*time.Millisecond {
		t.Fatalf("b deveria começar em 0,6 s (0,4 + 0,2 de intervalo): %v", seg.start)
	}
	color := func() (r, b byte) {
		var f dmx.Frame
		eng.Render(time.Now(), &f)
		return f[1], f[3]
	}
	sh.Play()
	if r, _ := color(); r != 255 {
		t.Fatal("começo deveria ser vermelho (a)")
	}
	sh.Seek(0.35)
	waitFor(t, "virada para b", func() bool { sh.Tick(); return sh.Status().Current == 1 })
	if r, b := color(); b != 255 || r != 0 {
		t.Fatalf("depois da emenda deveria ser azul (b): r=%d b=%d", r, b)
	}
	if changed != 1 || sh.Timeline().Song.Title != "b" {
		t.Fatalf("interface deveria ser avisada da troca (changed=%d)", changed)
	}
	if st := sh.Status(); st.Position > 0.4 || st.Duration != 0.4 {
		t.Fatalf("posição deveria ser relativa a b: %+v", st)
	}

	// O setlist.txt guarda a emenda.
	data, _ := os.ReadFile(filepath.Join(dir, OrderFile))
	if !strings.Contains(string(data), "a.json\temendar=0.2") {
		t.Fatalf("setlist.txt:\n%s", data)
	}
}

func TestUnlinkRemovesQueued(t *testing.T) {
	dir := t.TempDir()
	writeSong(t, dir, "a", 2, "todos:vermelho")
	writeSong(t, dir, "b", 2, "todos:azul")
	sh := New(dir, NewPresets(workspace(t), rig(t)), &engine.Engine{}, nil, time.Second, func(string, ...any) {})
	sh.Scan()
	sh.SetLink("a.json", true, 0)
	sh.Load(0)
	waitFor(t, "b enfileirada", func() bool { return len(segs(sh)) == 2 })
	if err := sh.SetLink("a.json", false, 0); err != nil {
		t.Fatal(err)
	}
	if len(segs(sh)) != 1 {
		t.Fatal("desligar a emenda deveria tirar b do fluxo")
	}
	sh.Play()
	sh.Seek(1.99)
	time.Sleep(50 * time.Millisecond)
	sh.Tick()
	if st := sh.Status(); st.Current != 0 || st.Playing {
		t.Fatalf("sem emenda, a deveria terminar e parar: %+v", st)
	}
}

// segs copia os trechos do fluxo (com o lock do playhead).
func segs(sh *Show) []pseg {
	sh.head.mu.Lock()
	defer sh.head.mu.Unlock()
	return append([]pseg(nil), sh.head.segs...)
}

func TestColorPreset(t *testing.T) {
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Compile(timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "cor:#c8a2c8", Name: "COR | LILÁS"},
		Cue{T: 5, Layer: "base", Kind: "preset", Preset: "cor:#00ff80"},
	), NewPresets(workspace(t), fixtures), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	f := at(p, 1)
	for _, fx := range fixtures {
		if c, strobe := fx.Look(&f); c != (fixture.Color{R: 0xc8, G: 0xa2, B: 0xc8}) || strobe {
			t.Fatalf("%s: %+v strobe=%v", fx.Name, c, strobe)
		}
	}
	if cur, _ := p.Current(secs(6)); cur.Name != "COR | #00ff80" {
		t.Fatalf("nome gerado: %q", cur.Name)
	}
	for _, bad := range []string{"cor:#12345", "cor:roxo", "cor:#gg0000"} {
		if _, err := Compile(timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: bad}), NewPresets(workspace(t), fixtures), time.Second); err == nil {
			t.Errorf("%s deveria falhar", bad)
		}
	}
}

// No mesmo instante, a fritação aparece antes da base (ela acaba e a base fica).
func TestSortCues(t *testing.T) {
	cues := []Cue{
		{T: 31.7, Layer: "base", Preset: "pista:pop"},
		{T: 31.7, Layer: "flash", Preset: "fritacao:ciano"},
		{T: 0, Layer: "base", Preset: "pista:chill"},
		{T: 31.7, Layer: "base", Kind: "blackout"},
	}
	SortCues(cues)
	var got []string
	for _, c := range cues {
		got = append(got, c.Layer+":"+c.Preset+c.Kind)
	}
	if want := "base:pista:chill|flash:fritacao:ciano|base:pista:pop|base:blackout"; strings.Join(got, "|") != want {
		t.Fatalf("ordem %s, quero %s", strings.Join(got, "|"), want)
	}
}

// TestAccentBump: num pico da música o canhão do centro dá um flash branco e
// os dos lados clareiam de leve, voltando em menos de um terço de segundo;
// sem base (apagado) não acende nada, e dois picos colados viram um só.
func TestAccentBump(t *testing.T) {
	tl := timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:azul"},
		Cue{T: 10, Layer: "base", Kind: "blackout"},
	)
	tl.Analysis.Accents = []Accent{{T: 2, V: 1}, {T: 2.6, V: 1}, {T: 5, V: 0.6}, {T: 12, V: 1}}
	p, err := Compile(tl, NewPresets(workspace(t), rig(t)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.accents) != 3 {
		t.Fatalf("picos a menos de 1 s deveriam virar um: %+v", p.accents)
	}
	look := func(pos int, s float64) fixture.Color {
		f := at(p, s)
		c, strobe := rig(t)[pos].Look(&f)
		if strobe {
			t.Fatalf("realce não liga strobo (%.2fs)", s)
		}
		return c
	}
	const left, center = 0, 1
	blue, white := fixture.Color{B: 255}, fixture.Color{R: 255, G: 255, B: 255}
	if look(center, 1.9) != blue || look(center, 2.3) != blue || look(center, 2.6) != blue || look(left, 4.9) != blue {
		t.Errorf("fora do pico a cor não muda: %v %v", look(center, 1.9), look(center, 2.3))
	}
	if c := look(center, 2.0); c != white {
		t.Errorf("no pico, o centro vai ao branco: %v", c)
	}
	if c := look(left, 2.0); c.R < 70 || c.R > 82 || c.B != 255 { // os lados só clareiam
		t.Errorf("no pico, o lado: %v", c)
	}
	if a, b := look(center, 2.05), look(center, 2.2); !(a.R > b.R && b.R > 0 && a.R < 255) {
		t.Errorf("o flash deveria cair suave: %v depois %v", a, b)
	}
	if c := look(center, 5.0); c.R < 148 || c.R > 158 { // pico mais fraco, flash mais fraco
		t.Errorf("pico de força 0,6: %v", c)
	}
	if c := look(center, 12.0); c != (fixture.Color{}) {
		t.Errorf("apagado continua apagado: %v", c)
	}
}

// TestFadeOut: antes de um blackout com fade a luz apaga aos poucos, mesmo
// atravessando uma troca de base, e só fica preta no blackout.
func TestFadeOut(t *testing.T) {
	tl := timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:vermelho"},
		Cue{T: 16, Layer: "base", Kind: "preset", Preset: "todos:azul"},
		Cue{T: 20, Layer: "base", Kind: "blackout", Fade: 8},
	)
	p, err := Compile(tl, NewPresets(workspace(t), rig(t)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	look := func(s float64) fixture.Color {
		f := at(p, s)
		c, _ := rig(t)[0].Look(&f)
		return c
	}
	if c := look(11.9); c != (fixture.Color{R: 255}) {
		t.Errorf("antes do fade: %v", c)
	}
	if c := look(14); c.R < 185 || c.R > 197 { // 2 de 8 s: 75% do brilho
		t.Errorf("fade em 14 s: %v", c)
	}
	if c := look(18); c.R != 0 || c.B < 60 || c.B > 68 { // já no azul, 25% do brilho
		t.Errorf("fade em 18 s: %v", c)
	}
	if c := look(19.9); c.B == 0 || c.B > 6 {
		t.Errorf("ainda há um resto de luz logo antes do fim: %v", c)
	}
	if c := look(20); c != (fixture.Color{}) {
		t.Errorf("no blackout: %v", c)
	}
	// Um fade absurdo num arquivo editado à mão é limitado.
	bad := timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:azul"}, Cue{T: 10, Layer: "base", Kind: "blackout", Fade: 999})
	if _, err := Compile(bad, NewPresets(workspace(t), rig(t)), time.Second); err != nil || bad.Cues[1].Fade != maxCueFade {
		t.Errorf("fade limitado: %v, %v", bad.Cues[1].Fade, err)
	}
}
