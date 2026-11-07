package dmx

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Tempos do quadro. Os drivers FTDI ligam/desligam o break por mensagens USB,
// então os valores reais ficam na casa de ~1 ms (bem acima do mínimo e
// dentro da norma).
const (
	breakTime = 120 * time.Microsecond
	mabTime   = 12 * time.Microsecond
)

// Open abre a saída indicada:
//   - "mock": simulador (escreve as mudanças em mockLog, se não for nil);
//   - "auto": a primeira interface FTDI encontrada;
//   - outro valor: a porta serial de uma interface Open DMX (/dev/ttyUSB0 no
//     Linux, /dev/cu.usbserial-XXXX no Mac, COM3 no Windows).
func Open(device string, mockLog io.Writer) (Output, error) {
	switch device {
	case "mock":
		return NewMock(mockLog), nil
	case "auto":
		found := detect()
		if len(found) == 0 {
			return nil, fmt.Errorf("nenhuma interface DMX (FTDI) encontrada; conecte a SL1000 ou use -dev <porta> / -dev mock")
		}
		if len(found) > 1 {
			return nil, fmt.Errorf("mais de uma porta serial encontrada (%s); escolha com -dev", strings.Join(found, ", "))
		}
		device = found[0]
	}
	return OpenOpenDMX(device)
}
