package analysis

import (
	"testing"

	"github.com/ncastellani/DMXController/internal/show"
)

// TestSplitByPulse: uma parte em que a bateria entra no meio é dividida ali
// (na batida), o pedaço menor leva "+" ou "-", e uma parte uniforme fica
// inteira.
func TestSplitByPulse(t *testing.T) {
	const beat = 0.5
	var hits, strength, beats []float64
	level := func(t float64) float64 {
		switch {
		case t < 30: // parte A: uniforme
			return 0.5
		case t < 40: // parte B: 10 s fracos...
			return 0.3
		case t < 60: // ...e a bateria entra
			return 0.9
		case t < 88: // parte C: forte...
			return 0.8
		}
		return 0.3 // ...e some nos últimos 2 s (curto demais para dividir: menos de 8 toques)
	}
	for i := 0; i < 180; i++ {
		tm := float64(i) * beat
		beats = append(beats, tm)
		hits = append(hits, tm+0.01)
		strength = append(strength, level(tm)*(1+0.1*float64(i%3-1))) // um pouco de variação
	}
	secs := []show.Section{{Start: 0, End: 30, Group: "A"}, {Start: 30, End: 60, Group: "B"}, {Start: 60, End: 90, Group: "C"}}
	out := splitByPulse(secs, hits, strength, beats, beat, func(a, b float64) float64 { return 0.5 })
	if len(out) != 4 {
		t.Fatalf("partes: %+v", out)
	}
	if a := out[0]; a.Group != "A" || a.End != 30 {
		t.Errorf("a parte uniforme mudou: %+v", a)
	}
	if weak, strong := out[1], out[2]; weak.Group != "B-" || strong.Group != "B" || weak.End != 40 || strong.Start != 40 || weak.Bars != 5 || strong.Bars != 10 {
		t.Errorf("divisão da parte B: %+v e %+v", weak, strong)
	}
	if c := out[3]; c.Group != "C" || c.Start != 60 || c.End != 90 {
		t.Errorf("a parte C não deveria ser dividida por um rabo de 2 s: %+v", c)
	}
	setPulse(out, hits, strength)
	if out[1].Pulse >= out[2].Pulse || out[2].Pulse < 0.9 || out[0].Pulse < 0.4 || out[0].Pulse > 0.7 {
		t.Errorf("batida das partes: %v %v %v", out[0].Pulse, out[1].Pulse, out[2].Pulse)
	}
	// O verso de batida forte e volume baixo pede mais agitação que o volume diz.
	if s := (show.Section{Energy: 0.2, Pulse: 1}); drive(s) != 0.6 || drive(show.Section{Energy: 0.2}) != 0.2 {
		t.Errorf("drive: %v", drive(s))
	}
}

// TestPickAccents: só os toques que se destacam dos vizinhos viram realce,
// com espaço entre eles.
func TestPickAccents(t *testing.T) {
	const beat = 0.5
	var hits, strength []float64
	for i := 0; i < 80; i++ {
		hits = append(hits, float64(i)*beat)
		strength = append(strength, 0.4)
	}
	strength[20] = 1.4                    // uma batida solta, 3,5× os vizinhos
	strength[40], strength[41] = 0.9, 1.0 // dois seguidos: fica o maior
	strength[60] = 0.5                    // pouco acima: não é destaque
	for i := 70; i < 80; i++ {            // trecho todo forte: ninguém se destaca
		strength[i] = 1.2
	}
	acc := pickAccents(hits, strength, beat)
	if len(acc) != 2 || acc[0].T != 10 || acc[0].V != 1 || acc[1].T != 20.5 || acc[1].V != 0.8 {
		t.Fatalf("destaques: %+v", acc)
	}
	// Um toque só 1,8× acima dos vizinhos é ruído, não pico.
	strength[20], strength[41], strength[40] = 0.72, 0.4, 0.4
	if acc := pickAccents(hits, strength, beat); len(acc) != 0 {
		t.Errorf("pico fraco virou destaque: %+v", acc)
	}

	// Fim do som: a luz vai até o último quadro audível; a cauda é o trecho
	// em que o som já caiu. A 43 quadros/s: 60 s altos, 6 s de cauda baixa, 4 s de silêncio.
	var r []float64
	for i := 0; i < 70*SR/hop; i++ {
		switch tm := float64(i) * hop / SR; {
		case tm < 60:
			r = append(r, 0.2)
		case tm < 66:
			r = append(r, 0.005)
		default:
			r = append(r, 0)
		}
	}
	if end, fade := soundEnd(r, 70); end < 65.9 || end > 66.1 || fade < 5.9 || fade > 6.1 {
		t.Errorf("fim do som em %.2f com cauda de %.2f; quero 66 e 6", end, fade)
	}
	if end, fade := soundEnd(r[:60*SR/hop], 60); end < 59.9 || end > 60 || fade != 0 { // música que acaba no talo
		t.Errorf("fim seco: %.2f, cauda %.2f", end, fade)
	}
	if pickAccents(nil, nil, beat) != nil {
		t.Errorf("sem toques não há destaque")
	}
}
