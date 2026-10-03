package dmx

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// configure aplica 250000 baud via termios2 (BOTHER), aceito pelo ftdi_sio.
func configure(fd int) error {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS2)
	if err != nil {
		return err
	}
	raw(t)
	t.Cflag &^= unix.CBAUD | unix.CBAUD<<unix.IBSHIFT
	t.Cflag |= unix.BOTHER | unix.BOTHER<<unix.IBSHIFT
	t.Ispeed = 250000
	t.Ospeed = 250000
	return unix.IoctlSetTermios(fd, unix.TCSETS2, t)
}

// drain espera a transmissão terminar (TCSBRK com argumento != 0 = tcdrain).
func drain(fd int) error {
	return ioctl(fd, unix.TCSBRK, 1)
}

// detect procura uma interface FTDI conectada.
func detect() []string {
	m, _ := filepath.Glob("/dev/serial/by-id/usb-FTDI_*")
	if len(m) == 0 {
		m, _ = filepath.Glob("/dev/ttyUSB*")
	}
	return m
}
