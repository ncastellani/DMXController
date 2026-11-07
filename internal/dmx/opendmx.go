//go:build linux || darwin

package dmx

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// OpenDMX fala com interfaces compatíveis com Enttec Open DMX (FTDI sem
// microcontrolador, ex.: SourLight SL1000) pelo driver serial do sistema.
// A parte específica de cada sistema (baud de 250000, break, drain) está em
// opendmx_linux.go e opendmx_darwin.go; o Windows tem a sua versão inteira
// em opendmx_windows.go.
type OpenDMX struct {
	fd  int
	buf [1 + Universe]byte // start code + dados
}

// OpenOpenDMX abre a porta serial e a configura para 250000 baud, 8N2.
func OpenOpenDMX(path string) (*OpenDMX, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", path, err)
	}
	if err := configure(fd); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("configurando %s: %w", path, err)
	}
	return &OpenDMX{fd: fd}, nil
}

// raw ajusta flags comuns de termios: modo raw, 8 bits, 2 stop bits, sem
// paridade e sem controle de fluxo.
func raw(t *unix.Termios) {
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON | unix.IXOFF | unix.IXANY
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB | unix.CRTSCTS
	t.Cflag |= unix.CS8 | unix.CSTOPB | unix.CLOCAL | unix.CREAD
	t.Cc[unix.VMIN] = 0
	t.Cc[unix.VTIME] = 0
}

// Send transmite BREAK, MAB, start code 0x00 e os 512 canais, e espera o
// último byte sair da interface antes de retornar.
func (o *OpenDMX) Send(frame *Frame) error {
	if err := ioctl(o.fd, unix.TIOCSBRK, 0); err != nil {
		return fmt.Errorf("break: %w", err)
	}
	time.Sleep(breakTime)
	if err := ioctl(o.fd, unix.TIOCCBRK, 0); err != nil {
		return fmt.Errorf("fim do break: %w", err)
	}
	time.Sleep(mabTime)

	o.buf[0] = 0x00
	copy(o.buf[1:], frame[:])
	for b := o.buf[:]; len(b) > 0; {
		n, err := unix.Write(o.fd, b)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return fmt.Errorf("escrevendo quadro: %w", err)
		}
		b = b[n:]
	}
	if err := drain(o.fd); err != nil {
		return fmt.Errorf("tcdrain: %w", err)
	}
	return nil
}

// ioctl repete a chamada se ela for interrompida por um sinal (o runtime do
// Go envia sinais de preempção que interrompem ioctls bloqueantes do tty).
func ioctl(fd int, req uint, arg int) error {
	for {
		err := unix.IoctlSetInt(fd, req, arg)
		if err != unix.EINTR {
			return err
		}
	}
}

// Close fecha a porta.
func (o *OpenDMX) Close() error {
	return unix.Close(o.fd)
}
