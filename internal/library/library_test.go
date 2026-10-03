package library

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/fixture"
)

func positions(t *testing.T) []*fixture.Fixture {
	t.Helper()
	fx, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return fx
}

func frame(t *testing.T, l *Library, id string, at time.Duration) []byte {
	t.Helper()
	_, src, ok := l.Preset(id, positions(t))
	if !ok {
		t.Fatalf("preset %s não existe", id)
	}
	var f dmx.Frame
	src.Render(at, &f)
	return f[:21]
}

func same(a []byte, want ...byte) bool { return string(a) == string(want) }

// TestShowLibrary confere a biblioteca do show contra os valores DMX do
// workspace do QLC+ de onde ela foi convertida (a conversão foi provada
// quadro a quadro; aqui ficam amostras, para o arquivo não mudar sem querer).
func TestShowLibrary(t *testing.T) {
	l, err := Load("../../config/biblioteca.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Colors) != 29 || len(l.Strobes) != 11 || len(l.Effects) != 32 {
		t.Fatalf("%d cores, %d strobos, %d modos de pista; quero 29, 11 e 32", len(l.Colors), len(l.Strobes), len(l.Effects))
	}
	// Cor fixa com dimmer (TODOS | ÂMBAR ESCURO: dimmer 180, R 255, G 35).
	if f := frame(t, l, "todos:ambar-escuro", 0); !same(f, 180, 255, 35, 0, 0, 0, 0, 180, 255, 35, 0, 0, 0, 0, 180, 255, 35, 0, 0, 0, 0) {
		t.Errorf("ÂMBAR ESCURO: %v", f)
	}
	// Fritação: strobo nativo (canal 6 = 50, canal 7 = velocidade); o id antigo ainda vale.
	if f := frame(t, l, "qlc:97", 0); !same(f[:7], 255, 0, 0, 160, 0, 50, 253) || !same(f[14:], 255, 0, 0, 160, 0, 50, 253) {
		t.Errorf("FRITAÇÃO | AZUL PROFUNDO: %v", f)
	}
	if name, _, _ := l.Preset("strobe:branco-254", nil); name != "STROBE | BRANCO 254" || l.Canonical("qlc:101") != "strobe:branco-254" {
		t.Errorf("strobo: %q, %q", name, l.Canonical("qlc:101"))
	}
	// PISTA | FLASH: 100 ms branco, 100 ms apagado (dimmer 0), os passos rápidos intactos.
	if f := frame(t, l, "pista:flash", 50*time.Millisecond); !same(f[:7], 255, 255, 255, 255, 0, 0, 0) {
		t.Errorf("FLASH em 50 ms: %v", f)
	}
	if f := frame(t, l, "pista:flash", 150*time.Millisecond); !same(f[:7], 0, 0, 0, 0, 0, 0, 0) {
		t.Errorf("FLASH em 150 ms: %v", f)
	}
	// PISTA | CHILL: o 2º passo começa em 2,5 s e leva 900 ms de fade só na
	// direita (#5a00c8 → #ff2878); no meio do fade está a meio caminho.
	if f := frame(t, l, "pista:chill", 2950*time.Millisecond); !same(f[:7], 255, 0, 0, 160, 0, 0, 0) || !same(f[14:], 255, 173, 20, 160, 0, 0, 0) {
		t.Errorf("CHILL no meio do fade: %v", f)
	}
	// O loop volta ao primeiro passo.
	chill := l.Effect("pista:chill")
	var cycle time.Duration
	for _, st := range chill.Parsed() {
		cycle += st.Duration
	}
	if cycle != 32700*time.Millisecond || !chill.Fades() || chill.TypicalStep() != 2800 {
		t.Errorf("CHILL: ciclo %v, passo típico %v", cycle, chill.TypicalStep())
	}
	if a, b := frame(t, l, "pista:chill", time.Second), frame(t, l, "pista:chill", cycle+time.Second); string(a) != string(b) {
		t.Errorf("CHILL não repete depois do ciclo")
	}
	if c := l.Effect("pista:pulso-quente").StepColors()[0]; c[0] != "#4e0000" { // #c80000 com dimmer 100
		t.Errorf("PULSO QUENTE, cor emitida: %v", c)
	}
}

func TestTone(t *testing.T) {
	for in, want := range map[string]Tone{
		"#ff2300@180": {fixture.Color{R: 255, G: 0x23}, 180},
		"FF0000":      {fixture.Color{R: 255}, 255},
		"#000000":     {fixture.Color{}, 0},
		"#000000@255": {fixture.Color{}, 255},
	} {
		got, err := ParseTone(in)
		if err != nil || got != want {
			t.Errorf("%s: %+v, %v", in, got, err)
		}
		if back, _ := ParseTone(got.String()); back != got {
			t.Errorf("%s não volta igual: %s", in, got)
		}
	}
	for _, bad := range []string{"", "#ff00", "vermelho", "#ff0000@300", "#ff0000@x"} {
		if _, err := ParseTone(bad); err == nil {
			t.Errorf("tom %q aceito", bad)
		}
	}
}

func TestParseAndSave(t *testing.T) {
	src := `
colors:
  - {id: 'todos:rubi', name: RUBI, hex: 'E0115F'}
strobes:
  - {id: 'fritacao:rubi', name: FRITAÇÃO | RUBI, hex: '#e0115f', speed: 250}
effects:
  - id: pista:vai-e-vem
    name: VAI E VEM
    step_ms: 400
    steps:
      - '#e0115f #000000 #000000'
      - '#000000 #e0115f@128 #000000 800ms fade200ms'
`
	l, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	e := l.Effect("pista:vai-e-vem")
	if st := e.Parsed(); len(st) != 2 || st[0].Duration != 400*time.Millisecond || st[1].Duration != 800*time.Millisecond || st[1].FadeIn != 200*time.Millisecond || st[1].Tones[1].Dimmer != 128 {
		t.Fatalf("passos: %+v", st)
	}
	if e.Agitacao != "media" || e.Temp != "mista" || l.Colors[0].Hex != "#e0115f" {
		t.Errorf("padrões: %+v, %+v", e, l.Colors[0])
	}
	e.Mood = &Mood{Energy: 1.7, Joy: 0.333, Label: "  teste  "}
	path := filepath.Join(t.TempDir(), "b.yaml")
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if m := back.Effect("pista:vai-e-vem").Mood; m == nil || m.Energy != 1 || m.Joy != 0.33 || m.Label != "teste" || len(back.Strobes) != 1 || back.Strobes[0].Speed != 250 {
		t.Errorf("relido: %+v, %+v", m, back.Strobes)
	}
	for name, bad := range map[string]string{
		"id sem grupo":         "colors: [{id: rubi, name: R, hex: '#ff0000'}]",
		"id repetido":          "colors: [{id: 'todos:a', name: A, hex: '#ff0000'}, {id: 'todos:a', name: B, hex: '#00ff00'}]",
		"cor inválida":         "colors: [{id: 'todos:a', name: A, hex: 'vermelho'}]",
		"strobo sem speed":     "strobes: [{id: 'strobe:a', name: A, hex: '#ff0000'}]",
		"efeito sem passos":    "effects: [{id: 'pista:a', name: A, step_ms: 300}]",
		"passo sem duração":    "effects: [{id: 'pista:a', name: A, steps: ['#ff0000 #ff0000 #ff0000']}]",
		"passo com lixo":       "effects: [{id: 'pista:a', name: A, step_ms: 300, steps: ['#ff0000 rapido']}]",
		"efeito fora do grupo": "effects: [{id: 'modo:a', name: A, step_ms: 300, steps: ['#ff0000']}]",
	} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%s: biblioteca inválida aceita", name)
		} else if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("%s: erro vazio", name)
		}
	}
}
