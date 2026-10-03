package analysis

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"testing"

	"luzes/internal/audio"
)

// track monta uma faixa do player a partir de um sinal mono a 44,1 kHz.
func track(x []float64) *audio.Track {
	pcm := make([]byte, 4*len(x))
	for i, v := range x {
		s := uint16(int16(math.Max(-1, math.Min(1, v)) * 32767))
		binary.LittleEndian.PutUint16(pcm[i*4:], s)
		binary.LittleEndian.PutUint16(pcm[i*4+2:], s)
	}
	return &audio.Track{PCM: pcm}
}

// fakeSong é um "instrumental": acordes que mudam a cada 2 s e um chimbal
// (ruído curto) a cada batida de 0,5 s.
func fakeSong(sec float64, rng *rand.Rand) []float64 {
	const sr = audio.SampleRate
	x := make([]float64, int(sec*sr))
	var notes [3]float64
	for i := range x {
		t := float64(i) / sr
		if i%(2*sr) == 0 {
			for k := range notes {
				notes[k] = 110 * math.Pow(2, float64(rng.IntN(24))/12)
			}
		}
		for _, f := range notes {
			x[i] += 0.08 * math.Sin(2*math.Pi*f*t)
		}
		if beat := math.Mod(t, 0.5); beat < 0.05 {
			x[i] += 0.15 * (rng.Float64()*2 - 1) * (1 - beat/0.05)
		}
	}
	return x
}

func TestIsolateVoice(t *testing.T) {
	const sr = audio.SampleRate
	rng := rand.New(rand.NewPCG(1, 2))
	play := fakeSong(40, rng)
	// "Voz": harmônicos de uma nota com vibrato, nestes trechos.
	sung := append([]float64(nil), play...)
	want := []phrase{{5, 8}, {10, 13.5}, {20, 21}, {30, 36}}
	for _, ph := range want {
		for i := int(ph.Start * sr); i < int(ph.End*sr); i++ {
			tt := float64(i) / sr
			f0 := 220 * (1 + 0.01*math.Sin(2*math.Pi*5*tt))
			for h := 1; h <= 6; h++ {
				sung[i] += 0.12 / float64(h) * math.Sin(2*math.Pi*f0*float64(h)*tt)
			}
		}
	}
	const delay = 5425 // 0,123 s; ímpar: cai entre duas amostras a 22.050 Hz
	sung = append(make([]float64, delay), sung...)

	p := track(play)
	v, err := isolateVoice(p, track(sung), Mono(p))
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(v.Shift-float64(delay)/sr) > 1e-9 || !v.Clean || v.Note != "" {
		t.Fatalf("encaixe: atraso %.5f, limpa %v, aviso %q", v.Shift, v.Clean, v.Note)
	}
	if len(v.Phrases) != len(want) {
		t.Fatalf("frases: %+v", v.Phrases)
	}
	for i, ph := range v.Phrases {
		if math.Abs(ph.Start-want[i].Start) > 0.15 || math.Abs(ph.End-want[i].End) > 0.15 {
			t.Errorf("frase %d: %+v, quero %+v", i, ph, want[i])
		}
	}

	// Outra música não é a versão cantada.
	other := track(fakeSong(40, rand.New(rand.NewPCG(3, 4))))
	if _, err := isolateVoice(p, other, Mono(p)); err == nil {
		t.Fatal("aceitou uma faixa diferente")
	}
}

func TestPhrasesSplitLong(t *testing.T) {
	// 30 s seguidos de voz viram frases de até 12 s, cortadas onde a voz
	// está mais fraca.
	n := int(30 * SR / hop)
	active, ev := make([]bool, n), make([]float64, n)
	for i := range n {
		active[i], ev[i] = true, 1
	}
	ev[int(13*SR/hop)] = 0.1
	ps := phrases(active, ev)
	cut := false
	for _, p := range ps {
		cut = cut || math.Abs(p.End-13) < 0.05
	}
	if len(ps) < 3 || !cut {
		t.Fatalf("frases: %+v", ps)
	}
	for _, p := range ps {
		if p.End-p.Start > phraseMax+0.01 {
			t.Fatalf("frase longa: %+v", p)
		}
	}
}
