package analysis

import (
	"math"
	"math/cmplx"
	"runtime"
	"slices"
	"sync"

	"gonum.org/v1/gonum/dsp/fourier"
)

// Parâmetros iguais aos do librosa (scripts/analisar.py, versão original).
const (
	SR   = 22050
	nFFT = 2048
	hop  = 512
	nBin = nFFT/2 + 1
)

// matrix é uma matriz bins × quadros em float32 (economiza memória: uma
// música de 4 min tem ~11 mil quadros).
type matrix struct {
	rows, cols int
	v          []float32
}

func newMatrix(rows, cols int) *matrix {
	return &matrix{rows: rows, cols: cols, v: make([]float32, rows*cols)}
}

func (m *matrix) at(r, c int) float32     { return m.v[r*m.cols+c] }
func (m *matrix) set(r, c int, x float32) { m.v[r*m.cols+c] = x }
func (m *matrix) row(r int) []float32     { return m.v[r*m.cols : (r+1)*m.cols] }

// parallel divide [0, n) entre os núcleos.
func parallel(n int, f func(lo, hi int)) {
	workers := min(runtime.NumCPU(), n)
	if workers <= 1 {
		f(0, n)
		return
	}
	var wg sync.WaitGroup
	chunk := (n + workers - 1) / workers
	for lo := 0; lo < n; lo += chunk {
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			f(lo, hi)
		}(lo, min(lo+chunk, n))
	}
	wg.Wait()
}

// hann é a janela de Hann periódica (scipy get_window('hann', n, fftbins=True)).
func hann(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	return w
}

// stftMag calcula |STFT| (center=True com zeros nas bordas, como o librosa).
func stftMag(y []float64) *matrix {
	pad := nFFT / 2
	padded := make([]float64, len(y)+2*pad)
	copy(padded[pad:], y)
	frames := 1 + len(y)/hop
	S := newMatrix(nBin, frames)
	win := hann(nFFT)
	parallel(frames, func(lo, hi int) {
		fft := fourier.NewFFT(nFFT)
		buf := make([]float64, nFFT)
		coef := make([]complex128, nBin)
		for t := lo; t < hi; t++ {
			start := t * hop
			for i := range buf {
				if start+i < len(padded) {
					buf[i] = padded[start+i] * win[i]
				} else {
					buf[i] = 0
				}
			}
			fft.Coefficients(coef, buf)
			for k, c := range coef {
				S.set(k, t, float32(cmplx.Abs(c)))
			}
		}
	})
	return S
}

// medianFilter aplica mediana de tamanho k ao longo das linhas (time=true)
// ou das colunas, com bordas refletidas (scipy mode='reflect').
func medianFilter(S *matrix, k int, time bool) *matrix {
	out := newMatrix(S.rows, S.cols)
	half := k / 2
	reflect := func(i, n int) int {
		for i < 0 || i >= n {
			if i < 0 {
				i = -i - 1
			}
			if i >= n {
				i = 2*n - i - 1
			}
		}
		return i
	}
	if time {
		parallel(S.rows, func(lo, hi int) {
			w := make([]float32, k)
			for r := lo; r < hi; r++ {
				row := S.row(r)
				for c := 0; c < S.cols; c++ {
					for j := range w {
						w[j] = row[reflect(c-half+j, S.cols)]
					}
					out.set(r, c, median(w))
				}
			}
		})
		return out
	}
	parallel(S.cols, func(lo, hi int) {
		w := make([]float32, k)
		for c := lo; c < hi; c++ {
			for r := 0; r < S.rows; r++ {
				for j := range w {
					w[j] = S.at(reflect(r-half+j, S.rows), c)
				}
				out.set(r, c, median(w))
			}
		}
	})
	return out
}

func median(w []float32) float32 {
	slices.Sort(w)
	return w[len(w)/2]
}

// hpss separa o espectro em harmônico e percussivo (máscaras suaves de
// potência 2, kernel 31 — librosa.decompose.hpss).
func hpss(S *matrix) (harm, perc *matrix) {
	h := medianFilter(S, 31, true)
	p := medianFilter(S, 31, false)
	harm, perc = newMatrix(S.rows, S.cols), newMatrix(S.rows, S.cols)
	for i, s := range S.v {
		hh, pp := float64(h.v[i])*float64(h.v[i]), float64(p.v[i])*float64(p.v[i])
		z := hh + pp
		if z < 1e-20 {
			// librosa: sem energia nos dois, divide igualmente
			harm.v[i], perc.v[i] = s/2, s/2
			continue
		}
		harm.v[i] = float32(float64(s) * hh / z)
		perc.v[i] = float32(float64(s) * pp / z)
	}
	return harm, perc
}

// Escala mel de Slaney (librosa htk=False).
func hzToMel(f float64) float64 {
	const fsp, minLogHz, minLogMel = 200.0 / 3, 1000.0, 1000.0 / (200.0 / 3)
	logstep := math.Log(6.4) / 27
	if f >= minLogHz {
		return minLogMel + math.Log(f/minLogHz)/logstep
	}
	return f / fsp
}

func melToHz(m float64) float64 {
	const fsp, minLogHz, minLogMel = 200.0 / 3, 1000.0, 1000.0 / (200.0 / 3)
	logstep := math.Log(6.4) / 27
	if m >= minLogMel {
		return minLogHz * math.Exp(logstep*(m-minLogMel))
	}
	return fsp * m
}

// melFilters devolve o banco de filtros mel (n × nBin), normalizado à Slaney.
func melFilters(n int, fmin, fmax float64) [][]float64 {
	mels := make([]float64, n+2)
	lo, hi := hzToMel(fmin), hzToMel(fmax)
	for i := range mels {
		mels[i] = melToHz(lo + (hi-lo)*float64(i)/float64(n+1))
	}
	w := make([][]float64, n)
	for i := range w {
		w[i] = make([]float64, nBin)
		enorm := 2 / (mels[i+2] - mels[i])
		for k := range w[i] {
			f := float64(k) * SR / nFFT
			lower := (f - mels[i]) / (mels[i+1] - mels[i])
			upper := (mels[i+2] - f) / (mels[i+2] - mels[i+1])
			w[i][k] = math.Max(0, math.Min(lower, upper)) * enorm
		}
	}
	return w
}

// melDB calcula o espectrograma mel de potência em dB (ref=1, amin=1e-10,
// top_db=80), a partir da magnitude S.
func melDB(S *matrix, filters [][]float64) *matrix {
	M := newMatrix(len(filters), S.cols)
	parallel(S.cols, func(lo, hi int) {
		for t := lo; t < hi; t++ {
			for m, fw := range filters {
				var sum float64
				for k, w := range fw {
					if w != 0 {
						a := float64(S.at(k, t))
						sum += w * a * a
					}
				}
				M.set(m, t, float32(10*math.Log10(math.Max(1e-10, sum))))
			}
		}
	})
	top := float32(math.Inf(-1))
	for _, v := range M.v {
		top = max(top, v)
	}
	for i, v := range M.v {
		M.v[i] = max(v, top-80)
	}
	return M
}

// onsetStrength é o fluxo espectral positivo médio sobre o mel em dB, com o
// atraso de enquadramento compensado (librosa.onset.onset_strength).
func onsetStrength(M *matrix) []float64 {
	n := M.cols
	env := make([]float64, n)
	const padWidth = 1 + nFFT/(2*hop) // lag + n_fft/(2·hop)
	for t := 1; t < n; t++ {
		var sum float64
		for m := 0; m < M.rows; m++ {
			if d := float64(M.at(m, t) - M.at(m, t-1)); d > 0 {
				sum += d
			}
		}
		if i := t - 1 + padWidth; i < n {
			env[i] = sum / float64(M.rows)
		}
	}
	return env
}

// rms é a energia RMS por quadro do sinal (frame 2048, hop 512, center).
func rms(y []float64, frames int) []float64 {
	out := make([]float64, frames)
	for t := range out {
		start := t*hop - nFFT/2
		var sum float64
		for i := start; i < start+nFFT; i++ {
			if i >= 0 && i < len(y) {
				sum += y[i] * y[i]
			}
		}
		out[t] = math.Sqrt(sum / nFFT)
	}
	return out
}

// chroma soma a potência de cada bin na classe de altura (Dó = 0), com
// peso gaussiano em torno da 5ª oitava, normalizado pelo máximo do quadro.
// (O script original usava chroma_cqt; aqui basta para comparar partes.)
func chroma(S *matrix) *matrix {
	class := make([]int, nBin)
	weight := make([]float64, nBin)
	for k := range class {
		f := float64(k) * SR / nFFT
		if f < 30 || f > 5000 {
			class[k] = -1
			continue
		}
		pitch := 12*math.Log2(f/440) + 69
		class[k] = ((int(math.Round(pitch)) % 12) + 12) % 12
		oct := pitch/12 - 1
		weight[k] = math.Exp(-0.5 * math.Pow((oct-5)/2, 2))
	}
	C := newMatrix(12, S.cols)
	for t := 0; t < S.cols; t++ {
		var col [12]float64
		for k := range class {
			if class[k] >= 0 {
				a := float64(S.at(k, t))
				col[class[k]] += weight[k] * a * a
			}
		}
		mx := slices.Max(col[:])
		for c, v := range col {
			if mx > 0 {
				v /= mx
			}
			C.set(c, t, float32(v))
		}
	}
	return C
}

// mfcc aplica DCT-II ortonormal ao mel em dB e devolve os n primeiros.
func mfcc(M *matrix, n int) *matrix {
	N := M.rows
	basis := make([][]float64, n)
	for k := range basis {
		basis[k] = make([]float64, N)
		scale := math.Sqrt(2 / float64(N))
		if k == 0 {
			scale = math.Sqrt(1 / float64(N))
		}
		for i := range basis[k] {
			basis[k][i] = scale * math.Cos(math.Pi/float64(N)*(float64(i)+0.5)*float64(k))
		}
	}
	out := newMatrix(n, M.cols)
	for t := 0; t < M.cols; t++ {
		for k, b := range basis {
			var sum float64
			for i, w := range b {
				sum += w * float64(M.at(i, t))
			}
			out.set(k, t, float32(sum))
		}
	}
	return out
}

// zscore normaliza cada linha (média 0, desvio 1).
func zscore(m *matrix) *matrix {
	out := newMatrix(m.rows, m.cols)
	for r := 0; r < m.rows; r++ {
		row := m.row(r)
		var mean, sq float64
		for _, v := range row {
			mean += float64(v)
		}
		mean /= float64(len(row))
		for _, v := range row {
			sq += (float64(v) - mean) * (float64(v) - mean)
		}
		std := math.Sqrt(sq/float64(len(row))) + 1e-9
		for c, v := range row {
			out.set(r, c, float32((float64(v)-mean)/std))
		}
	}
	return out
}

// percentile com interpolação linear (numpy default).
func percentile(x []float64, p float64) float64 {
	s := slices.Clone(x)
	slices.Sort(s)
	pos := p / 100 * float64(len(s)-1)
	i := int(pos)
	if i+1 >= len(s) {
		return s[len(s)-1]
	}
	return s[i] + (s[i+1]-s[i])*(pos-float64(i))
}
