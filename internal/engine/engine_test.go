package engine

import (
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
)

// fill escreve um valor fixo no canal 1.
type fill byte

func (v fill) Render(_ time.Duration, f *dmx.Frame) { f[0] = byte(v) }

// clock escreve no canal 1 os décimos de segundo desde o início.
type clock struct{}

func (clock) Render(e time.Duration, f *dmx.Frame) { f[0] = byte(e / (100 * time.Millisecond)) }

func ch1(e *Engine, now time.Time) byte {
	var f dmx.Frame
	e.Render(now, &f)
	return f[0]
}

func TestLayers(t *testing.T) {
	var e Engine
	now := time.Now()
	if got := ch1(&e, now); got != 0 {
		t.Fatalf("vazio: %d", got)
	}
	e.Play(Base, "base", fill(10), now)
	e.Play(Flash, "flash", fill(30), now)
	e.Play(Strobe, "strobe", fill(20), now)
	if got := ch1(&e, now); got != 30 {
		t.Fatalf("flash deveria vencer: %d", got)
	}
	e.Stop(Flash)
	if got := ch1(&e, now); got != 20 {
		t.Fatalf("strobe deveria vencer: %d", got)
	}
	e.Stop(Strobe)
	if got := ch1(&e, now); got != 10 {
		t.Fatalf("base: %d", got)
	}
}

func TestBlackoutAndStopAll(t *testing.T) {
	var e Engine
	now := time.Now()
	e.Play(Flash, "flash", fill(30), now)
	e.SetBlackout(true)
	if got := ch1(&e, now); got != 0 {
		t.Fatalf("blackout deveria apagar tudo: %d", got)
	}
	if e.Last()[0] != 0 {
		t.Fatal("Last deveria refletir o blackout")
	}
	e.StopAll()
	st := e.Status()
	if st.Blackout || st.Names[Flash] != "" {
		t.Fatalf("StopAll deveria limpar tudo: %+v", st)
	}
}

func TestElapsedFromPlay(t *testing.T) {
	var e Engine
	start := time.Now()
	e.Play(Base, "seq", clock{}, start)
	if got := ch1(&e, start.Add(500*time.Millisecond)); got != 5 {
		t.Fatalf("elapsed: %d", got)
	}
	// Tocar de novo reinicia do começo.
	e.Play(Base, "seq", clock{}, start.Add(time.Second))
	if got := ch1(&e, start.Add(1200*time.Millisecond)); got != 2 {
		t.Fatalf("reinício: %d", got)
	}
}

func TestShowLayerSurvivesStopAll(t *testing.T) {
	var e Engine
	now := time.Now()
	e.Play(Show, "música", fill(5), now)
	e.Play(Base, "manual", fill(10), now)
	if got := ch1(&e, now); got != 10 {
		t.Fatalf("preset manual deveria ficar por cima do show: %d", got)
	}
	e.StopAll()
	if got := ch1(&e, now); got != 5 {
		t.Fatalf("parar tudo deveria manter o show: %d", got)
	}
}
