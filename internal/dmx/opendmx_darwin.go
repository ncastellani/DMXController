package dmx

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ioSSIOSpeed é IOSSIOSPEED (<IOKit/serial/ioss.h>): _IOW('T', 2, speed_t).
// É o jeito oficial do macOS de pedir um baud fora da tabela padrão.
const ioSSIOSpeed = 0x80085402

// configure aplica o modo raw e depois 250000 baud via IOSSIOSPEED (o
// tcsetattr do macOS recusa velocidades não padrão em vários drivers).
func configure(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return err
	}
	raw(t)
	t.Ispeed = 9600 // provisório; IOSSIOSPEED troca logo abaixo
	t.Ospeed = 9600
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, t); err != nil {
		return err
	}
	speed := uint64(250000) // speed_t = unsigned long
	for {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), ioSSIOSpeed, uintptr(unsafe.Pointer(&speed)))
		if errno != unix.EINTR {
			if errno != 0 {
				return errno
			}
			return nil
		}
	}
}

// drain espera a transmissão terminar.
func drain(fd int) error {
	return ioctl(fd, unix.TIOCDRAIN, 0)
}

// detect procura uma interface FTDI conectada (driver da Apple ou da FTDI).
func detect() []string {
	m, _ := filepath.Glob("/dev/cu.usbserial-*")
	return m
}
