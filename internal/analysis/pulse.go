package analysis

import (
	"math"
	"slices"

	"github.com/ncastellani/DMXController/internal/show"
)

// A batida dentro das partes (pedido da usuária: "no meio de um verso tem uma
// batida de bateria, ou alguma coisa de funk, eu queria que a luz
// acompanhasse essas nuances"). As partes vêm do timbre e da harmonia e a
// energia delas é volume; aqui entra a percussão: a força de cada toque, a
// "batida" de cada parte, a divisão de uma parte onde a bateria entra ou sai
// e os toques que se destacam dos vizinhos.

const (
	splitWin    = 8    // toques de cada lado comparados para dividir uma parte
	splitRatio  = 1.6  // quanto a percussão tem de mudar (mediana de um lado sobre a do outro)
	splitBeats  = 8    // batidas mínimas de cada pedaço
	accentRatio = 2.0  // quanto um toque tem de passar da mediana dos vizinhos para ser um destaque (com 1,6 saíam ~20 por música e a usuária viu "muito ruído")
	accentWin   = 8    // vizinhos de cada lado
	accentGap   = 4.0  // batidas mínimas entre dois destaques
	accentFloor = 0.45 // força mínima (0 a 1) de um destaque
)

// hitStrengths devolve a força percussiva de cada toque: o pico da curva de
// ataques da percussão env em volta dele.
func hitStrengths(env, hits []float64) []float64 {
	out := make([]float64, len(hits))
	for k, h := range hits {
		i := int(h*SR/hop + 0.5)
		if lo, hi := max(0, i-2), min(len(env), i+3); lo < hi {
			out[k] = slices.Max(env[lo:hi])
		}
	}
	return out
}

func medianOf(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := slices.Clone(x)
	slices.Sort(s)
	return s[len(s)/2]
}

// splitPoint acha, entre os toques [lo, hi) de uma parte, onde a percussão
// muda de patamar (a bateria entra ou sai): o toque em que a mediana dos
// splitWin seguintes mais difere da dos splitWin anteriores, se a diferença
// passa de splitRatio e se sustenta no resto de cada lado. Devolve -1 se a
// parte é uniforme.
func splitPoint(hits, strength []float64, lo, hi int, beat float64) int {
	best, bestScore := -1, 0.0
	for k := lo + splitWin; k+splitWin <= hi; k++ {
		if hits[k]-hits[lo] < splitBeats*beat || hits[hi-1]-hits[k] < splitBeats*beat {
			continue
		}
		l, r := medianOf(strength[k-splitWin:k]), medianOf(strength[k:k+splitWin])
		if l <= 0 || r <= 0 {
			continue
		}
		// O patamar novo tem de valer para o lado inteiro, não só perto da troca.
		wl, wr := medianOf(strength[lo:k]), medianOf(strength[k:hi])
		if whole := math.Abs(math.Log(wr / math.Max(wl, 1e-9))); whole < math.Log(splitRatio)*0.6 {
			continue
		}
		if math.Abs(math.Log(r/l)) < math.Log(splitRatio) {
			continue
		}
		// As medianas dizem se há mudança; as médias dizem onde ela é mais nítida.
		if score := math.Abs(math.Log(mean(strength[k:k+splitWin]) / mean(strength[k-splitWin:k]))); score > bestScore {
			best, bestScore = k, score
		}
	}
	return best
}

// splitByPulse divide as partes em que a percussão muda de patamar no meio
// (no máximo em 4 pedaços). O pedaço maior fica com o grupo da parte; os
// outros ganham "+" (percussão bem mais forte que a dele) ou "-" (bem mais
// fraca), ou ficam no mesmo grupo se estão no mesmo patamar: um refrão em que
// a bateria some nos últimos compassos vira C e C-, e o refrão seguinte,
// inteiro, continua C. Pedaços vizinhos que acabam no mesmo grupo voltam a
// ser um só. hits e strength são os instantes e os valores comparados (os
// toques e a força deles, ou as batidas e o nível da bateria em cada uma);
// energy dá a energia (volume) de um trecho.
func splitByPulse(secs []show.Section, hits, strength, beats []float64, beat float64, energy func(start, end float64) float64) []show.Section {
	bars := func(start, end float64) int {
		n := 0
		for _, b := range beats {
			if b >= start && b < end {
				n++
			}
		}
		return int(math.RoundToEven(float64(n) / 4))
	}
	rng := func(start, end float64) (lo, hi int) {
		lo, _ = slices.BinarySearch(hits, start)
		hi, _ = slices.BinarySearch(hits, end)
		return lo, hi
	}
	level := func(s show.Section) float64 {
		lo, hi := rng(s.Start, s.End)
		return medianOf(strength[lo:hi])
	}
	var cut func(s show.Section, depth int) []show.Section
	cut = func(s show.Section, depth int) []show.Section {
		lo, hi := rng(s.Start, s.End)
		k := -1
		if depth < 2 {
			k = splitPoint(hits, strength, lo, hi, beat)
		}
		if k < 0 {
			return []show.Section{s}
		}
		at := nearest(beats, hits[k]) // a troca cai na batida
		if at <= s.Start || at >= s.End {
			return []show.Section{s}
		}
		a, b := s, s
		a.End, b.Start = at, at
		return append(cut(a, depth+1), cut(b, depth+1)...)
	}
	var out []show.Section
	for _, s := range secs {
		pieces := cut(s, 0)
		if len(pieces) == 1 {
			out = append(out, s)
			continue
		}
		big := 0
		for i, p := range pieces {
			if p.End-p.Start > pieces[big].End-pieces[big].Start {
				big = i
			}
		}
		ref := level(pieces[big])
		for i := range pieces {
			switch v := level(pieces[i]); {
			case i == big:
			case v > ref*splitRatio:
				pieces[i].Group += "+"
			case v*splitRatio < ref:
				pieces[i].Group += "-"
			}
		}
		first := len(out)
		for _, p := range pieces {
			if n := len(out); n > first && out[n-1].Group == p.Group {
				out[n-1].End = p.End
			} else {
				out = append(out, p)
			}
		}
		for i := first; i < len(out); i++ {
			out[i].Bars, out[i].Energy = bars(out[i].Start, out[i].End), round(energy(out[i].Start, out[i].End), 3)
		}
	}
	return out
}

// setPulse grava em cada parte a batida: a força típica dos toques dela, de
// 0 a 1 (1 = as partes de percussão mais forte da música).
func setPulse(secs []show.Section, hits, strength []float64) {
	if len(strength) == 0 {
		return
	}
	top := percentile(strength, 90)
	if top <= 0 {
		return
	}
	for i := range secs {
		lo, _ := slices.BinarySearch(hits, secs[i].Start)
		hi, _ := slices.BinarySearch(hits, secs[i].End)
		secs[i].Pulse = round(math.Min(1, medianOf(strength[lo:hi])/top), 2)
	}
}

// pickAccents acha os toques que se destacam dos vizinhos (uma virada de
// bateria, uma batida solta no meio do verso): bem acima da mediana dos
// accentWin toques de cada lado e fortes em relação à música. Entre dois a
// menos de accentGap batidas, fica o que mais se destaca.
func pickAccents(hits, strength []float64, beat float64) []show.Accent {
	if len(strength) == 0 {
		return nil
	}
	top := percentile(strength, 90)
	if top <= 0 {
		return nil
	}
	type cand struct {
		t, over float64
	}
	var cands []cand
	for k, s := range strength {
		around := append(slices.Clone(strength[max(0, k-accentWin):k]), strength[k+1:min(len(strength), k+1+accentWin)]...)
		med := medianOf(around)
		if med <= 0 || s < accentRatio*med || s/top < accentFloor {
			continue
		}
		cands = append(cands, cand{hits[k], s / med})
	}
	slices.SortFunc(cands, func(a, b cand) int { return -cmpF(a.over, b.over) })
	var out []show.Accent
	for _, c := range cands {
		if !slices.ContainsFunc(out, func(a show.Accent) bool { return math.Abs(a.T-c.t) < accentGap*beat }) {
			// 2× a mediana = flash de 0,6; 3× ou mais = cheio (1).
			out = append(out, show.Accent{T: c.t, V: round(math.Min(1, 0.6+0.4*(c.over-accentRatio)/(3-accentRatio)), 2)})
		}
	}
	slices.SortFunc(out, func(a, b show.Accent) int { return cmpF(a.T, b.T) })
	return out
}

// drive é o quanto a parte pede de agitação: o volume e a batida juntos (um
// verso só de beat e voz tem volume baixo e batida alta).
func drive(s show.Section) float64 {
	if s.Pulse <= 0 {
		return s.Energy
	}
	return (s.Energy + s.Pulse) / 2
}
