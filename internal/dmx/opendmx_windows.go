package dmx

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// No Windows, a interface aparece como uma porta COM (driver VCP da FTDI, que
// o Windows instala sozinho). O quadro é o mesmo do Linux e do Mac: BREAK,
// MAB, start code e 512 canais a 250000 baud, 8N2.

// chipDrain é o tempo para esvaziar o buffer de transmissão do FT232R (256
// bytes a 44 µs cada) depois que a fila do driver zera: o próximo BREAK só
// pode começar depois do último canal sair.
const chipDrain = 12 * time.Millisecond

// drainTimeout é o máximo de espera pela fila do driver (um quadro leva
// ~23 ms no fio); passou disso, a interface parou e o Resilient a reabre.
const drainTimeout = 500 * time.Millisecond

// OpenDMX fala com interfaces compatíveis com Enttec Open DMX (FTDI sem
// microcontrolador, ex.: SourLight SL1000) pela porta COM do driver da FTDI.
type OpenDMX struct {
	h   windows.Handle
	buf [1 + Universe]byte // start code + dados
}

// OpenOpenDMX abre a porta COM (ex.: "COM3") e a configura para 250000
// baud, 8N2, sem controle de fluxo.
func OpenOpenDMX(port string) (*OpenDMX, error) {
	name := port
	if !strings.HasPrefix(name, `\\.\`) {
		name = `\\.\` + name // obrigatório a partir da COM10, aceito em todas
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", port, err)
	}
	if err := configure(h); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("configurando %s: %w", port, err)
	}
	return &OpenDMX{h: h}, nil
}

// configure aplica 250000 baud, 8 bits, 2 stop bits e sem paridade. DTR e
// RTS ficam ligados, como o Linux deixa a porta ao abrir (é o estado com
// que a SL1000 foi testada).
func configure(h windows.Handle) error {
	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(h, &dcb); err != nil {
		return err
	}
	dcb.BaudRate = 250000
	dcb.ByteSize = 8
	dcb.Parity = windows.NOPARITY
	dcb.StopBits = windows.TWOSTOPBITS
	// fBinary (obrigatório no Windows) + DTR e RTS ligados; sem controle de
	// fluxo por hardware nem por XON/XOFF, sem abortar em erro.
	dcb.Flags = 0x1 | windows.DTR_CONTROL_ENABLE | windows.RTS_CONTROL_ENABLE
	if err := windows.SetCommState(h, &dcb); err != nil {
		return err
	}
	// Leitura não bloqueia (não lemos nada); escrita desiste depois de 1 s.
	t := windows.CommTimeouts{ReadIntervalTimeout: 0xFFFFFFFF, WriteTotalTimeoutConstant: 1000}
	if err := windows.SetCommTimeouts(h, &t); err != nil {
		return err
	}
	return windows.PurgeComm(h, windows.PURGE_TXCLEAR|windows.PURGE_RXCLEAR)
}

// Send transmite BREAK, MAB, start code 0x00 e os 512 canais, e espera o
// último byte sair da interface antes de retornar.
func (o *OpenDMX) Send(frame *Frame) error {
	if err := windows.EscapeCommFunction(o.h, windows.SETBREAK); err != nil {
		return fmt.Errorf("break: %w", err)
	}
	time.Sleep(breakTime)
	if err := windows.EscapeCommFunction(o.h, windows.CLRBREAK); err != nil {
		return fmt.Errorf("fim do break: %w", err)
	}
	time.Sleep(mabTime)

	o.buf[0] = 0x00
	copy(o.buf[1:], frame[:])
	for b := o.buf[:]; len(b) > 0; {
		var n uint32
		if err := windows.WriteFile(o.h, b, &n, nil); err != nil {
			return fmt.Errorf("escrevendo quadro: %w", err)
		}
		if n == 0 {
			return errors.New("escrevendo quadro: a interface não aceitou os dados")
		}
		b = b[n:]
	}
	return o.drain()
}

// drain espera a fila do driver zerar e o buffer do chip esvaziar.
func (o *OpenDMX) drain() error {
	deadline := time.Now().Add(drainTimeout)
	for {
		var errs uint32
		var st windows.ComStat
		if err := windows.ClearCommError(o.h, &errs, &st); err != nil {
			return fmt.Errorf("esperando a transmissão: %w", err)
		}
		if st.CBOutQue == 0 {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("a interface parou de transmitir")
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(chipDrain)
	return nil
}

// Close fecha a porta.
func (o *OpenDMX) Close() error {
	return windows.CloseHandle(o.h)
}

// detect procura interfaces FTDI conectadas. O driver da FTDI registra cada
// porta como \Device\VCPn em HARDWARE\DEVICEMAP\SERIALCOMM (outras portas
// seriais, como as do Bluetooth, têm outros nomes e ficam de fora).
func detect() []string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DEVICEMAP\SERIALCOMM`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, err := k.ReadValueNames(0)
	if err != nil {
		return nil
	}
	var ports []string
	for _, n := range names {
		if !strings.Contains(strings.ToUpper(n), `\VCP`) {
			continue
		}
		if port, _, err := k.GetStringValue(n); err == nil && port != "" {
			ports = append(ports, port)
		}
	}
	slices.Sort(ports)
	return ports
}
