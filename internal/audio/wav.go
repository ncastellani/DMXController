// Package audio lê faixas WAV e as toca na saída de som do sistema
// (CoreAudio no macOS, PulseAudio/PipeWire ou ALSA no Linux), informando a
// posição de reprodução para sincronizar a luz.
package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"time"
)

// Formato interno de reprodução: PCM 16 bits, estéreo, 44,1 kHz.
const (
	SampleRate    = 44100
	Channels      = 2
	bytesPerFrame = 2 * Channels
)

// Track é uma faixa decodificada no formato interno.
type Track struct {
	Path string
	PCM  []byte // int16 little-endian intercalado (L, R)
}

// Duration é a duração da faixa.
func (t *Track) Duration() time.Duration {
	return framesToDur(int64(len(t.PCM) / bytesPerFrame))
}

func framesToDur(frames int64) time.Duration {
	return time.Duration(frames) * time.Second / SampleRate
}

func durToFrames(d time.Duration) int64 {
	return int64(d) * SampleRate / int64(time.Second)
}

// LoadWAV lê um WAV PCM (8/16/24/32 bits inteiros ou float 32/64), em
// qualquer taxa e número de canais, e converte para o formato interno.
func LoadWAV(path string) (*Track, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pcm, err := decodeWAV(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Track{Path: path, PCM: pcm}, nil
}

type wavFormat struct {
	format     uint16 // 1 = PCM inteiro, 3 = float
	channels   int
	sampleRate int
	bits       int
}

func decodeWAV(b []byte) ([]byte, error) {
	if len(b) < 12 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, errors.New("não é um arquivo WAV")
	}
	var f *wavFormat
	var data []byte
	for p := 12; p+8 <= len(b); {
		id, size := string(b[p:p+4]), int(binary.LittleEndian.Uint32(b[p+4:p+8]))
		body := b[p+8 : min(p+8+size, len(b))]
		switch id {
		case "fmt ":
			if len(body) < 16 {
				return nil, errors.New("chunk fmt curto")
			}
			f = &wavFormat{
				format:     binary.LittleEndian.Uint16(body[0:2]),
				channels:   int(binary.LittleEndian.Uint16(body[2:4])),
				sampleRate: int(binary.LittleEndian.Uint32(body[4:8])),
				bits:       int(binary.LittleEndian.Uint16(body[14:16])),
			}
			if f.format == 0xFFFE && len(body) >= 26 { // WAVE_FORMAT_EXTENSIBLE
				f.format = binary.LittleEndian.Uint16(body[24:26])
			}
		case "data":
			data = body
		}
		p += 8 + size + size%2 // chunks são alinhados em 2 bytes
	}
	if f == nil || data == nil {
		return nil, errors.New("WAV sem chunks fmt/data")
	}
	if f.channels < 1 || f.sampleRate < 1000 {
		return nil, fmt.Errorf("formato inválido (%d canais, %d Hz)", f.channels, f.sampleRate)
	}

	sample, err := sampleReader(f)
	if err != nil {
		return nil, err
	}
	width := f.bits / 8
	frames := len(data) / (width * f.channels)
	// Converte para estéreo float: mono é duplicado; acima de 2 canais, usa os 2 primeiros.
	left := make([]float32, frames)
	right := make([]float32, frames)
	for i := 0; i < frames; i++ {
		base := i * width * f.channels
		left[i] = sample(data[base:])
		if f.channels > 1 {
			right[i] = sample(data[base+width:])
		} else {
			right[i] = left[i]
		}
	}
	if f.sampleRate != SampleRate {
		left = resample(left, f.sampleRate)
		right = resample(right, f.sampleRate)
	}

	out := make([]byte, len(left)*bytesPerFrame)
	for i := range left {
		binary.LittleEndian.PutUint16(out[i*4:], uint16(toInt16(left[i])))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(toInt16(right[i])))
	}
	return out, nil
}

// sampleReader devolve uma função que lê uma amostra como float em -1..1.
func sampleReader(f *wavFormat) (func([]byte) float32, error) {
	switch {
	case f.format == 1 && f.bits == 8:
		return func(b []byte) float32 { return (float32(b[0]) - 128) / 128 }, nil
	case f.format == 1 && f.bits == 16:
		return func(b []byte) float32 { return float32(int16(binary.LittleEndian.Uint16(b))) / 32768 }, nil
	case f.format == 1 && f.bits == 24:
		return func(b []byte) float32 {
			v := int32(b[0]) | int32(b[1])<<8 | int32(int8(b[2]))<<16
			return float32(v) / (1 << 23)
		}, nil
	case f.format == 1 && f.bits == 32:
		return func(b []byte) float32 { return float32(int32(binary.LittleEndian.Uint32(b))) / (1 << 31) }, nil
	case f.format == 3 && f.bits == 32:
		return func(b []byte) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(b)) }, nil
	case f.format == 3 && f.bits == 64:
		return func(b []byte) float32 { return float32(math.Float64frombits(binary.LittleEndian.Uint64(b))) }, nil
	}
	return nil, fmt.Errorf("formato de WAV não suportado (formato %d, %d bits)", f.format, f.bits)
}

// resample converte de rate para SampleRate por interpolação linear
// (suficiente para playback de show; o ideal é exportar em 44,1 kHz).
func resample(in []float32, rate int) []float32 {
	n := int(int64(len(in)) * SampleRate / int64(rate))
	out := make([]float32, n)
	step := float64(rate) / SampleRate
	for i := range out {
		pos := float64(i) * step
		j := int(pos)
		if j+1 >= len(in) {
			out[i] = in[len(in)-1]
			continue
		}
		frac := float32(pos - float64(j))
		out[i] = in[j] + (in[j+1]-in[j])*frac
	}
	return out
}

func toInt16(v float32) int16 {
	switch {
	case v >= 1:
		return math.MaxInt16
	case v <= -1:
		return math.MinInt16
	}
	return int16(v * 32767)
}

// Clicks gera uma faixa de cliques (metrônomo) em bpm, para calibrar o
// atraso entre som e luz.
func Clicks(bpm float64, dur time.Duration) *Track {
	frames := int(durToFrames(dur))
	period := float64(SampleRate) * 60 / bpm
	clickLen := int(0.04 * SampleRate)
	pcm := make([]byte, frames*bytesPerFrame)
	for i := 0; i < frames; i++ {
		phase := int(math.Mod(float64(i), period))
		if phase >= clickLen {
			continue
		}
		t := float64(phase) / SampleRate
		v := toInt16(float32(0.8 * math.Sin(2*math.Pi*1500*t) * math.Exp(-t*90)))
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
	}
	return &Track{Path: "cliques", PCM: pcm}
}
