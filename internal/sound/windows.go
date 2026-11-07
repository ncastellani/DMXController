package sound

import (
	"context"
	"errors"
	"time"
)

// Windows não tem ferramentas de linha de comando para trocar a saída de som
// nem para parear caixas Bluetooth. O player toca na saída padrão do Windows
// e a acompanha quando ela muda (o WASAPI do oto troca sozinho); a saída e
// a caixa se escolhem nas Configurações do Windows. Atraso e volume
// continuam pela interface, porque são do próprio programa.
type Windows struct{}

// WindowsOutput é a única saída mostrada no Windows: a padrão do sistema.
const WindowsOutput = "padrao-do-windows"

var (
	errWindowsOutput    = errors.New("no Windows, escolha a saída em Configurações → Sistema → Som; o programa acompanha a saída padrão")
	errWindowsBluetooth = errors.New("no Windows, pareie e conecte a caixa em Configurações → Bluetooth e dispositivos")
)

// NewWindows cria o backend do Windows.
func NewWindows() *Windows { return &Windows{} }

func (*Windows) Missing() []string { return nil }

func (*Windows) Outputs(context.Context) ([]Output, error) {
	return []Output{{ID: WindowsOutput, Name: "Saída padrão do Windows", Kind: "other", Default: true}}, nil
}

func (*Windows) SetOutput(_ context.Context, id string) error {
	if id == WindowsOutput {
		return nil
	}
	return errWindowsOutput
}

func (*Windows) Devices(context.Context) ([]Device, error) { return nil, nil }

func (*Windows) Scan(context.Context, time.Duration) ([]Device, error) {
	return nil, errWindowsBluetooth
}

func (*Windows) Pair(context.Context, string) error       { return errWindowsBluetooth }
func (*Windows) Connect(context.Context, string) error    { return errWindowsBluetooth }
func (*Windows) Disconnect(context.Context, string) error { return errWindowsBluetooth }
func (*Windows) Forget(context.Context, string) error     { return errWindowsBluetooth }
