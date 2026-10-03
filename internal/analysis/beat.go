package analysis

import (
	"math"
	"slices"

	"gonum.org/v1/gonum/dsp/fourier"
)

// tempo estima o andamento global (BPM) do envelope de ataques: média do
// tempograma (autocorrelação em janelas de 8 s) ponderada por uma
// preferência log-normal em torno de 120 BPM (librosa.feature.tempo).
func tempo(env []float64) float64 {
	const startBPM, stdBPM, maxTempo = 120.0, 1.0, 320.0
	win := int(math.Floor(8.0 * SR / hop)) // time_to_frames(8 s)
	half := win / 2

	// Bordas com rampa linear até zero (np.pad mode='linear_ramp').
	n := len(env)
	padded := make([]float64, n+2*half)
	copy(padded[half:], env)
	if n > 0 {
		for i := 0; i < half; i++ {
			frac := float64(i) / float64(half)
			padded[i] = env[0] * frac
			padded[len(padded)-1-i] = env[n-1] * frac
		}
	}

	size := 1
	for size < 2*win {
		size *= 2
	}
	fft := fourier.NewFFT(size)
	w := hann(win)
	acc := make([]float64, win)
	buf := make([]float64, size)
	coef := make([]complex128, size/2+1)
	ac := make([]float64, size)
	frames := 0
	// Média sobre os quadros; um a cada 4 basta para a média global.
	for t := 0; t+win <= len(padded); t += 4 {
		for i := range buf {
			buf[i] = 0
		}
		for i := 0; i < win; i++ {
			buf[i] = padded[t+i] * w[i]
		}
		fft.Coefficients(coef, buf)
		for i, c := range coef {
			coef[i] = complex(real(c)*real(c)+imag(c)*imag(c), 0)
		}
		fft.Sequence(ac, coef)
		frames++ // quadros sem energia contam como zero na média (como no librosa)
		if ac[0] <= 0 {
			continue
		}
		for lag := 0; lag < win; lag++ {
			acc[lag] += ac[lag] / ac[0] // norm=inf: o máximo é o lag 0
		}
	}
	if frames == 0 {
		return startBPM
	}

	best, bestScore := startBPM, math.Inf(-1)
	for lag := 1; lag < win; lag++ {
		bpm := 60.0 * SR / (hop * float64(lag))
		if bpm >= maxTempo {
			continue
		}
		prior := -0.5 * math.Pow((math.Log2(bpm)-math.Log2(startBPM))/stdBPM, 2)
		score := math.Log1p(1e6*acc[lag]/float64(frames)) + prior
		if score > bestScore {
			best, bestScore = bpm, score
		}
	}
	return best
}

// beatTrack encontra as batidas (quadros) por programação dinâmica
// (Ellis 2007; librosa.beat.beat_track com tightness=100, trim=False).
func beatTrack(env []float64, bpm float64) []int {
	n := len(env)
	if n == 0 || slices.Max(env) == 0 {
		return nil
	}
	const tightness = 100.0
	period := math.Round(SR / hop * 60 / bpm)

	// Normaliza pelo desvio padrão (ddof=1).
	var mean, sq float64
	for _, v := range env {
		mean += v
	}
	mean /= float64(n)
	for _, v := range env {
		sq += (v - mean) * (v - mean)
	}
	norm := math.Sqrt(sq/float64(max(n-1, 1))) + 1e-300
	on := make([]float64, n)
	for i, v := range env {
		on[i] = v / norm
	}

	// Pontuação local: convolução 'same' com uma gaussiana de ±1 período.
	p := int(period)
	win := make([]float64, 2*p+1)
	for i := range win {
		x := float64(i-p) * 32 / period
		win[i] = math.Exp(-0.5 * x * x)
	}
	K := len(win)
	local := make([]float64, n)
	for i := range local {
		for k := max(0, i+K/2-n+1); k < min(i+K/2, K); k++ {
			local[i] += win[k] * on[i+K/2-k]
		}
	}

	// Programação dinâmica.
	thresh := 0.01 * slices.Max(local)
	back := make([]int, n)
	cum := make([]float64, n)
	first := true
	for i, s := range local {
		best, loc := math.Inf(-1), -1
		for l := i - int(math.Round(period/2)); l > int(float64(i)-2*period-1); l-- {
			if l < 0 {
				break
			}
			d := math.Log(float64(i-l)) - math.Log(period)
			if sc := cum[l] - tightness*d*d; sc > best {
				best, loc = sc, l
			}
		}
		if loc >= 0 {
			cum[i] = s + best
		} else {
			cum[i] = s
		}
		if first && s < thresh {
			back[i] = -1
		} else {
			back[i] = loc
			first = false
		}
	}

	// Última batida: o último máximo local acima de metade da mediana dos máximos.
	isMax := func(i int) bool {
		prev, next := cum[max(i-1, 0)], cum[min(i+1, n-1)]
		return cum[i] > prev && cum[i] >= next
	}
	var maxima []float64
	for i := range cum {
		if isMax(i) {
			maxima = append(maxima, cum[i])
		}
	}
	tail := n - 1
	if len(maxima) > 0 {
		slices.Sort(maxima)
		med := maxima[len(maxima)/2]
		if len(maxima)%2 == 0 {
			med = (maxima[len(maxima)/2-1] + maxima[len(maxima)/2]) / 2
		}
		for i := n - 1; i >= 0; i-- {
			if isMax(i) && cum[i] >= 0.5*med {
				tail = i
				break
			}
		}
	}
	var beats []int
	for i := tail; i >= 0; i = back[i] {
		beats = append(beats, i)
	}
	slices.Reverse(beats)

	// trim=False ainda descarta batidas onde a pontuação local é ≤ 0.
	lo, hi := 0, n-1
	for lo < n && local[lo] <= 0 {
		lo++
	}
	for hi >= 0 && local[hi] <= 0 {
		hi--
	}
	return slices.DeleteFunc(beats, func(b int) bool { return b < lo || b > hi })
}
