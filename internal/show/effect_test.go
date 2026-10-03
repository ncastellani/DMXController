package show

import (
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/fixture"
)

func looks(t *testing.T, p *Program, fixtures []*fixture.Fixture, s float64) []fixture.Color {
	t.Helper()
	f := at(p, s)
	var out []fixture.Color
	for _, fx := range fixtures {
		c, strobe := fx.Look(&f)
		if strobe {
			t.Fatalf("%.2f s: efeito livre ligou o strobo do canhão", s)
		}
		out = append(out, c)
	}
	return out
}

func TestEffects(t *testing.T) {
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tl := timeline(
		Cue{T: 0, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXFixed, Colors: []string{"FF0000", "#00ff00", "#0000ff"}}},
		Cue{T: 10, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXChase, Colors: []string{"#ff0000", "#0000ff"}, Beats: 1}},
		Cue{T: 20, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXAlternate, Colors: []string{"#ff0000", "#0000ff"}, Beats: 2}},
		Cue{T: 30, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXPulse, Colors: []string{"#ff0000"}, Beats: 1}},
		Cue{T: 40, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXFade, Colors: []string{"#ff0000", "#0000ff"}, Beats: 4, Level: 0.5}},
		Cue{T: 50, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXSequence, Colors: []string{"#ffffff"}, Beats: 2, Steps: [][]string{
			{"#FF0000", "#00ff00", "#0000ff"}, {"#00ff00"}, {"#0000ff", "#000000"}}}},
		Cue{T: 60, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXSequence, Beats: 2, Fade: true, Steps: [][]string{{"#ff0000"}, {"#0000ff"}}}},
	)
	tl.Analysis.BPM = 120 // batida de 0,5 s
	p, err := Compile(tl, NewPresets(workspace(t), fixtures), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	red, green, blue := fixture.Color{R: 255}, fixture.Color{G: 255}, fixture.Color{B: 255}
	if c := tl.Cues[0]; c.Name != "LIVRE | CORES FIXAS" || c.Effect.Colors[0] != "#ff0000" || c.Effect.Beats != 0 {
		t.Errorf("efeito não foi acertado: %q %+v", c.Name, c.Effect)
	}
	if got := looks(t, p, fixtures, 5); got[0] != red || got[1] != green || got[2] != blue {
		t.Errorf("fixed: %v", got)
	}
	// Corrida: esquerda, centro, direita, centro, esquerda (um passo por batida).
	for k, lit := range []int{0, 1, 2, 1, 0} {
		got := looks(t, p, fixtures, 10.1+0.5*float64(k))
		for i, c := range got {
			if want := map[bool]fixture.Color{true: red, false: blue}[i == lit]; c != want {
				t.Errorf("chase, passo %d, canhão %d: %v", k, i, c)
			}
		}
	}
	if a, b := looks(t, p, fixtures, 20.5), looks(t, p, fixtures, 21.5); a[0] != red || a[1] != blue || a[2] != red || b[0] != blue || b[1] != red {
		t.Errorf("alternate: %v depois %v", a, b)
	}
	// Pulso: forte na batida, cai sem apagar e volta na batida seguinte.
	hi, lo, again := looks(t, p, fixtures, 30.01)[0], looks(t, p, fixtures, 30.45)[0], looks(t, p, fixtures, 30.51)[0]
	if hi.R < 240 || lo.R > 120 || lo.R < 60 || again.R < 240 {
		t.Errorf("pulse: %d → %d → %d", hi.R, lo.R, again.R)
	}
	// Transição a 50%: no meio do passo (1 s de 2 s), metade de cada cor.
	if c := looks(t, p, fixtures, 41)[0]; c.R < 55 || c.R > 72 || c.B < 55 || c.B > 72 || c.G != 0 {
		t.Errorf("fade: %v", c)
	}
	// Sequência: um passo a cada 2 batidas (1 s), em loop; 1 cor = os 3 iguais,
	// 2 cores = laterais e centro.
	if e := tl.Cues[5].Effect; e.Colors != nil || len(e.Steps[1]) != 3 || e.Steps[0][0] != "#ff0000" || tl.Cues[5].Name != "LIVRE | SEQUÊNCIA" {
		t.Errorf("sequência não foi acertada: %+v", e)
	}
	off := fixture.Color{}
	for k, want := range [][]fixture.Color{{red, green, blue}, {green, green, green}, {blue, off, blue}, {red, green, blue}} {
		if got := looks(t, p, fixtures, 50.5+float64(k)); got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Errorf("sequence, passo %d: %v", k, got)
		}
	}
	// Com fade: no começo do passo ainda é quase a cor anterior; da metade em diante, a do passo.
	if a, b, c := looks(t, p, fixtures, 61.05)[0], looks(t, p, fixtures, 61.25)[0], looks(t, p, fixtures, 61.6)[0]; a.R < 200 || b.R < 100 || b.B < 100 || c != blue {
		t.Errorf("sequence com fade: %v → %v → %v", a, b, c)
	}
}

// Com sync "hits", o passo troca nos toques da música (não na grade), e
// toques colados demais são pulados.
func TestEffectHits(t *testing.T) {
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	seq := func() *Effect {
		return &Effect{Type: FXSequence, Sync: SyncHits, Beats: 4, Steps: [][]string{{"#ff0000"}, {"#00ff00"}, {"#0000ff"}}}
	}
	tl := timeline(
		Cue{T: 0, Layer: "base", Kind: "effect", Effect: seq()},
		Cue{T: 10, Layer: "base", Kind: "effect", Effect: seq()},
		Cue{T: 15, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXPulse, Sync: SyncHits, Colors: []string{"#ff0000"}}},
		Cue{T: 18, Layer: "base", Kind: "effect", Effect: &Effect{Type: FXFixed, Sync: SyncHits, Colors: []string{"#ff0000"}}},
	)
	tl.Analysis.BPM = 120
	tl.Analysis.Hits = []float64{0.1, 1, 1.2, 2.7, 3, 10.5, 11.4, 16, 17}
	p, err := Compile(tl, NewPresets(workspace(t), fixtures), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if tl.Cues[0].Effect.Sync != SyncHits || tl.Cues[3].Effect.Sync != "" {
		t.Errorf("sync: %q e %q (fixo não tem passo)", tl.Cues[0].Effect.Sync, tl.Cues[3].Effect.Sync)
	}
	red, green, blue := fixture.Color{R: 255}, fixture.Color{G: 255}, fixture.Color{B: 255}
	// 0,1 s está colado no início e 1,2 s no toque de 1 s: não contam. 3 s está a 0,3 s de 2,7: também não.
	for _, c := range []struct {
		t    float64
		want fixture.Color
	}{{0.5, red}, {1.1, green}, {1.5, green}, {2.8, blue}, {3.5, blue}, {9.9, blue}, {10.2, red}, {10.6, green}, {11.5, blue}, {14.9, blue}} {
		if got := looks(t, p, fixtures, c.t)[0]; got != c.want {
			t.Errorf("%.1f s: %v, esperava %v", c.t, got, c.want)
		}
	}
	// Pulso: acende no toque (16 s) e cai até o toque seguinte.
	if a, b, c := looks(t, p, fixtures, 16.02)[0], looks(t, p, fixtures, 16.9)[0], looks(t, p, fixtures, 17.02)[0]; a.R < 240 || b.R > 100 || c.R < 240 {
		t.Errorf("pulse nos toques: %d → %d → %d", a.R, b.R, c.R)
	}
	// Sem toques na análise, valem as batidas.
	tl2 := timeline(Cue{T: 0, Layer: "base", Kind: "effect", Effect: seq()})
	tl2.Analysis.BPM, tl2.Analysis.Beats = 120, []float64{0.5, 1, 1.5, 2}
	p2, _ := Compile(tl2, NewPresets(workspace(t), fixtures), 10*time.Second)
	if a, b := looks(t, p2, fixtures, 0.7)[0], looks(t, p2, fixtures, 1.2)[0]; a != green || b != blue {
		t.Errorf("sem toques: %v, %v", a, b)
	}
}

// Nenhum efeito troca de cor 3 vezes por segundo ou mais, seja qual for o
// andamento e o passo pedido.
func TestEffectsNeverFlash(t *testing.T) {
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, bpm := range []float64{60, 90, 126, 174, 240} {
		for _, kind := range []string{FXFixed, FXAlternate, FXChase, FXPulse, FXFade, FXSequence} {
			e := &Effect{Type: kind, Colors: []string{"#ff0000", "#0000ff", "#00ff00"}, Beats: 0.5, Steps: [][]string{{"#ffffff"}, {"#000000"}, {"#ff0000", "#0000ff"}}}
			tl := timeline(Cue{T: 0, Layer: "base", Kind: "effect", Effect: e})
			tl.Analysis.BPM = bpm
			if bpm == 174 { // toques a cada 50 ms: o efeito pula os que estão colados
				e.Sync = SyncHits
				for h := 0.0; h < 20; h += 0.05 {
					tl.Analysis.Hits = append(tl.Analysis.Hits, h)
				}
			}
			p, err := Compile(tl, NewPresets(workspace(t), fixtures), 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if kind != FXFixed && e.Step(bpm) < MinEffectStep {
				t.Errorf("%s a %g BPM: passo de %v", kind, bpm, e.Step(bpm))
			}
			pv := RenderPreview(p, fixtures, 20, 30)
			for i, pos := range pv.Positions {
				for j, s := range pos {
					if s.FX != 0 {
						t.Fatalf("%s a %g BPM, canhão %d, %.2f s: marcado como rápido/strobo", kind, bpm, i, float64(j)/30)
					}
				}
			}
		}
	}
}

func TestEffectErrors(t *testing.T) {
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]Cue{
		"sem efeito":        {Layer: "base", Kind: "effect"},
		"tipo desconhecido": {Layer: "base", Kind: "effect", Effect: &Effect{Type: "laser", Colors: []string{"#ff0000"}}},
		"cor inválida":      {Layer: "base", Kind: "effect", Effect: &Effect{Type: FXFixed, Colors: []string{"vermelho"}}},
		"poucas cores":      {Layer: "base", Kind: "effect", Effect: &Effect{Type: FXFade, Colors: []string{"#ff0000"}}},
		"sequência curta":   {Layer: "base", Kind: "effect", Effect: &Effect{Type: FXSequence, Steps: [][]string{{"#ff0000"}}}},
		"passo vazio":       {Layer: "base", Kind: "effect", Effect: &Effect{Type: FXSequence, Steps: [][]string{{"#ff0000"}, {}}}},
		"na fritação":       {Layer: "flash", Kind: "effect", Duration: 1, Effect: &Effect{Type: FXFixed, Colors: []string{"#ff0000"}}},
	} {
		if _, err := Compile(timeline(c), NewPresets(workspace(t), fixtures), 10*time.Second); err == nil {
			t.Errorf("%s: deveria dar erro", name)
		}
	}
}
