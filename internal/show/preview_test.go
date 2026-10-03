package show

import (
	"testing"
	"time"

	"luzes/internal/fixture"
)

func TestPreview(t *testing.T) {
	fixtures, err := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Compile(timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "cor:#ff0000"},
		Cue{T: 6, Layer: "flash", Kind: "preset", Preset: "qlc:100", Duration: 1}, // FRITAÇÃO | BRANCO (strobo nativo)
		Cue{T: 10, Layer: "base", Kind: "preset", Preset: "qlc:55"},               // PISTA | FLASH: pisca rápido
		Cue{T: 16, Layer: "base", Kind: "blackout"},
	), NewPresets(workspace(t), fixtures), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	pv := RenderPreview(p, fixtures, 20, 10)
	if len(pv.Positions) != 3 || len(pv.Positions[0]) != 200 || pv.FPS != 10 {
		t.Fatalf("formato: %d canhões, %d amostras, fps %d", len(pv.Positions), len(pv.Positions[0]), pv.FPS)
	}
	red := fixture.Color{R: 255}
	for i, pos := range pv.Positions {
		if s := pos[30]; s.Color != red || s.FX != 0 {
			t.Errorf("canhão %d, 3 s: %+v (deveria ser vermelho parado)", i, s)
		}
		if s := pos[65]; s.FX&FXStrobe == 0 {
			t.Errorf("canhão %d, 6,5 s: %+v (deveria marcar o strobo)", i, s)
		}
		fast := 0
		for _, s := range pos[100:160] {
			if s.FX&FXFast != 0 {
				fast++
			}
		}
		if fast < 30 {
			t.Errorf("canhão %d: só %d de 60 amostras do PISTA | FLASH marcadas como troca rápida", i, fast)
		}
		if s := pos[180]; s.Color != (fixture.Color{}) || s.FX != 0 {
			t.Errorf("canhão %d, 18 s: %+v (deveria estar apagado)", i, s)
		}
	}
	// Na tela, nenhum trecho troca de cor mais de 3 vezes por segundo.
	for i, pos := range pv.Positions {
		for a := 0; a+10 <= len(pos); a++ {
			n := 0
			for j := a + 1; j < a+10; j++ {
				if differs(pos[j-1].Color, pos[j].Color) {
					n++
				}
			}
			if n > MaxPreviewChanges {
				t.Fatalf("canhão %d: %d trocas entre %.1f s e %.1f s", i, n, float64(a)/10, float64(a+10)/10)
			}
		}
	}
}
