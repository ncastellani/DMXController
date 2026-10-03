package dmx

import (
	"fmt"
	"io"
	"time"
)

// Logger escreve os quadros enviados (só quando mudam, no máximo um a cada
// Every), para acompanhar o DMX no terminal. Format descreve o quadro numa
// linha.
type Logger struct {
	Output
	W      io.Writer
	Every  time.Duration
	Format func(f *Frame) string

	last   Frame
	lastAt time.Time
	first  bool
}

// NewLogger embrulha out; format descreve um quadro numa linha.
func NewLogger(out Output, w io.Writer, format func(*Frame) string) *Logger {
	return &Logger{Output: out, W: w, Every: 100 * time.Millisecond, Format: format, first: true}
}

// Send envia o quadro e, se ele mudou desde a última linha, a escreve.
func (l *Logger) Send(f *Frame) error {
	err := l.Output.Send(f)
	if now := time.Now(); (l.first || *f != l.last) && now.Sub(l.lastAt) >= l.Every {
		fmt.Fprintf(l.W, "%s  %s\n", now.Format("15:04:05.000"), l.Format(f))
		l.last, l.lastAt, l.first = *f, now, false
	}
	return err
}
