package analysis

// Versão cantada: a mesma faixa do playback com a voz. Como o instrumental é
// o mesmo, alinhar as duas e tirar o playback da cantada deixa (quase) só a
// voz. Da energia dela saem as frases cantadas, com início e fim exatos, que
// a IA depois transcreve (llm.go) para a luz acender na palavra.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/cmplx"
	"slices"

	"gonum.org/v1/gonum/dsp/fourier"

	"github.com/ncastellani/DMXController/internal/audio"
)

const (
	maxVoiceShift = 5.0  // s: deslocamento máximo procurado entre as versões
	voiceLo       = 150  // Hz: faixa da voz usada para achar as frases
	voiceHi       = 5000 // Hz
	phraseGap     = 0.35 // s: silêncio menor que isso não separa frases
	phraseMin     = 0.3  // s: mais curto que isso não é frase
	phraseMax     = 12.0 // s: frases mais longas são divididas no ponto mais baixo
	maxPhrases    = 160
)

// voice é a voz tirada da versão cantada, já no tempo do playback.
type voice struct {
	Shift   float64   // s: quanto a versão cantada está atrasada em relação ao playback
	Clean   bool      // a subtração deixou a voz limpa (senão Audio é a cantada inteira)
	Audio   []float64 // mono a SR, alinhado ao playback: o que a IA ouve
	Phrases []phrase
	Note    string // aviso sobre o encaixe (duração diferente etc.)
}

// phrase é um trecho cantado contínuo, em segundos do playback.
type phrase struct{ Start, End float64 }

// isolateVoice alinha a versão cantada (sung) ao playback (play; y é o
// playback já em Mono) e acha as frases cantadas.
func isolateVoice(play, sung *audio.Track, y []float64) (*voice, error) {
	ys := Mono(sung)
	if len(y) < SR || len(ys) < SR {
		return nil, errors.New("faixa curta demais")
	}
	lag, score := alignLag(y, ys, int(maxVoiceShift*SR))
	if score < 30 {
		return nil, errors.New("a versão cantada não bate com o playback (precisa ser a mesma faixa, só que com a voz)")
	}
	// Na taxa original o atraso pode cair entre duas amostras a SR: refina.
	p44, s44 := mono44(play), mono44(sung)
	lag44 := refineLag(p44, s44, 2*lag)
	v := &voice{Shift: float64(lag44) / audio.SampleRate}
	if d := float64(len(s44)-lag44-len(p44)) / audio.SampleRate; math.Abs(d) > 0.5 {
		v.Note = fmt.Sprintf("a versão cantada tem %.1f s a %s que o playback", math.Abs(d), map[bool]string{true: "mais", false: "menos"}[d > 0])
	}
	aligned := make([]float32, len(p44))
	for i := range aligned {
		if j := i + lag44; j >= 0 && j < len(s44) {
			aligned[i] = s44[j]
		}
	}
	ya := Mono(fromMono(aligned))

	P, S := stftMag(y), stftMag(ya)
	ev, es := voiceEnergy(P, S)
	active := voiceActivity(ev, es)
	v.Phrases = phrases(active, ev)

	// Voz limpa no tempo: cantada − g·playback. Só serve se o instrumental
	// some de verdade nos trechos sem voz (mesma mixagem e master).
	silent := func(i int) bool { return !active[min((i/2+hop/2)/hop, len(active)-1)] }
	var sp, pp float64
	for i, x := range p44 {
		if silent(i) {
			sp += float64(aligned[i]) * float64(x)
			pp += float64(x) * float64(x)
		}
	}
	g := 1.0
	if pp > 0 {
		g = sp / pp
	}
	// Mediana por quadro: as bordas das frases que escaparam da detecção
	// não estragam a medida.
	res := make([]float32, len(p44))
	rr, ref := make([]float64, len(active)), make([]float64, len(active))
	for i, x := range p44 {
		res[i] = aligned[i] - float32(g)*x
		f := min((i/2+hop/2)/hop, len(active)-1)
		rr[f] += float64(res[i]) * float64(res[i])
		ref[f] += float64(x) * float64(x)
	}
	var ratios []float64
	for f, a := range active {
		if !a && ref[f] > 1e-6 {
			ratios = append(ratios, rr[f]/ref[f])
		}
	}
	v.Clean = len(ratios) > 50 && percentile(ratios, 50) < 0.01 // instrumental 20 dB mais baixo
	if v.Clean {
		v.Audio = Mono(fromMono(res))
	} else {
		v.Audio = ya
	}
	return v, nil
}

// mono44 é a média dos dois canais, na taxa do player.
func mono44(t *audio.Track) []float32 {
	out := make([]float32, len(t.PCM)/4)
	for i := range out {
		l := int16(binary.LittleEndian.Uint16(t.PCM[i*4:]))
		r := int16(binary.LittleEndian.Uint16(t.PCM[i*4+2:]))
		out[i] = (float32(l) + float32(r)) / 2 / 32768
	}
	return out
}

// fromMono monta uma faixa do player (os dois canais iguais) a partir de x.
func fromMono(x []float32) *audio.Track {
	pcm := make([]byte, 4*len(x))
	for i, v := range x {
		s := uint16(int16(math.Max(-1, math.Min(1, float64(v))) * 32767))
		binary.LittleEndian.PutUint16(pcm[i*4:], s)
		binary.LittleEndian.PutUint16(pcm[i*4+2:], s)
	}
	return &audio.Track{PCM: pcm}
}

// refineLag procura, perto de guess (±3 amostras), o atraso com a maior
// correlação num trecho de 10 s do meio.
func refineLag(p, s []float32, guess int) int {
	n := min(10*audio.SampleRate, len(p))
	start := (len(p) - n) / 2
	best, bestC := guess, math.Inf(-1)
	for l := guess - 3; l <= guess+3; l++ {
		var c float64
		for i := start; i < start+n; i++ {
			if j := i + l; j >= 0 && j < len(s) {
				c += float64(p[i]) * float64(s[j])
			}
		}
		if c > bestC {
			best, bestC = l, c
		}
	}
	return best
}

// alignLag acha o atraso (em amostras) da versão cantada em relação ao
// playback por correlação cruzada com fase normalizada (GCC-PHAT) num trecho
// de ~47 s do meio da música. score é o pico dividido pelo desvio padrão da
// correlação: com o mesmo instrumental passa de 100; com outra música, < 10.
func alignLag(play, sung []float64, maxLag int) (lag int, score float64) {
	seg := min(1<<20, len(play))
	start := (len(play) - seg) / 2
	n := 1
	for n < 2*(seg+maxLag) {
		n <<= 1
	}
	a, b := make([]float64, n), make([]float64, n)
	copy(a, play[start:start+seg])
	for i := range seg + 2*maxLag {
		if j := start - maxLag + i; j >= 0 && j < len(sung) {
			b[i] = sung[j]
		}
	}
	fft := fourier.NewFFT(n)
	A, B := fft.Coefficients(nil, a), fft.Coefficients(nil, b)
	for k := range A {
		c := cmplx.Conj(A[k]) * B[k]
		if m := cmplx.Abs(c); m > 1e-12 {
			A[k] = c / complex(m, 0)
		} else {
			A[k] = 0
		}
	}
	r := fft.Sequence(nil, A)
	// r[k] compara play[start+i] com b[i+k] = sung[start-maxLag+i+k]: o
	// atraso é k-maxLag, procurado em [0, 2·maxLag].
	best, sum, sq := 0, 0.0, 0.0
	for k := 0; k <= 2*maxLag; k++ {
		if r[k] > r[best] {
			best = k
		}
		sum += r[k]
		sq += r[k] * r[k]
	}
	cnt := float64(2*maxLag + 1)
	std := math.Sqrt(math.Max(sq/cnt-(sum/cnt)*(sum/cnt), 1e-30))
	return best - maxLag, r[best] / std
}

// voiceEnergy subtrai o espectro do playback do da cantada na faixa da voz.
// O ganho de cada banda vem dos quadros sem voz (o quantil baixo da razão
// cantada/playback), e o playback usado é o máximo do quadro e dos vizinhos,
// para sobrar só o que é claramente voz. Devolve a energia da voz e a da
// cantada (mesma faixa) por quadro.
func voiceEnergy(P, S *matrix) (ev, es []float64) {
	lo, hi := voiceLo*nFFT/SR, voiceHi*nFFT/SR
	const bands = 12
	edge := func(b int) int { // bandas logarítmicas
		return int(math.Round(float64(lo) * math.Pow(float64(hi)/float64(lo), float64(b)/bands)))
	}
	frames := P.cols
	gain := make([]float64, hi+1)
	for b := range bands {
		k0, k1 := edge(b), edge(b+1)
		var ratios, pe []float64
		for t := range frames {
			var p, s float64
			for k := k0; k < k1; k++ {
				p += float64(P.at(k, t)) * float64(P.at(k, t))
				s += float64(S.at(k, t)) * float64(S.at(k, t))
			}
			pe = append(pe, p)
			ratios = append(ratios, math.Sqrt(s/math.Max(p, 1e-12)))
		}
		floor := percentile(pe, 30)
		var loud []float64
		for t, r := range ratios {
			if pe[t] > floor {
				loud = append(loud, r)
			}
		}
		g := 1.0
		if len(loud) > 10 {
			g = math.Max(0.25, math.Min(4, percentile(loud, 25)))
		}
		for k := k0; k <= min(k1, hi); k++ {
			gain[k] = g
		}
	}
	ev, es = make([]float64, frames), make([]float64, frames)
	for t := range frames {
		for k := lo; k <= hi; k++ {
			p := P.at(k, t)
			if t > 0 {
				p = max(p, P.at(k, t-1))
			}
			if t+1 < frames {
				p = max(p, P.at(k, t+1))
			}
			s := float64(S.at(k, t))
			if d := s - gain[k]*float64(p); d > 0 {
				ev[t] += d * d
			}
			es[t] += s * s
		}
	}
	return ev, es
}

// voiceActivity marca os quadros com voz: a voz tem de ser uma parte boa do
// som da cantada e estar acima do piso de ruído da própria voz.
func voiceActivity(ev, es []float64) []bool {
	n := len(ev)
	smooth := func(x []float64, w int) []float64 {
		out := make([]float64, n)
		for t := range n {
			var s float64
			c := 0
			for i := max(0, t-w); i <= min(n-1, t+w); i++ {
				s += x[i]
				c++
			}
			out[t] = s / float64(c)
		}
		return out
	}
	sv, ss := smooth(ev, 3), smooth(es, 3)
	floor := percentile(sv, 98) * 1e-3 // 30 dB abaixo dos trechos mais fortes
	active := make([]bool, n)
	for t := range n {
		active[t] = sv[t] > floor && sv[t] > 0.12*ss[t]
	}
	return active
}

// phrases junta os quadros com voz em frases: lacunas curtas não separam,
// trechos curtos demais saem e frases longas são divididas no quadro de
// menor energia (longe das pontas).
func phrases(active []bool, ev []float64) []phrase {
	ft := func(f int) float64 { return float64(f) * hop / SR }
	frames := func(sec float64) int { return int(sec * SR / hop) }
	gap, minLen, maxLen := frames(phraseGap), frames(phraseMin), frames(phraseMax)
	type run struct{ a, b int } // quadros [a, b)
	var runs []run
	for t := 0; t < len(active); {
		if !active[t] {
			t++
			continue
		}
		u := t
		for u < len(active) && active[u] {
			u++
		}
		if len(runs) > 0 && t-runs[len(runs)-1].b <= gap {
			runs[len(runs)-1].b = u
		} else {
			runs = append(runs, run{t, u})
		}
		t = u
	}
	var split func(r run) []run
	split = func(r run) []run {
		if r.b-r.a <= maxLen {
			return []run{r}
		}
		q := (r.b - r.a) / 4
		cut := r.a + q
		for f := r.a + q; f < r.b-q; f++ {
			if ev[f] < ev[cut] {
				cut = f
			}
		}
		return append(split(run{r.a, cut}), split(run{cut, r.b})...)
	}
	var out []phrase
	for _, r := range runs {
		if r.b-r.a < minLen {
			continue
		}
		for _, s := range split(r) {
			out = append(out, phrase{round(ft(s.a), 3), round(ft(s.b), 3)})
		}
	}
	if len(out) > maxPhrases { // fica com as mais longas, na ordem
		idx := make([]int, len(out))
		for i := range idx {
			idx[i] = i
		}
		slices.SortFunc(idx, func(x, y int) int {
			return cmpF(out[y].End-out[y].Start, out[x].End-out[x].Start)
		})
		idx = idx[:maxPhrases]
		slices.Sort(idx)
		var keep []phrase
		for _, i := range idx {
			keep = append(keep, out[i])
		}
		out = keep
	}
	return out
}

// clip recorta [start, end] s do áudio, com uma folga de pad s dos dois lados.
func clip(y []float64, start, end, pad float64) []float64 {
	a := max(0, int((start-pad)*SR))
	b := min(len(y), int((end+pad)*SR))
	if b <= a {
		return nil
	}
	return y[a:b]
}
