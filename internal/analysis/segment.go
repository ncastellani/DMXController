package analysis

import (
	"math"
	"slices"
)

// beatSync agrega as features por batida (mediana dos quadros de cada
// intervalo [batida i, batida i+1)). Devolve um vetor por batida.
func beatSync(feats []*matrix, beats []int, frames int) [][]float64 {
	dims := 0
	for _, f := range feats {
		dims += f.rows
	}
	out := make([][]float64, len(beats))
	vals := make([]float64, 0, 64)
	for i, b := range beats {
		end := frames
		if i+1 < len(beats) {
			end = beats[i+1]
		}
		vec := make([]float64, 0, dims)
		for _, f := range feats {
			for r := 0; r < f.rows; r++ {
				vals = vals[:0]
				for t := b; t < end; t++ {
					vals = append(vals, float64(f.at(r, t)))
				}
				vec = append(vec, medianF(vals))
			}
		}
		out[i] = vec
	}
	return out
}

func medianF(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// agglomerative agrupa batidas vizinhas em k segmentos por Ward com
// restrição temporal (só junta segmentos adjacentes), como
// librosa.segment.agglomerative. Devolve o índice inicial de cada segmento.
func agglomerative(data [][]float64, k int) []int {
	type seg struct {
		start, n int
		sum      []float64
	}
	segs := make([]seg, len(data))
	for i, v := range data {
		segs[i] = seg{start: i, n: 1, sum: slices.Clone(v)}
	}
	cost := func(a, b seg) float64 {
		var d float64
		for i := range a.sum {
			x := a.sum[i]/float64(a.n) - b.sum[i]/float64(b.n)
			d += x * x
		}
		return float64(a.n*b.n) / float64(a.n+b.n) * d
	}
	for len(segs) > max(k, 1) {
		best, bi := math.Inf(1), 0
		for i := 0; i+1 < len(segs); i++ {
			if c := cost(segs[i], segs[i+1]); c < best {
				best, bi = c, i
			}
		}
		a, b := segs[bi], segs[bi+1]
		for i := range a.sum {
			a.sum[i] += b.sum[i]
		}
		a.n += b.n
		segs[bi] = a
		segs = slices.Delete(segs, bi+1, bi+2)
	}
	starts := make([]int, len(segs))
	for i, s := range segs {
		starts[i] = s.start
	}
	return starts
}

// labelRepeats dá a mesma letra (A, B, C...) a partes parecidas (distância
// de cosseno < thresh), para refrões repetidos receberem a mesma luz.
func labelRepeats(vecs [][]float64, thresh float64) []string {
	labels := make([]string, len(vecs))
	used := 0
	for i, v := range vecs {
		best, bd := "", thresh
		for j := 0; j < i; j++ {
			if d := 1 - cosine(vecs[j], v); d < bd {
				best, bd = labels[j], d
			}
		}
		if best == "" {
			best = string(rune('A' + used))
			used++
		}
		labels[i] = best
	}
	return labels
}

func cosine(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	return dot / (math.Sqrt(na)*math.Sqrt(nb) + 1e-9)
}
