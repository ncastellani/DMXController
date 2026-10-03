package dmx

import (
	"errors"
	"testing"
	"time"
)

type flaky struct {
	sent int
	fail bool
}

func (f *flaky) Send(*Frame) error {
	if f.fail {
		return errors.New("cabo solto")
	}
	f.sent++
	return nil
}
func (f *flaky) Close() error { return nil }

func TestResilientReconnects(t *testing.T) {
	dev := &flaky{}
	plugged := false
	var events []bool
	r := NewResilient(func() (Output, error) {
		if !plugged {
			return nil, errors.New("interface não encontrada")
		}
		dev.fail = false
		return dev, nil
	}, func(ok bool, err error) { events = append(events, ok) })
	r.RetryEvery = 0

	var f Frame
	if err := r.Send(&f); err != nil {
		t.Fatal("Send não deveria devolver erro sem interface")
	}
	if ok, err := r.Status(); ok || err == nil {
		t.Fatal("deveria estar desconectado com erro")
	}

	plugged = true
	r.Send(&f)
	if ok, _ := r.Status(); !ok || dev.sent != 1 {
		t.Fatalf("deveria reconectar e enviar (sent=%d)", dev.sent)
	}

	dev.fail = true // cabo puxado no meio do show
	r.Send(&f)
	if ok, _ := r.Status(); ok {
		t.Fatal("falha no envio deveria marcar desconectado")
	}
	r.Send(&f) // reabre
	if ok, _ := r.Status(); !ok {
		t.Fatal("deveria reconectar de novo")
	}
	if want := []bool{false, true, false, true}; len(events) != len(want) {
		t.Fatalf("eventos = %v, quero %v", events, want)
	}
}

func TestResilientKeepsPace(t *testing.T) {
	r := NewResilient(func() (Output, error) { return nil, errors.New("sem interface") }, nil)
	start := time.Now()
	var f Frame
	r.Send(&f)
	if time.Since(start) < frameTime/2 {
		t.Fatal("sem interface, Send deveria esperar o tempo de um quadro")
	}
}

func TestResilientReportsInitialSuccess(t *testing.T) {
	var events []bool
	NewResilient(func() (Output, error) { return &flaky{}, nil }, func(ok bool, err error) { events = append(events, ok) })
	if len(events) != 1 || !events[0] {
		t.Fatalf("conectar de primeira deveria avisar ok: %v", events)
	}
}
