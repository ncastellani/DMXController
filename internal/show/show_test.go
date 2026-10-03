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
	"github.com/ncastellani/DMXController/internal/qlc"
)

func workspace(t *testing.T) *qlc.Workspace {
	t.Helper()
	ws, err := qlc.Load("../../config/aniversario_STROBE.qxw")
	if err != nil {
		t.Fatal(err)
	}
	return ws
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
	presets := NewPresets(workspace(t), nil)
	prog, err := Compile(timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "qlc:0"},                   // TODOS | VERMELHO
		Cue{T: 10, Layer: "base", Kind: "preset", Preset: "qlc:1"},                  // TODOS | AZUL
		Cue{T: 12, Layer: "flash", Kind: "preset", Preset: "qlc:100", Duration: 60}, // FRITAÇÃO | BRANCO
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
	if cur == nil || cur.Preset != "qlc:1" || next == nil || next.T != 12 {
		t.Fatalf("Current(11) = %+v, %+v", cur, next)
	}
}

func TestSequenceRate(t *testing.T) {
	presets := NewPresets(workspace(t), nil)
	// PISTA | FLASH (id 55): passos de 100 ms alternando branco e apagado.
	normal, _ := Compile(timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "qlc:55", Rate: 1}), presets, time.Second)
	fast, _ := Compile(timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "qlc:55", Rate: 2}), presets, time.Second)
	// Em 0,15 s: normal está no passo 1 (apagado); 2× está no passo 3 (apagado)... compara 0,05 vs 0,025.
	if at(normal, 0.15)[0] != 0 || at(normal, 0.05)[0] != 255 {
		t.Fatal("sequência normal fora do esperado")
	}
	if at(fast, 0.075)[0] != 0 {
		t.Fatal("rate 2 deveria chegar no passo apagado em 0,075 s")
	}
}

func TestCompileErrors(t *testing.T) {
	presets := NewPresets(workspace(t), nil)
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
	if _, err := Compile(tl, NewPresets(workspace(t), nil), 10*time.Second); err != nil {
		t.Fatal(err)
	}
}

// TestShowClock usa o relógio interno (sem saída de som).
func TestShowClock(t *testing.T) {
	dir := t.TempDir()
	tl := timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "qlc:0"}, Cue{T: 10, Layer: "base", Kind: "preset", Preset: "qlc:1"})
	data, _ := json.Marshal(tl)
	os.WriteFile(filepath.Join(dir, "01-teste.json"), data, 0o644)
	os.WriteFile(filepath.Join(dir, "teste.wav"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "02-quebrado.json"), []byte("{"), 0o644)

	eng := &engine.Engine{}
	var log []string
	sh := New(dir, NewPresets(workspace(t), nil), eng, nil, time.Second, func(f string, a ...any) { log = append(log, f) })
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
	writeSong(t, dir, "a", 0.4, "qlc:0") // TODOS | VERMELHO
	writeSong(t, dir, "b", 0.4, "qlc:1") // TODOS | AZUL
	eng := &engine.Engine{}
	sh := New(dir, NewPresets(workspace(t), nil), eng, nil, time.Second, func(string, ...any) {})
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
	writeSong(t, dir, "a", 2, "qlc:0")
	writeSong(t, dir, "b", 2, "qlc:1")
	sh := New(dir, NewPresets(workspace(t), nil), &engine.Engine{}, nil, time.Second, func(string, ...any) {})
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
		{T: 31.7, Layer: "base", Preset: "qlc:41"},
		{T: 31.7, Layer: "flash", Preset: "qlc:99"},
		{T: 0, Layer: "base", Preset: "qlc:47"},
		{T: 31.7, Layer: "base", Kind: "blackout"},
	}
	SortCues(cues)
	var got []string
	for _, c := range cues {
		got = append(got, c.Layer+":"+c.Preset+c.Kind)
	}
	if want := "base:qlc:47|flash:qlc:99|base:qlc:41|base:blackout"; strings.Join(got, "|") != want {
		t.Fatalf("ordem %s, quero %s", strings.Join(got, "|"), want)
	}
}
