package dmx

import (
	"fmt"
	"io"
	"strings"
)

// Open abre a saída indicada:
//   - "mock": simulador (escreve as mudanças em mockLog, se não for nil);
//   - "auto": a primeira interface FTDI encontrada;
//   - outro valor: o caminho da porta serial de uma interface Open DMX.
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
