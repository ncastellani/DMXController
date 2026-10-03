package audio

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const (
	// bufferTime é o buffer da saída do oto; entra na compensação de atraso.
	bufferTime = 100 * time.Millisecond
	// openTimeout limita a espera pela saída de som ao iniciar (um Bluetooth
	// instável não pode impedir o programa de subir).
	openTimeout = 5 * time.Second
)

// Player toca um fluxo contínuo de faixas e informa a posição que está
// saindo na caixa. Faixas acrescentadas com Append entram no mesmo fluxo,
// depois da anterior e de um intervalo de silêncio exato — sem nenhum
// corte entre elas (para músicas emendadas soarem como uma só).
//
// Posições são do fluxo (stream): a primeira faixa começa em 0.
type Player struct {
	ctx *oto.Context

	mu      sync.Mutex
	pl      *oto.Player
	src     *source
	volume  float64
	latency time.Duration // atraso extra da saída (ex.: Bluetooth), calibrado; negativo adianta a luz
}

// NewPlayer abre a saída de som padrão do sistema. Só pode existir um.
func NewPlayer() (*Player, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   SampleRate,
		ChannelCount: Channels,
		Format:       oto.FormatSignedInt16LE,
		BufferSize:   bufferTime,
	})
	if err != nil {
		return nil, err
	}
	select {
	case <-ready:
	case <-time.After(openTimeout):
		return nil, fmt.Errorf("a saída de som não respondeu em %v", openTimeout)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Player{ctx: ctx, volume: 1}, nil
}

// Load começa um fluxo novo só com t (parado, no início).
func (p *Player) Load(t *Track) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pl != nil {
		p.pl.Pause()
		p.pl.Close()
	}
	p.src = &source{}
	p.src.append(t.PCM, 0)
	p.pl = p.ctx.NewPlayer(p.src)
	p.pl.SetVolume(p.volume)
}

// Append acrescenta t ao fim do fluxo, depois de gap de silêncio, e devolve
// onde ela começa.
func (p *Player) Append(t *Track, gap time.Duration) (time.Duration, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.src == nil {
		return 0, errors.New("nenhuma faixa carregada")
	}
	return bytesToDur(p.src.append(t.PCM, durToFrames(max(0, gap))*bytesPerFrame)), nil
}

// Truncate tira do fluxo as faixas que começam em at ou depois — só se
// nada delas foi lido ainda. Devolve false se já é tarde.
func (p *Player) Truncate(at time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.src != nil && p.src.truncate(durToFrames(at)*bytesPerFrame)
}

// Release libera a memória das faixas que terminaram antes de before.
func (p *Player) Release(before time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.src != nil {
		p.src.release(durToFrames(before) * bytesPerFrame)
	}
}

// End é o fim do fluxo.
func (p *Player) End() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.src == nil {
		return 0
	}
	return bytesToDur(p.src.end())
}

// Unload descarrega tudo.
func (p *Player) Unload() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pl != nil {
		p.pl.Pause()
		p.pl.Close()
	}
	p.pl, p.src = nil, nil
}

// Play começa ou continua a reprodução.
func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pl != nil {
		p.pl.Play()
	}
}

// Pause pausa a reprodução.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pl != nil {
		p.pl.Pause()
	}
}

// Seek vai para a posição d do fluxo.
func (p *Player) Seek(d time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pl == nil {
		return errors.New("nenhuma faixa carregada")
	}
	off := max(0, durToFrames(d)) * bytesPerFrame
	_, err := p.pl.Seek(min(off, p.src.end()), io.SeekStart)
	return err
}

// Playing informa se está tocando.
func (p *Player) Playing() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pl != nil && p.pl.IsPlaying()
}

// Position é a posição do fluxo que está saindo na caixa agora: o que já
// foi lido, menos o que ainda está em buffer e o atraso da saída. Com atraso
// negativo (o som chega antes da luz) a posição anda à frente.
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pl == nil {
		return 0
	}
	pos := bytesToDur(p.src.Pos() - int64(p.pl.BufferedSize()))
	if p.pl.IsPlaying() {
		pos -= bufferTime + p.latency
	}
	return max(0, pos)
}

// SetVolume ajusta o volume (0..1).
func (p *Player) SetVolume(v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = min(1, max(0, v))
	if p.pl != nil {
		p.pl.SetVolume(p.volume)
	}
}

// Volume devolve o volume (0..1).
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

// SetLatency define o atraso extra da saída de som (calibração). Negativo
// adianta a luz: é para quando o som sai antes dela.
func (p *Player) SetLatency(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.latency = d
}

// Latency devolve o atraso extra configurado.
func (p *Player) Latency() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.latency
}

func bytesToDur(b int64) time.Duration { return framesToDur(b / bytesPerFrame) }

// segment é uma faixa no fluxo, começando no byte start.
type segment struct {
	start int64
	data  []byte // nil depois de liberada
	size  int64
}

// source é o fluxo lido pelo oto (numa goroutine própria): as faixas em
// sequência, com silêncio nos intervalos, e posição consultável.
type source struct {
	mu   sync.Mutex
	segs []segment
	pos  int64
}

func (s *source) end() int64 {
	if len(s.segs) == 0 {
		return 0
	}
	last := s.segs[len(s.segs)-1]
	return last.start + last.size
}

func (s *source) append(data []byte, gap int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := s.end() + gap
	if len(s.segs) == 0 {
		start = 0
	}
	s.segs = append(s.segs, segment{start: start, data: data, size: int64(len(data))})
	return start
}

func (s *source) truncate(at int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pos >= at {
		return false
	}
	i := len(s.segs)
	for i > 0 && s.segs[i-1].start >= at {
		i--
	}
	if i == 0 {
		return false // não tira a primeira faixa
	}
	s.segs = s.segs[:i]
	return true
}

func (s *source) release(before int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.segs {
		if s.segs[i].start+s.segs[i].size <= before {
			s.segs[i].data = nil
		}
	}
}

func (s *source) Read(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	end := s.end()
	if s.pos >= end {
		return 0, io.EOF
	}
	n := 0
	for n < len(b) && s.pos < end {
		seg := s.segs[0]
		for _, sg := range s.segs { // a faixa em pos, ou a próxima depois de um intervalo
			if s.pos < sg.start+sg.size {
				seg = sg
				break
			}
		}
		var k int
		switch {
		case s.pos < seg.start: // intervalo: silêncio
			k = int(min(int64(len(b)-n), seg.start-s.pos))
			clear(b[n : n+k])
		case seg.data == nil: // faixa já liberada (não deveria ser lida de novo)
			k = int(min(int64(len(b)-n), seg.start+seg.size-s.pos))
			clear(b[n : n+k])
		default:
			k = copy(b[n:], seg.data[s.pos-seg.start:])
		}
		n += k
		s.pos += int64(k)
	}
	return n, nil
}

func (s *source) Seek(off int64, whence int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch whence {
	case io.SeekCurrent:
		off += s.pos
	case io.SeekEnd:
		off += s.end()
	}
	off -= off % bytesPerFrame
	if off < 0 {
		return 0, errors.New("posição negativa")
	}
	s.pos = off
	return off, nil
}

func (s *source) Pos() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pos
}
