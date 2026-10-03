package audio

import (
	"bytes"
	"io"
	"testing"
)

func fill(n int, v byte) []byte { return bytes.Repeat([]byte{v}, n) }

func TestSourceSequenceWithGap(t *testing.T) {
	s := &source{}
	a, b := fill(4*100, 1), fill(4*50, 2)
	if st := s.append(a, 0); st != 0 {
		t.Fatalf("primeira faixa começa em %d", st)
	}
	if st := s.append(b, 4*10); st != 4*110 {
		t.Fatalf("segunda faixa começa em %d, quero %d", st, 4*110)
	}
	got, err := io.ReadAll(readerFunc(func(p []byte) (int, error) { return s.Read(p[:min(len(p), 37)]) })) // leituras picadas
	if err != nil {
		t.Fatal(err)
	}
	want := append(append(append([]byte{}, a...), make([]byte, 4*10)...), b...)
	if !bytes.Equal(got, want) {
		t.Fatalf("fluxo errado: %d bytes, quero %d", len(got), len(want))
	}
}

func TestSourceTruncateAndRelease(t *testing.T) {
	s := &source{}
	s.append(fill(400, 1), 0)
	st := s.append(fill(400, 2), 0)
	buf := make([]byte, 100)
	s.Read(buf)
	if !s.truncate(st) || s.end() != 400 {
		t.Fatal("deveria tirar a faixa ainda não lida")
	}
	st = s.append(fill(400, 3), 0)
	s.Seek(st+4, io.SeekStart)
	if s.truncate(st) {
		t.Fatal("não pode tirar uma faixa que já começou a ser lida")
	}
	s.release(st)
	if s.segs[0].data != nil || s.segs[1].data == nil {
		t.Fatal("release deveria liberar só a faixa que terminou")
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
