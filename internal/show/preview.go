package show

import (
	"math"
	"time"

	"luzes/internal/dmx"
	"luzes/internal/fixture"
)

// Prévia da luz de uma música sem tocar no DMX: o Program é desenhado num
// quadro à parte e a cor de cada canhão é lida com fixture.Look, como na
// prévia ao vivo do topo da interface.

// Efeitos que a prévia marca em vez de animar (segurança: a tela nunca pisca).
const (
	FXStrobe = 1 // strobo nativo do canhão (CH6 + CH7)
	FXFast   = 2 // trocas mais rápidas que MaxPreviewChanges: a prévia mostra a média
)

// MaxPreviewChanges é o máximo de trocas de cor por segundo que a prévia
// anima (limite usual para fotossensibilidade). Acima disso, o trecho vira
// a cor média com a marca FXFast.
const MaxPreviewChanges = 3

// previewSub é quantas vezes por amostra o programa é desenhado, para não
// perder trocas rápidas que caem entre duas amostras.
const previewSub = 4

// changeThreshold é a diferença num canal (0..255) que conta como troca de
// cor; fades lentos (CHILL, CINEMA) mudam pouco de um quadro para o outro.
const changeThreshold = 80

// Sample é a cor de um canhão num instante da prévia.
type Sample struct {
	Color fixture.Color
	FX    uint8 // FXStrobe | FXFast
}

// Preview é a luz de uma música amostrada FPS vezes por segundo.
type Preview struct {
	FPS       int
	Duration  float64
	Positions [][]Sample // [canhão][amostra], na ordem de positions
}

// RenderPreview desenha o programa de 0 a dur segundos e lê cada canhão
// (fixtures no quadro lógico, na ordem do palco).
func RenderPreview(prog *Program, positions []*fixture.Fixture, dur float64, fps int) *Preview {
	fps = min(max(fps, 1), 30)
	n := max(1, int(math.Ceil(dur*float64(fps))))
	rate := fps * previewSub
	step := time.Second / time.Duration(rate)
	raw := make([][]Sample, len(positions))
	for p := range raw {
		raw[p] = make([]Sample, n*previewSub)
	}
	for i := range n * previewSub {
		var f dmx.Frame
		prog.Render(time.Duration(i)*step, &f)
		for p, fx := range positions {
			c, strobe := fx.Look(&f)
			s := Sample{Color: c}
			if strobe {
				s.FX = FXStrobe
			}
			raw[p][i] = s
		}
	}
	pv := &Preview{FPS: fps, Duration: dur, Positions: make([][]Sample, len(positions))}
	for p := range raw {
		pv.Positions[p] = downsample(raw[p], previewSub, rate)
	}
	return pv
}

// downsample junta sub subamostras por amostra (cor média). Onde a cor troca
// mais de MaxPreviewChanges vezes em 1 s (janela centrada), a amostra vira a
// média da janela inteira, marcada com FXFast.
func downsample(raw []Sample, sub, rate int) []Sample {
	n := len(raw) / sub
	// Somas acumuladas: trocas e canais, para médias de qualquer trecho.
	changes := make([]int, len(raw)+1)
	var sum [3][]int
	for c := range sum {
		sum[c] = make([]int, len(raw)+1)
	}
	for i, s := range raw {
		ch := 0
		if i > 0 && differs(raw[i-1].Color, s.Color) {
			ch = 1
		}
		changes[i+1] = changes[i] + ch
		sum[0][i+1] = sum[0][i] + int(s.Color.R)
		sum[1][i+1] = sum[1][i] + int(s.Color.G)
		sum[2][i+1] = sum[2][i] + int(s.Color.B)
	}
	mean := func(a, b int) fixture.Color {
		k := b - a
		avg := func(c int) uint8 { return uint8((sum[c][b] - sum[c][a] + k/2) / k) }
		return fixture.Color{R: avg(0), G: avg(1), B: avg(2)}
	}
	out := make([]Sample, n)
	half := rate / 2 // subamostras em 0,5 s
	for j := range out {
		a, b := j*sub, (j+1)*sub
		s := Sample{Color: mean(a, b)}
		for _, r := range raw[a:b] {
			s.FX |= r.FX
		}
		wa, wb := max(0, a+sub/2-half), min(len(raw), a+sub/2+half)
		if changes[wb]-changes[wa+1] > MaxPreviewChanges {
			s.Color = mean(wa, wb)
			s.FX |= FXFast
		}
		out[j] = s
	}
	return out
}

func differs(a, b fixture.Color) bool {
	d := func(x, y uint8) bool { return max(x, y)-min(x, y) > changeThreshold }
	return d(a.R, b.R) || d(a.G, b.G) || d(a.B, b.B)
}
