// Package engine compõe as camadas de iluminação e gera o quadro DMX final.
package engine

import (
	"context"
	"sync"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
)

// Source gera valores DMX em função do tempo desde que começou a tocar.
type Source interface {
	Render(elapsed time.Duration, f *dmx.Frame)
}

// Layer é uma camada do engine. Camadas maiores sobrepõem as menores.
type Layer int

const (
	Show     Layer = iota // timeline da música tocando (automático)
	Base                  // modo de pista ou cor fixa
	Strobe                // strobo contínuo (toggle)
	Flash                 // momentâneo, enquanto o botão estiver segurado
	Identify              // acende um canhão para achá-lo no palco
	NumLayers
)

func (l Layer) String() string {
	return [...]string{"show", "base", "strobe", "flash", "identify"}[l]
}

type slot struct {
	name  string
	src   Source
	start time.Time
}

// Status descreve o que está ativo.
type Status struct {
	Names    [NumLayers]string // "" se a camada está livre
	Blackout bool
}

// Engine guarda o que está tocando em cada camada. É seguro para uso
// concorrente: os controles chamam Play/Stop e o loop chama Render.
//
// Ordem de composição: a camada ativa mais alta define o quadro inteiro;
// depois entram os canais manuais (exceto durante Identify); o blackout
// zera tudo. Render produz o quadro lógico; Run aplica os moves para gerar
// o quadro físico enviado à interface.
type Engine struct {
	mu         sync.Mutex
	slots      [NumLayers]slot
	blackout   bool
	manual     dmx.Frame
	manualMask [dmx.Universe]bool
	moves      []dmx.Move
	last       dmx.Frame
}

// Play coloca src na camada l, começando em now.
func (e *Engine) Play(l Layer, name string, src Source, now time.Time) {
	e.mu.Lock()
	e.slots[l] = slot{name: name, src: src, start: now}
	e.mu.Unlock()
}

// Stop libera a camada l.
func (e *Engine) Stop(l Layer) {
	e.mu.Lock()
	e.slots[l] = slot{}
	e.mu.Unlock()
}

// StopAll libera as camadas manuais e os canais manuais e desliga o
// blackout. A camada Show (música tocando) continua: para ela, use o player.
func (e *Engine) StopAll() {
	e.mu.Lock()
	show := e.slots[Show]
	e.slots = [NumLayers]slot{}
	e.slots[Show] = show
	e.blackout = false
	e.manualMask = [dmx.Universe]bool{}
	e.mu.Unlock()
}

// SetBlackout liga ou desliga o blackout geral, que apaga tudo.
func (e *Engine) SetBlackout(on bool) {
	e.mu.Lock()
	e.blackout = on
	e.mu.Unlock()
}

// SetManual fixa o canal ch (1..512 no quadro lógico) em v por cima das camadas.
func (e *Engine) SetManual(ch int, v byte) {
	if ch < 1 || ch > dmx.Universe {
		return
	}
	e.mu.Lock()
	e.manual[ch-1] = v
	e.manualMask[ch-1] = true
	e.mu.Unlock()
}

// ClearManual solta os canais manuais de from até to (inclusive).
func (e *Engine) ClearManual(from, to int) {
	e.mu.Lock()
	for ch := max(from, 1); ch <= min(to, dmx.Universe); ch++ {
		e.manualMask[ch-1] = false
	}
	e.mu.Unlock()
}

// Manual informa se o canal ch está em controle manual.
func (e *Engine) Manual(ch int) bool {
	if ch < 1 || ch > dmx.Universe {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.manualMask[ch-1]
}

// SetMoves define o mapeamento do quadro lógico para o físico (nil = igual).
func (e *Engine) SetMoves(moves []dmx.Move) {
	e.mu.Lock()
	e.moves = moves
	e.mu.Unlock()
}

// Status devolve o estado atual.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	var s Status
	for i, sl := range e.slots {
		s.Names[i] = sl.name
	}
	s.Blackout = e.blackout
	return s
}

// Render calcula o quadro lógico no instante now.
func (e *Engine) Render(now time.Time, f *dmx.Frame) {
	e.mu.Lock()
	defer e.mu.Unlock()
	*f = dmx.Frame{}
	if !e.blackout {
		top := Layer(-1)
		for l := NumLayers - 1; l >= 0; l-- {
			if sl := e.slots[l]; sl.src != nil {
				sl.src.Render(now.Sub(sl.start), f)
				top = l
				break
			}
		}
		if top != Identify {
			for i, on := range e.manualMask {
				if on {
					f[i] = e.manual[i]
				}
			}
		}
	}
	e.last = *f
}

// Last devolve o último quadro lógico calculado.
func (e *Engine) Last() dmx.Frame {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.last
}

// Run calcula quadros a hz por segundo e entrega o quadro físico ao
// Refresher até ctx ser cancelado.
func (e *Engine) Run(ctx context.Context, out *dmx.Refresher, hz int) {
	ticker := time.NewTicker(time.Second / time.Duration(hz))
	defer ticker.Stop()
	var logical, physical dmx.Frame
	for {
		e.Render(time.Now(), &logical)
		e.mu.Lock()
		moves := e.moves
		e.mu.Unlock()
		dmx.Remap(&logical, &physical, moves)
		out.Update(&physical)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
