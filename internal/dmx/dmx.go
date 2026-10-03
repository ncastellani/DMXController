// Package dmx define a saída DMX512 (um universo) e o loop de reenvio contínuo.
package dmx

import (
	"context"
	"sync"
	"time"
)

// Universe é o tamanho de um universo DMX512.
const Universe = 512

// Frame contém os 512 canais de um universo. O índice 0 é o canal 1.
type Frame [Universe]byte

// Set grava value no canal DMX ch (1..512). Canais fora da faixa são ignorados.
func (f *Frame) Set(ch int, value byte) {
	if ch >= 1 && ch <= Universe {
		f[ch-1] = value
	}
}

// Output envia quadros completos para o hardware (ou simulador).
// Send bloqueia até o quadro ter sido transmitido.
type Output interface {
	Send(frame *Frame) error
	Close() error
}

// Refresher mantém o último quadro e o reenvia continuamente numa goroutine
// própria, para que o resto do programa nunca atrase o sinal DMX.
type Refresher struct {
	out    Output
	period time.Duration

	mu    sync.Mutex
	frame Frame
}

// NewRefresher cria um Refresher que envia até hz quadros por segundo.
func NewRefresher(out Output, hz int) *Refresher {
	return &Refresher{out: out, period: time.Second / time.Duration(hz)}
}

// Update troca o quadro que será enviado a partir do próximo ciclo.
func (r *Refresher) Update(frame *Frame) {
	r.mu.Lock()
	r.frame = *frame
	r.mu.Unlock()
}

// Run envia quadros até ctx ser cancelado. Ao sair, envia um blackout.
func (r *Refresher) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.period)
	defer ticker.Stop()
	for {
		r.mu.Lock()
		frame := r.frame
		r.mu.Unlock()
		if err := r.out.Send(&frame); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			var black Frame
			return r.out.Send(&black)
		case <-ticker.C:
		}
	}
}

// Move copia Len canais a partir de From para a partir de To (canais 1..512).
type Move struct{ From, To, Len int }

// Remap monta dst a partir de src aplicando os moves. Canais não cobertos
// ficam em zero. Sem moves, dst é uma cópia de src.
func Remap(src, dst *Frame, moves []Move) {
	if moves == nil {
		*dst = *src
		return
	}
	*dst = Frame{}
	for _, m := range moves {
		for i := 0; i < m.Len; i++ {
			if v := m.From + i; v >= 1 && v <= Universe {
				dst.Set(m.To+i, src[v-1])
			}
		}
	}
}
