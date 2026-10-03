// Package analysis analisa uma faixa (BPM, batidas, compassos, energia,
// partes) e gera a timeline de luz da música (README.md, "Formato da
// timeline"), com um LLM opcional via OpenRouter para nomear as partes e
// escolher os presets.
//
// É o porte em Go do antigo scripts/analisar.py (librosa), para o programa
// rodar no Mac sem Python. Os parâmetros seguem os do librosa.
package analysis

import (
	"encoding/binary"
	"math"
	"slices"

	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/show"
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
func Analyze(y []float64) show.Analysis { return AnalyzeDrums(y, nil) }

// AnalyzeDrums analisa o sinal mono y a 22.050 Hz; drums, se não for nil, é
// a bateria da mesma música separada dos outros instrumentos (mesma taxa),
// e a batida passa a ser ouvida nela (drums.go).
func AnalyzeDrums(y, drums []float64) show.Analysis {
	duration := float64(len(y)) / SR
	S := stftMag(y)
	frames := S.cols
	harm, perc := hpss(S)

	onset := onsetStrength(melDB(perc, melFilters(128, 0, SR/2)))
	bpm := tempo(onset)
	beats := beatTrack(onset, bpm)
	beatTime := func(b int) float64 { return float64(b) * hop / SR }

	r := rms(y, frames)
	var ear *drumEar
	if drums != nil {
		ear = hearDrums(drums, frames, percentile(r, 95))
	}

	// Compasso (4/4): a fase de 4 batidas com mais grave (bumbo) é o "1".
	low := onsetStrength(melDB(perc, melFilters(32, 0, 200)))
	phase, bestScore := 0, math.Inf(-1)
	kickAt := func(frame int) float64 { return low[frame] }
	if ear != nil && percentile(ear.kick, 99) > 0 { // com a bateria separada, o bumbo dela
		kickAt = func(frame int) float64 { return around(ear.kick, frame, 1) }
	}
	for p := 0; p < 4 && p < len(beats); p++ {
		var sum float64
		var n int
		for i := p; i < len(beats); i += 4 {
			sum += kickAt(beats[i])
			n++
		}
		if n > 0 && sum/float64(n) > bestScore {
			phase, bestScore = p, sum/float64(n)
		}
	}

	// Energia: RMS normalizado (5º–95º percentil) a cada 0,5 s e por quadro.
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
	a.Energy.Hop = round(float64(step)*hop/SR, 5) // 22 quadros = 0,51 s (com 0,5 a curva encolhia 2% e o fim da música adiantava)
	a.Energy.Values = curve
	// Fim do som: o último quadro ainda audível (50 dB abaixo do nível alto da
	// música) e, antes dele, quanto dura a cauda (desde o último quadro a
	// menos de 20 dB do nível alto), em que a luz apaga aos poucos.
	a.SoundEnd, a.FadeOut = soundEnd(r, duration)
	for i, b := range beats {
		a.Beats = append(a.Beats, round(beatTime(b), 3))
		if i >= phase && (i-phase)%4 == 0 {
			a.Downbeats = append(a.Downbeats, round(beatTime(b), 3))
		}
	}
	// Toques: ataques na música inteira (bateria, guitarra, entradas), não só
	// na parte percussiva.
	mixEnv := onsetStrength(melDB(S, melFilters(128, 0, SR/2)))
	var strength []float64 // força percussiva de cada toque (sem a bateria separada, na escala da curva)
	if ear != nil {
		var kinds []byte
		a.Hits, kinds, strength = pickHitsDrums(ear, mixEnv, onset, a.Beats, a.BPM)
		a.Drums, a.HitKind, a.HitStrength = true, string(kinds), strength
		a.Accents = pickAccentsDrums(a.Hits, kinds, strength, ear.beatLevels(a.Beats, 60/a.BPM), a.Beats, 60/a.BPM)
	} else {
		a.Hits = pickHits(mixEnv, a.Beats, a.BPM)
		strength = hitStrengths(onset, a.Hits) // percussão em cada toque
		if len(strength) > 0 && percentile(strength, 90) > 0 {
			top := percentile(strength, 90)
			for _, v := range strength {
				a.HitStrength = append(a.HitStrength, round(math.Min(1, v/top), 2))
			}
			a.Accents = pickAccents(a.Hits, strength, 60/a.BPM)
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
	// A batida: divide a parte onde a bateria entra ou sai e mede a de cada uma.
	energyOf := func(start, end float64) float64 {
		fa, fb := int(start*SR/hop), min(int(end*SR/hop), frames)
		return mean(norm[fa:max(fb, fa+1)])
	}
	if ear != nil { // pelo nível da bateria em cada batida
		levels := ear.beatLevels(a.Beats, 60/a.BPM)
		floor := make([]float64, len(levels)) // com um piso, para comparar com os trechos sem bateria
		for i, v := range levels {
			floor[i] = v + 0.02
		}
		snapToDrums(a.Sections, levels, a.Beats, 60/a.BPM)
		for i := range a.Sections {
			s := &a.Sections[i]
			lo, _ := slices.BinarySearch(a.Beats, s.Start)
			hi, _ := slices.BinarySearch(a.Beats, s.End)
			s.Bars, s.Energy = int(math.RoundToEven(float64(hi-lo)/4)), round(energyOf(s.Start, s.End), 3)
		}
		a.Sections = splitByPulse(a.Sections, a.Beats, floor, a.Beats, 60/a.BPM, energyOf)
		setPulseDrums(a.Sections, a.Beats, levels)
	} else { // pela força percussiva dos toques
		a.Sections = splitByPulse(a.Sections, a.Hits, strength, a.Beats, 60/a.BPM, energyOf)
		setPulse(a.Sections, a.Hits, strength)
	}
	return a
}

// soundEnd devolve onde o som acaba (s) e a duração da cauda final (s, até
// maxFadeOut), a partir do RMS por quadro.
func soundEnd(r []float64, duration float64) (end, fade float64) {
	const maxFadeOut = 12.0
	loud := percentile(r, 95)
	last := func(level float64) float64 {
		for i := len(r) - 1; i >= 0; i-- {
			if r[i] > loud*level {
				return math.Min(duration, float64(i+1)*hop/SR)
			}
		}
		return 0
	}
	end = duration
	if loud > 0 {
		end = last(0.003)
	}
	fade = math.Min(maxFadeOut, math.Max(0, end-last(0.1)))
	return round(end, 3), round(fade, 3)
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

// minHitGap é o intervalo mínimo entre dois toques (o passo mais curto de um
// efeito: menos de 3 trocas por segundo).
var minHitGap = show.MinEffectStep.Seconds()

// pickHits acha os toques fortes da música na curva de ataques env (um valor
// por quadro): picos locais acima da média da vizinhança. Entre dois toques
// a menos de minHitGap, fica o mais forte. Onde a música passa mais de 4
// batidas sem toque nenhum (trecho parado), entram batidas da grade, de 2 em
// 2, para a luz não congelar.
func pickHits(env, beats []float64, bpm float64) []float64 {
	const (
		peakWin = 3    // quadros de cada lado em que o pico é o maior (~70 ms)
		avgWin  = 11   // quadros de cada lado para a média local (~0,25 s)
		delta   = 0.06 // quanto acima da média local, em relação ao maior ataque
	)
	top := 0.0
	for _, v := range env {
		top = math.Max(top, v)
	}
	if top <= 0 {
		return nil
	}
	type peak struct{ t, v float64 }
	var cand []peak
	for i, v := range env {
		if v < mean(env[max(0, i-avgWin):min(len(env), i+avgWin+1)])+delta*top {
			continue
		}
		if v < slices.Max(env[max(0, i-peakWin):min(len(env), i+peakWin+1)]) {
			continue
		}
		cand = append(cand, peak{float64(i) * hop / SR, v})
	}
	slices.SortFunc(cand, func(a, b peak) int { return -cmpF(a.v, b.v) }) // os mais fortes escolhem primeiro
	var hits []float64
	free := func(t, gap float64) bool {
		for _, h := range hits {
			if math.Abs(h-t) < gap {
				return false
			}
		}
		return true
	}
	for _, c := range cand {
		if free(c.t, minHitGap) {
			hits = append(hits, c.t)
		}
	}
	if bpm > 0 {
		beat := 60 / bpm
		for _, b := range beats {
			if free(b, math.Max(2*beat*0.99, minHitGap)) {
				hits = append(hits, b)
			}
		}
	}
	slices.Sort(hits)
	for i := range hits {
		hits[i] = round(hits[i], 3)
	}
	return hits
}
