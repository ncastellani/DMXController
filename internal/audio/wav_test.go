package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

// wav monta um WAV mínimo com as amostras já codificadas.
func wav(format, channels, rate, bits int, data []byte) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WAVE")
	fmtc := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtc[0:], uint16(format))
	binary.LittleEndian.PutUint16(fmtc[2:], uint16(channels))
	binary.LittleEndian.PutUint32(fmtc[4:], uint32(rate))
	binary.LittleEndian.PutUint16(fmtc[14:], uint16(bits))
	b = append(b, chunk("LIST", []byte("xx"))...) // chunk desconhecido no caminho
	b = append(b, chunk("fmt ", fmtc)...)
	return append(b, chunk("data", data)...)
}

func chunk(id string, body []byte) []byte {
	h := make([]byte, 8)
	copy(h, id)
	binary.LittleEndian.PutUint32(h[4:], uint32(len(body)))
	out := append(h, body...)
	if len(body)%2 == 1 {
		out = append(out, 0)
	}
	return out
}

func frame(pcm []byte, i int) (int16, int16) {
	return int16(binary.LittleEndian.Uint16(pcm[i*4:])), int16(binary.LittleEndian.Uint16(pcm[i*4+2:]))
}

func TestDecode16Stereo(t *testing.T) {
	data := []byte{0x00, 0x40, 0x00, 0xC0} // L=16384 R=-16384
	pcm, err := decodeWAV(wav(1, 2, SampleRate, 16, data))
	if err != nil {
		t.Fatal(err)
	}
	if l, r := frame(pcm, 0); l < 16380 || l > 16385 || r > -16380 || r < -16385 {
		t.Fatalf("L=%d R=%d", l, r)
	}
}

func TestDecode24MonoAndFloat(t *testing.T) {
	// 24 bits mono: metade da escala positiva → duplicado nos dois canais.
	pcm, err := decodeWAV(wav(1, 1, SampleRate, 24, []byte{0x00, 0x00, 0x40}))
	if err != nil {
		t.Fatal(err)
	}
	if l, r := frame(pcm, 0); l != r || l < 16380 || l > 16385 {
		t.Fatalf("24 bits mono: L=%d R=%d", l, r)
	}
	// float32 estéreo, com clipping acima de 1.
	f := make([]byte, 8)
	binary.LittleEndian.PutUint32(f[0:], math.Float32bits(2))
	binary.LittleEndian.PutUint32(f[4:], math.Float32bits(-0.5))
	pcm, err = decodeWAV(wav(3, 2, SampleRate, 32, f))
	if err != nil {
		t.Fatal(err)
	}
	if l, r := frame(pcm, 0); l != math.MaxInt16 || r > -16380 {
		t.Fatalf("float: L=%d R=%d", l, r)
	}
}

func TestResample(t *testing.T) {
	// 1 s a 22050 Hz vira 1 s a 44100 Hz.
	data := make([]byte, 22050*2)
	pcm, err := decodeWAV(wav(1, 1, 22050, 16, data))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(pcm) / bytesPerFrame; got != SampleRate {
		t.Fatalf("quadros = %d, quero %d", got, SampleRate)
	}
}

func TestDecodeErrors(t *testing.T) {
	for name, b := range map[string][]byte{
		"não é wav": []byte("ID3 isso é um mp3"),
		"sem data":  wav(1, 2, SampleRate, 16, nil)[:len(wav(1, 2, SampleRate, 16, nil))-8],
		"12 bits":   wav(1, 2, SampleRate, 12, []byte{0, 0}),
		"float 16":  wav(3, 2, SampleRate, 16, []byte{0, 0}),
		"0 canais":  wav(1, 0, SampleRate, 16, []byte{0, 0}),
	} {
		if _, err := decodeWAV(b); err == nil {
			t.Errorf("%s: deveria dar erro", name)
		}
	}
}
