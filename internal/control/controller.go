package control

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"luzes/internal/dmx"
	"luzes/internal/engine"
	"luzes/internal/fixture"
)

// IdentifyTime é quanto tempo um canhão fica aceso ao ser identificado.
const IdentifyTime = 3 * time.Second

// Controller concentra o estado do show e as regras (toggle, solo, tetos de
// segurança). As conexões web chamam seus métodos de goroutines diferentes.
type Controller struct {
	mu        sync.Mutex
	c         *Console
	eng       *engine.Engine
	rig       *fixture.Rig
	listeners []func(string)
	pending   []string

	active      [engine.NumLayers]*Button // base e strobe
	blackout    bool
	strobeSince time.Time
	holds       []hold  // flashes segurados; o último vence
	flashing    *Button // flash tocando no engine
	identify    int     // posição sendo identificada, -1 se nenhuma
	identUntil  time.Time
}

// hold é um botão de flash segurado por alguém (uma conexão web).
type hold struct {
	holder  string
	btn     *Button
	since   time.Time
	expires time.Time // zero: quem segura avisa quando soltar
	capped  bool      // teto atingido: não toca até ser solto
}

// NewController cria o controlador e aplica o mapeamento de posições do rig.
func NewController(c *Console, eng *engine.Engine, rig *fixture.Rig) *Controller {
	eng.SetMoves(rig.Moves())
	return &Controller{c: c, eng: eng, rig: rig, identify: -1}
}

// Console devolve os botões do show.
func (ctl *Controller) Console() *Console { return ctl.c }

// OnEvent registra uma função que recebe as mensagens de evento. Ela é
// chamada fora do lock, mas não deve bloquear.
func (ctl *Controller) OnEvent(f func(string)) {
	ctl.mu.Lock()
	ctl.listeners = append(ctl.listeners, f)
	ctl.mu.Unlock()
}

func (ctl *Controller) lock() { ctl.mu.Lock() }

// unlock libera o lock e entrega os eventos acumulados.
func (ctl *Controller) unlock() {
	msgs, ls := ctl.pending, ctl.listeners
	ctl.pending = nil
	ctl.mu.Unlock()
	for _, m := range msgs {
		for _, l := range ls {
			l(m)
		}
	}
}

func (ctl *Controller) logf(format string, args ...any) {
	ctl.pending = append(ctl.pending, fmt.Sprintf(format, args...))
}

// Notice publica uma mensagem avulsa.
func (ctl *Controller) Notice(format string, args ...any) {
	ctl.lock()
	defer ctl.unlock()
	ctl.logf(format, args...)
}

// Press aciona um botão de base ou strobe: liga, ou desliga se já estava
// ligado. Na mesma camada, só um botão por vez.
func (ctl *Controller) Press(btn *Button, now time.Time) {
	if btn.Layer == engine.Flash {
		return // flash usa FlashDown/FlashUp
	}
	ctl.lock()
	defer ctl.unlock()
	if ctl.active[btn.Layer] == btn {
		ctl.stop(btn.Layer)
		return
	}
	ctl.active[btn.Layer] = btn
	ctl.eng.Play(btn.Layer, btn.Name, btn.Src, now)
	if btn.Layer == engine.Strobe {
		ctl.strobeSince = now
	}
	ctl.logf("▶ %s", btn.Label())
}

func (ctl *Controller) stop(l engine.Layer) {
	ctl.logf("■ %s", ctl.active[l].Label())
	ctl.active[l] = nil
	ctl.eng.Stop(l)
}

// FlashDown começa a segurar btn. ttl > 0 faz o hold expirar se não for
// renovado (Refresh) a tempo — proteção contra conexões que caem.
func (ctl *Controller) FlashDown(holder string, btn *Button, ttl time.Duration, now time.Time) {
	if btn.Layer != engine.Flash {
		return
	}
	ctl.lock()
	defer ctl.unlock()
	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}
	for i := range ctl.holds {
		if h := &ctl.holds[i]; h.holder == holder && h.btn == btn {
			h.expires = exp
			return
		}
	}
	ctl.holds = append(ctl.holds, hold{holder: holder, btn: btn, since: now, expires: exp})
	ctl.applyFlash(now)
}

// FlashUp solta btn (ou todos os botões do holder, se btn for nil).
func (ctl *Controller) FlashUp(holder string, btn *Button, now time.Time) {
	ctl.lock()
	defer ctl.unlock()
	ctl.holds = slices.DeleteFunc(ctl.holds, func(h hold) bool {
		return h.holder == holder && (btn == nil || h.btn == btn)
	})
	ctl.applyFlash(now)
}

// Refresh renova os holds com expiração de um holder.
func (ctl *Controller) Refresh(holder string, ttl time.Duration, now time.Time) {
	ctl.lock()
	defer ctl.unlock()
	for i := range ctl.holds {
		if h := &ctl.holds[i]; h.holder == holder && !h.expires.IsZero() {
			h.expires = now.Add(ttl)
		}
	}
}

// applyFlash coloca no engine o hold mais recente que não atingiu o teto.
func (ctl *Controller) applyFlash(now time.Time) {
	var top *Button
	for i := len(ctl.holds) - 1; i >= 0; i-- {
		if !ctl.holds[i].capped {
			top = ctl.holds[i].btn
			break
		}
	}
	if top == ctl.flashing {
		return
	}
	ctl.flashing = top
	if top == nil {
		ctl.eng.Stop(engine.Flash)
	} else {
		ctl.eng.Play(engine.Flash, top.Name, top.Src, now)
	}
}

// ToggleBlackout liga/desliga o blackout geral.
func (ctl *Controller) ToggleBlackout() {
	ctl.lock()
	defer ctl.unlock()
	ctl.blackout = !ctl.blackout
	ctl.eng.SetBlackout(ctl.blackout)
	ctl.logf("blackout %s", onOff(ctl.blackout))
}

// StopStrobe desliga o strobo contínuo.
func (ctl *Controller) StopStrobe() {
	ctl.lock()
	defer ctl.unlock()
	if ctl.active[engine.Strobe] != nil {
		ctl.stop(engine.Strobe)
	}
}

// StopAll desliga tudo: camadas, flashes, canais manuais e blackout.
func (ctl *Controller) StopAll() {
	ctl.lock()
	defer ctl.unlock()
	ctl.eng.StopAll()
	ctl.active = [engine.NumLayers]*Button{}
	ctl.holds = nil
	ctl.flashing = nil
	ctl.blackout = false
	ctl.identify = -1
	ctl.logf("parar tudo")
}

// Tick aplica expirações e tetos de segurança. Chamar com frequência.
func (ctl *Controller) Tick(now time.Time) {
	ctl.lock()
	defer ctl.unlock()
	ctl.holds = slices.DeleteFunc(ctl.holds, func(h hold) bool {
		return !h.expires.IsZero() && now.After(h.expires)
	})
	for i := range ctl.holds {
		if h := &ctl.holds[i]; !h.capped && now.Sub(h.since) >= ctl.c.Limits.Flash {
			h.capped = true
			ctl.logf("flash desligado: teto de %v (solte o botão)", ctl.c.Limits.Flash)
		}
	}
	ctl.applyFlash(now)
	if ctl.active[engine.Strobe] != nil && now.Sub(ctl.strobeSince) >= ctl.c.Limits.Strobe {
		ctl.logf("strobe desligado: teto de %v", ctl.c.Limits.Strobe)
		ctl.stop(engine.Strobe)
	}
	if ctl.identify >= 0 && now.After(ctl.identUntil) {
		ctl.identify = -1
		ctl.eng.Stop(engine.Identify)
	}
}

// SetManual fixa o canal ch (0-based no perfil) do canhão na posição pos.
func (ctl *Controller) SetManual(pos, ch int, v byte) error {
	ctl.lock()
	defer ctl.unlock()
	if pos < 0 || pos >= len(ctl.rig.Fixtures) {
		return fmt.Errorf("posição %d inválida", pos)
	}
	f := ctl.rig.Logical(pos)
	if ch < 0 || ch >= len(f.Profile.Channels) {
		return fmt.Errorf("canal %d inválido", ch)
	}
	ctl.eng.SetManual(f.Address+ch, v)
	return nil
}

// ClearManual solta os canais manuais da posição pos (-1 = todas).
func (ctl *Controller) ClearManual(pos int) {
	ctl.lock()
	defer ctl.unlock()
	for i := range ctl.rig.Fixtures {
		if pos == -1 || pos == i {
			f := ctl.rig.Logical(i)
			ctl.eng.ClearManual(f.Address, f.Address+len(f.Profile.Channels)-1)
		}
	}
}

// Swap troca os canhões das posições i e j e grava o patch.
func (ctl *Controller) Swap(i, j int) error {
	ctl.lock()
	defer ctl.unlock()
	if err := ctl.rig.Swap(i, j); err != nil {
		return err
	}
	ctl.eng.SetMoves(ctl.rig.Moves())
	a, b := ctl.rig.Fixtures[i], ctl.rig.Fixtures[j]
	ctl.logf("posições: %s = d%03d, %s = d%03d", a.Name, a.Address, b.Name, b.Address)
	if err := ctl.rig.Save(); err != nil {
		ctl.logf("erro ao gravar o patch: %v", err)
		return err
	}
	return nil
}

// Identify acende em branco só o canhão da posição pos por IdentifyTime.
func (ctl *Controller) Identify(pos int, now time.Time) error {
	ctl.lock()
	defer ctl.unlock()
	if pos < 0 || pos >= len(ctl.rig.Fixtures) {
		return fmt.Errorf("posição %d inválida", pos)
	}
	ctl.identify, ctl.identUntil = pos, now.Add(IdentifyTime)
	ctl.eng.Play(engine.Identify, "identificar", white{ctl.rig.Logical(pos)}, now)
	return nil
}

// AllWhite devolve um Source que acende todos os canhões em branco total
// (usado pela calibração de atraso).
func (ctl *Controller) AllWhite() engine.Source {
	ctl.lock()
	defer ctl.unlock()
	var all multi
	for i := range ctl.rig.Fixtures {
		all = append(all, white{ctl.rig.Logical(i)})
	}
	return all
}

type multi []engine.Source

func (m multi) Render(e time.Duration, f *dmx.Frame) {
	for _, s := range m {
		s.Render(e, f)
	}
}

// white acende um fixture em branco total.
type white struct{ f *fixture.Fixture }

func (w white) Render(_ time.Duration, f *dmx.Frame) {
	w.f.Render(f, fixture.State{Color: fixture.Color{R: 255, G: 255, B: 255}, Intensity: 1})
}

// State é um retrato do estado do show.
type State struct {
	Active    [engine.NumLayers]*Button // Identify fica nil
	Blackout  bool
	Identify  int // posição sendo identificada, -1 se nenhuma
	Positions []Position
}

// Position descreve o canhão em uma posição do palco.
type Position struct {
	Name    string
	Address int              // endereço DMX físico
	Logical *fixture.Fixture // fixture com endereço lógico (quadro do engine)
	Manual  []bool           // canais em controle manual
}

// State devolve o estado atual.
func (ctl *Controller) State() State {
	ctl.lock()
	defer ctl.unlock()
	s := State{Active: ctl.active, Blackout: ctl.blackout, Identify: ctl.identify}
	s.Active[engine.Flash] = ctl.flashing
	for i, f := range ctl.rig.Fixtures {
		lf := ctl.rig.Logical(i)
		manual := make([]bool, len(lf.Profile.Channels))
		for ch := range manual {
			manual[ch] = ctl.eng.Manual(lf.Address + ch)
		}
		s.Positions = append(s.Positions, Position{Name: f.Name, Address: f.Address, Logical: lf, Manual: manual})
	}
	return s
}

func onOff(b bool) string {
	if b {
		return "ligado"
	}
	return "desligado"
}
