package dmx

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Mock simula a interface: respeita o tempo de um quadro real e, se W não for
// nil, escreve os canais não-zero sempre que o quadro muda.
type Mock struct {
	W io.Writer

	last  Frame
	first bool
}

// NewMock cria um Mock que escreve em w (pode ser nil).
func NewMock(w io.Writer) *Mock {
	return &Mock{W: w, first: true}
}

// frameTime é a duração aproximada de um quadro completo a 250 kbaud.
const frameTime = 23 * time.Millisecond

func (m *Mock) Send(frame *Frame) error {
	time.Sleep(frameTime)
	if m.W != nil && (m.first || *frame != m.last) {
		fmt.Fprintln(m.W, "[mock dmx]", describe(frame))
	}
	m.last = *frame
	m.first = false
	return nil
}

func (m *Mock) Close() error { return nil }

// Last devolve o último quadro enviado.
func (m *Mock) Last() Frame { return m.last }

func describe(frame *Frame) string {
	var parts []string
	for i, v := range frame {
		if v != 0 {
			parts = append(parts, fmt.Sprintf("%d=%d", i+1, v))
		}
	}
	if len(parts) == 0 {
		return "(tudo zero)"
	}
	return strings.Join(parts, " ")
}
