// Package analysis analisa uma faixa (BPM, batidas, compassos, energia,
// partes) e gera a timeline de luz da música (TIMELINE.md), com um LLM
// opcional via OpenRouter para nomear as partes e escolher os presets.
//
// É o porte em Go do antigo scripts/analisar.py (librosa), para o programa
// rodar no Mac sem Python. Os parâmetros seguem os do librosa.
package analysis

import (
	"encoding/binary"
	"math"
	"slices"

	"luzes/internal/audio"
	"luzes/internal/show"
)

// Mono converte uma faixa do player (16 bits, estéreo, 44,1 kHz) para mono
// a 22.050 Hz, com um passa-baixas antes de dividir a taxa por 2.
func Mono(t *audio.Track) []float64 {
	frames := len(t.PCM) / 4
	x := make([]float64, frames)
	for i := range x {
		l := float64(int16(binary.LittleEndian.Uint16(t.PCM[i*4:])))
		r := float64(int16(binary.LittleEndian.Uint16(t.PCM[i*4+2:])))
		x[i] = (l + r) / 2 / 32768
	}
	// FIR passa-baixas (sinc com janela de Hann, corte em 0,45 da nova Nyquist).
	const taps = 63
	h := make([]float64, taps)
	var sum float64
	for i := range h {
		n := float64(i - taps/2)
		fc := 0.45 / 2 // relativo à taxa original
		v := 2 * fc
		if n != 0 {
			v = math.Sin(2*math.Pi*fc*n) / (math.Pi * n)
		}
		h[i] = v * (0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(taps-1)))
		sum += h[i]
	}
	y := make([]float64, frames/2)
	for j := range y {
		c := 2 * j
		var acc float64
		for i, w := range h {
			if k := c + i - taps/2; k >= 0 && k < frames {
				acc += w * x[k]
			}
		}
		y[j] = acc / sum
	}
	return y
}

// Analyze analisa o sinal mono a 22.050 Hz.
func Analyze(y []float64) show.Analysis {
	duration := float64(len(y)) / SR
	S := stftMag(y)
	frames := S.cols
	harm, perc := hpss(S)

	onset := onsetStrength(melDB(perc, melFilters(128, 0, SR/2)))
	bpm := tempo(onset)
	beats := beatTrack(onset, bpm)
	beatTime := func(b int) float64 { return float64(b) * hop / SR }

	// Compasso (4/4): a fase de 4 batidas com mais grave (bumbo) é o "1".
	low := onsetStrength(melDB(perc, melFilters(32, 0, 200)))
	phase, bestScore := 0, math.Inf(-1)
	for p := 0; p < 4 && p < len(beats); p++ {
		var sum float64
		var n int
		for i := p; i < len(beats); i += 4 {
			sum += low[beats[i]]
			n++
		}
		if n > 0 && sum/float64(n) > bestScore {
			phase, bestScore = p, sum/float64(n)
		}
	}

	// Energia: RMS normalizado (5º–95º percentil) a cada 0,5 s e por quadro.
	r := rms(y, frames)
	lo, hi := percentile(r, 5), percentile(r, 95)
	norm := make([]float64, frames)
	for i, v := range r {
		norm[i] = math.Min(1, math.Max(0, (v-lo)/math.Max(hi-lo, 1e-9)))
	}
	step := int(math.Round(0.5 * SR / hop))
	var curve []float64
	for i := 0; i < frames; i += step {
		curve = append(curve, round(mean(norm[i:min(i+step, frames)]), 3))
	}

	a := show.Analysis{Duration: round(duration, 3), BPM: round(refineBPM(beats, bpm), 2)}
	a.Energy.Hop = 0.5
	a.Energy.Values = curve
	for i, b := range beats {
		a.Beats = append(a.Beats, round(beatTime(b), 3))
		if i >= phase && (i-phase)%4 == 0 {
			a.Downbeats = append(a.Downbeats, round(beatTime(b), 3))
		}
	}
	if len(beats) < 8 {
		a.Sections = []show.Section{{Start: 0, End: a.Duration, Energy: round(mean(norm), 3), Group: "A"}}
		return a
	}

	// Partes: timbre + harmonia + energia por batida, agrupadas.
	nm := newMatrix(1, frames)
	for i, v := range norm {
		nm.v[i] = float32(v)
	}
	nz := zscore(nm)
	for i := range nz.v {
		nz.v[i] *= 2 // a energia pesa o dobro
	}
	sync := beatSync([]*matrix{zscore(chroma(harm)), zscore(mfcc(melDB(S, melFilters(128, 0, SR/2)), 13)), nz}, beats, frames)
	k := int(math.Max(4, math.Min(12, math.Round(duration/22))))
	bounds := agglomerative(sync, k)

	// Encaixa os limites no compasso mais próximo e junta partes curtas.
	var downIdx []int
	for i := phase; i < len(beats); i += 4 {
		downIdx = append(downIdx, i)
	}
	snapped := []int{0}
	for _, b := range bounds {
		if b == 0 {
			continue
		}
		best := downIdx[0]
		for _, d := range downIdx {
			if abs(d-b) < abs(best-b) {
				best = d
			}
		}
		snapped = append(snapped, best)
	}
	slices.Sort(snapped)
	snapped = slices.Compact(snapped)
	const minBeats = 16 // 4 compassos
	merged := []int{snapped[0]}
	for _, b := range snapped[1:] {
		if b-merged[len(merged)-1] >= minBeats {
			merged = append(merged, b)
		}
	}
	if len(beats)-merged[len(merged)-1] < minBeats && len(merged) > 1 {
		merged = merged[:len(merged)-1]
	}

	times := []float64{0}
	for _, b := range merged[1:] {
		times = append(times, beatTime(beats[b]))
	}
	times = append(times, duration)
	var vecs [][]float64
	for i := 0; i+1 < len(times); i++ {
		start, end := times[i], times[i+1]
		fa, fb := int(start*SR/hop), min(int(end*SR/hop), frames)
		var segBeats []int
		for j, b := range beats {
			if t := beatTime(b); t >= start && t < end {
				segBeats = append(segBeats, j)
			}
		}
		vec := make([]float64, len(sync[0]))
		for _, j := range segBeats {
			for d, v := range sync[j] {
				vec[d] += v / float64(len(segBeats))
			}
		}
		vecs = append(vecs, vec)
		a.Sections = append(a.Sections, show.Section{
			Start:  round(start, 3),
			End:    round(end, 3),
			Bars:   int(math.RoundToEven(float64(len(segBeats)) / 4)),
			Energy: round(mean(norm[fa:max(fb, fa+1)]), 3),
		})
	}
	for i, l := range labelRepeats(vecs, 0.2) {
		a.Sections[i].Group = l
	}
	return a
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(x*p) / p
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// refineBPM ajusta uma reta aos tempos das batidas: o tempograma só enxerga
// andamentos em passos de quadro (perto de 120 BPM: 117,45 ou 123,05), e as
// sequências são aceleradas pelo BPM — 2% de erro viram ~0,6 s em 30 s.
func refineBPM(beats []int, coarse float64) float64 {
	n := float64(len(beats))
	if n < 8 {
		return coarse
	}
	var sx, sy, sxx, sxy float64
	for i, b := range beats {
		x, y := float64(i), float64(b)*hop/SR
		sx, sy, sxx, sxy = sx+x, sy+y, sxx+x*x, sxy+x*y
	}
	slope := (n*sxy - sx*sy) / (n*sxx - sx*sx) // segundos por batida
	if bpm := 60 / slope; slope > 0 && math.Abs(bpm-coarse)/coarse < 0.1 {
		return bpm
	}
	return coarse
}
