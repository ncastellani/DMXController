package analysis

import (
	"math"
	"slices"

	"github.com/ncastellani/DMXController/internal/show"
)

// Com a bateria separada da música (Separator), a análise ouve a batida de
// verdade: os toques passam a ser os da bateria (bumbo e caixa mandam; o
// chimbal e os ataques dos outros instrumentos só preenchem os buracos), cada
// toque sabe que peça tocou, a "batida" de cada parte é o nível real da
// bateria (zero onde ela não toca) e as partes se dividem onde ela entra ou
// sai. Onde não há bateria, valem os ataques da música inteira, como antes.

// Tipos de toque (Analysis.HitKind, um caractere por toque).
const (
	kindKick  = 'k' // bumbo
	kindSnare = 's' // caixa, palma
	kindHat   = 'h' // chimbal, prato, percussão aguda
	kindOther = '-' // não é bateria: ataque de outro instrumento ou batida da grade
)

const (
	drumPeak    = 0.15 // um ataque da bateria conta a partir dessa fração do nível alto
	drumAudible = 0.15 // ...e só onde a bateria soa (fração do volume alto dela): fora disso é resto de outros instrumentos
	drumSilent  = 0.03 // bateria com menos que isso do volume da música = música sem bateria
	fillerGap   = 0.9  // perto da bateria, um toque de preenchimento precisa de quase 1 batida livre de cada lado
	loneFlash   = 0.5  // força mínima de um toque de bateria solto (sem bateria em volta) para virar pico
)

// drumEar é o que se ouve na bateria separada. As curvas têm um valor por
// quadro, do tamanho das da música.
type drumEar struct {
	onset            []float64 // ataques (todas as bandas), 1 = nível alto
	kick, snare, hat []float64 // ataques por banda, 1 = nível alto da banda
	rms              []float64 // volume, 1 = nível alto
}

// hearDrums prepara as curvas da bateria d para uma música de frames quadros
// cujo volume alto (95º percentil do RMS) é mixLoud. Uma bateria quase muda
// (música sem bateria) dá curvas zeradas.
func hearDrums(d []float64, frames int, mixLoud float64) *drumEar {
	fit := func(x []float64) []float64 {
		out := make([]float64, frames)
		copy(out, x)
		return out
	}
	e := &drumEar{}
	r := fit(rms(d, min(frames, 1+len(d)/hop)))
	loud := percentile(r, 95)
	if loud <= 0 || loud < drumSilent*mixLoud {
		e.onset, e.kick, e.snare, e.hat, e.rms = fit(nil), fit(nil), fit(nil), fit(nil), fit(nil)
		return e
	}
	D := stftMag(d)
	norm := func(env []float64) []float64 {
		env = fit(env)
		if top := percentile(env, 99); top > 0 {
			for i := range env {
				env[i] /= top
			}
		}
		return env
	}
	e.onset = norm(onsetStrength(melDB(D, melFilters(128, 0, SR/2))))
	e.kick = norm(onsetStrength(melDB(D, melFilters(16, 20, 150))))
	e.snare = norm(onsetStrength(melDB(D, melFilters(32, 180, 3000))))
	e.hat = norm(onsetStrength(melDB(D, melFilters(32, 6000, SR/2))))
	for i := range r {
		r[i] /= loud
	}
	e.rms = r
	return e
}

func around(env []float64, i, w int) float64 {
	lo, hi := max(0, i-w), min(len(env), i+w+1)
	if lo >= hi {
		return 0
	}
	return slices.Max(env[lo:hi])
}

// drumHit é um ataque da bateria.
type drumHit struct {
	t, v float64
	kind byte
}

// peaks devolve os ataques da bateria: picos locais da curva de ataques onde
// a bateria soa, cada um com a peça que tocou (a banda que mais atacou).
func (e *drumEar) peaks() []drumHit {
	var out []drumHit
	for i := 3; i+3 < len(e.onset); i++ {
		v := e.onset[i]
		if v < drumPeak || v < slices.Max(e.onset[i-3:i+4]) || around(e.rms, i, 3) < drumAudible {
			continue
		}
		k, s := around(e.kick, i, 2), around(e.snare, i, 2)
		kind := byte(kindHat)
		switch {
		case k >= 0.35 && k >= 0.8*s:
			kind = kindKick
		case s >= 0.3:
			kind = kindSnare
		}
		out = append(out, drumHit{float64(i) * hop / SR, v, kind})
	}
	return out
}

// pickHitsDrums escolhe os toques da música com a bateria separada. Primeiro
// os de bumbo e caixa, do mais forte ao mais fraco, sempre a pelo menos
// minHitGap um do outro; depois, para a luz não parar, o chimbal e os ataques
// dos outros instrumentos (mixEnv = curva de ataques da música; perc = a
// percussiva, para a força deles), que perto da bateria só entram num buraco
// de quase uma batida de cada lado; por fim, batidas da grade onde nada toca.
// Devolve os toques em ordem, a peça de cada um e a força (0 a 1; os que não
// são bateria ficam abaixo de 0,3).
func pickHitsDrums(e *drumEar, mixEnv, perc, beats []float64, bpm float64) (hits []float64, kinds []byte, strength []float64) {
	beat := 60 / math.Max(bpm, 1)
	type cand struct {
		t, v float64
		kind byte
	}
	var lead, fill []cand
	for _, p := range e.peaks() {
		c := cand{p.t, math.Min(1, p.v), p.kind}
		if p.kind == kindHat {
			c.v *= 0.6 // chimbal conta menos que bumbo e caixa de mesma força
			fill = append(fill, c)
		} else {
			lead = append(lead, c)
		}
	}
	// Ataques da música inteira, como em pickHits, com a força percussiva.
	const (
		peakWin = 3
		avgWin  = 11
		delta   = 0.06
	)
	top, percTop := 0.0, percentile(perc, 99)
	for _, v := range mixEnv {
		top = math.Max(top, v)
	}
	for i, v := range mixEnv {
		if top <= 0 || v < mean(mixEnv[max(0, i-avgWin):min(len(mixEnv), i+avgWin+1)])+delta*top || v < slices.Max(mixEnv[max(0, i-peakWin):min(len(mixEnv), i+peakWin+1)]) {
			continue
		}
		s := 0.0
		if percTop > 0 {
			s = 0.3 * math.Min(1, around(perc, i, 2)/percTop)
		}
		fill = append(fill, cand{float64(i) * hop / SR, s, kindOther})
	}
	byStrength := func(a, b cand) int { return -cmpF(a.v, b.v) }
	slices.SortStableFunc(lead, byStrength)
	slices.SortStableFunc(fill, byStrength)

	var chosen []cand
	free := func(t, gap float64) bool {
		for _, c := range chosen {
			if math.Abs(c.t-t) < gap {
				return false
			}
		}
		return true
	}
	for _, c := range lead {
		if free(c.t, minHitGap) {
			chosen = append(chosen, c)
		}
	}
	leads := len(chosen)
	for _, c := range fill {
		gap := minHitGap
		for _, l := range chosen[:leads] {
			if math.Abs(l.t-c.t) <= 2*beat { // há bateria por perto: ela manda
				gap = math.Max(minHitGap, fillerGap*beat)
				break
			}
		}
		if free(c.t, gap) {
			chosen = append(chosen, c)
		}
	}
	for _, b := range beats {
		if free(b, math.Max(2*beat*0.99, minHitGap)) {
			chosen = append(chosen, cand{b, 0, kindOther})
		}
	}
	slices.SortFunc(chosen, func(a, b cand) int { return cmpF(a.t, b.t) })
	for _, c := range chosen {
		hits = append(hits, round(c.t, 3))
		kinds = append(kinds, c.kind)
		strength = append(strength, round(c.v, 2))
	}
	return hits, kinds, strength
}

// beatLevels devolve o nível da bateria em cada batida, de 0 a 1 (1 = os
// trechos de bateria cheia da música).
func (e *drumEar) beatLevels(beats []float64, beat float64) []float64 {
	lv := make([]float64, len(beats))
	for i, b := range beats {
		end := b + beat
		if i+1 < len(beats) {
			end = beats[i+1]
		}
		fa, fb := min(int(b*SR/hop), len(e.rms)), min(int(end*SR/hop), len(e.rms))
		lv[i] = mean(e.rms[fa:max(fa, fb)])
	}
	if ref := percentile(lv, 90); len(lv) > 0 && ref > 0 {
		for i := range lv {
			lv[i] = math.Min(1, lv[i]/ref)
		}
	}
	return lv
}

// drumEdges devolve os instantes em que a bateria entra ou sai: a batida em
// que o nível cruza drumOn e fica do outro lado por pelo menos 4 batidas.
func drumEdges(levels, beats []float64) []float64 {
	const (
		drumOn = 0.3
		hold   = 4
	)
	var out []float64
	on := len(levels) > 0 && levels[0] >= drumOn
	for b := 1; b+hold <= len(levels); b++ {
		if (levels[b] >= drumOn) == on {
			continue
		}
		stable := true
		for _, v := range levels[b : b+hold] {
			stable = stable && (v >= drumOn) != on
		}
		if stable {
			on = !on
			out = append(out, beats[b])
		}
	}
	return out
}

// snapToDrums leva a fronteira entre duas partes para a entrada ou saída da
// bateria mais próxima, se ela está a menos de splitBeats batidas: as partes
// vêm do timbre, com um ou dois compassos de imprecisão, e a bateria entra
// num instante exato. (Mais longe que isso, a parte é dividida.)
func snapToDrums(secs []show.Section, levels, beats []float64, beat float64) {
	edges := drumEdges(levels, beats)
	for i := 1; i < len(secs); i++ {
		at, best := secs[i].Start, math.Inf(1)
		for _, e := range edges {
			if d := math.Abs(e - secs[i].Start); d > 0 && d < splitBeats*beat && d < best &&
				e-secs[i-1].Start >= splitBeats*beat && secs[i].End-e >= splitBeats*beat {
				at, best = e, d
			}
		}
		secs[i-1].End, secs[i].Start = at, at
	}
}

// setPulseDrums grava em cada parte a batida medida na bateria: o nível
// típico dela nas batidas da parte. Uma parte sem bateria fica com 0,01
// (zero quer dizer "não medido").
func setPulseDrums(secs []show.Section, beats, levels []float64) {
	for i := range secs {
		lo, _ := slices.BinarySearch(beats, secs[i].Start)
		hi, _ := slices.BinarySearch(beats, secs[i].End)
		secs[i].Pulse = math.Max(0.01, round(medianOf(levels[lo:max(lo, hi)]), 2))
	}
}

// drumEntries acha as entradas da bateria: a primeira batida em que ela soa
// (nível ≥ entryOn) depois de pelo menos entryQuiet batidas quase sem ela, se
// daí em diante ela vem cheia (mediana das entryQuiet batidas seguintes ≥
// entryFull). É o "drop": o momento de maior impacto da batida.
func drumEntries(levels, beats []float64) []float64 {
	const (
		entryOn    = 0.3
		entryQuiet = 8
		entryFull  = 0.5
	)
	var out []float64
	quiet := 0
	for b, v := range levels {
		if v < entryOn {
			quiet++
			continue
		}
		if quiet >= entryQuiet && medianOf(levels[b-quiet:b]) <= 0.12 && medianOf(levels[b:min(len(levels), b+entryQuiet)]) >= entryFull {
			out = append(out, beats[b])
		}
		quiet = 0
	}
	return out
}

// pickAccentsDrums acha os picos com a bateria separada, sempre em toques de
// bateria: a entrada da bateria depois de um trecho sem ela (o drop: flash
// cheio), um toque solto num trecho sem bateria (a batida solta no meio do
// verso) e um toque bem acima dos toques de bateria vizinhos (accentRatio).
// levels é o nível da bateria em cada batida de beats.
func pickAccentsDrums(hits []float64, kinds []byte, strength, levels, beats []float64, beat float64) []show.Accent {
	type cand struct{ t, v float64 }
	var cands []cand
	for _, entry := range drumEntries(levels, beats) {
		at := entry
		for k, h := range hits { // o primeiro toque de bateria da entrada
			if kinds[k] != kindOther && h >= entry-0.06 && h <= entry+beat {
				at = h
				break
			}
		}
		cands = append(cands, cand{at, 1})
	}
	for k, s := range strength {
		if kinds[k] == kindOther || s < accentFloor {
			continue
		}
		var drums []float64
		for j := max(0, k-accentWin); j < min(len(hits), k+1+accentWin); j++ {
			if j != k && kinds[j] != kindOther {
				drums = append(drums, strength[j])
			}
		}
		switch med := medianOf(drums); {
		case len(drums) < accentWin/2: // sem bateria em volta
			if s >= loneFlash {
				cands = append(cands, cand{hits[k], 0.6 + 0.4*s})
			}
		case med > 0 && s >= accentRatio*med:
			cands = append(cands, cand{hits[k], math.Min(1, 0.6+0.4*(s/med-accentRatio)/(3-accentRatio))})
		}
	}
	slices.SortStableFunc(cands, func(a, b cand) int { return -cmpF(a.v, b.v) })
	var out []show.Accent
	for _, c := range cands {
		if !slices.ContainsFunc(out, func(a show.Accent) bool { return math.Abs(a.T-c.t) < accentGap*beat }) {
			out = append(out, show.Accent{T: c.t, V: round(c.v, 2)})
		}
	}
	slices.SortFunc(out, func(a, b show.Accent) int { return cmpF(a.T, b.T) })
	return out
}
