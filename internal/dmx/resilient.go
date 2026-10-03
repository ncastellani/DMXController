package dmx

import (
	"sync"
	"time"
)

// Resilient é uma saída que sobrevive à interface ser desconectada: se abrir
// ou enviar falhar, descarta os quadros e tenta reabrir a cada RetryEvery.
// O show (música, interface web) continua rodando enquanto isso.
type Resilient struct {
	open       func() (Output, error)
	RetryEvery time.Duration

	mu       sync.Mutex
	out      Output
	err      error
	reported bool // o estado inicial já foi informado
	lastTry  time.Time
	onChange func(ok bool, err error)
}

// NewResilient cria a saída; open é chamado para (re)abrir a interface.
// onChange (opcional) é chamado quando a saída conecta ou cai.
func NewResilient(open func() (Output, error), onChange func(ok bool, err error)) *Resilient {
	r := &Resilient{open: open, RetryEvery: time.Second, onChange: onChange}
	r.tryOpen(time.Now())
	return r
}

func (r *Resilient) tryOpen(now time.Time) {
	r.lastTry = now
	out, err := r.open()
	if err != nil {
		r.setErr(err)
		return
	}
	r.out = out
	r.setErr(nil)
}

// setErr registra o estado e avisa se mudou. Requer r.mu (ou construção).
func (r *Resilient) setErr(err error) {
	changed := !r.reported || (err == nil) != (r.err == nil) || (err != nil && r.err != nil && err.Error() != r.err.Error())
	r.err, r.reported = err, true
	if changed && r.onChange != nil {
		r.onChange(err == nil, err)
	}
}

// Send envia o quadro; nunca devolve erro (a falha vira estado + nova tentativa).
func (r *Resilient) Send(frame *Frame) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.out == nil {
		if now := time.Now(); now.Sub(r.lastTry) >= r.RetryEvery {
			r.tryOpen(now)
		}
		if r.out == nil {
			time.Sleep(frameTime) // mantém o ritmo do Refresher
			return nil
		}
	}
	if err := r.out.Send(frame); err != nil {
		r.out.Close()
		r.out = nil
		r.lastTry = time.Now()
		r.setErr(err)
	}
	return nil
}

// Status informa se a interface está conectada e o último erro.
func (r *Resilient) Status() (ok bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.out != nil, r.err
}

// Close fecha a interface, se aberta.
func (r *Resilient) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.out == nil {
		return nil
	}
	err := r.out.Close()
	r.out = nil
	return err
}
