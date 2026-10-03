package sound

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Linux usa PipeWire (pw-dump, wpctl) e BlueZ (bluetoothctl).
type Linux struct {
	run runner
}

// NewLinux cria o backend do Linux.
func NewLinux() *Linux { return &Linux{run: execRun} }

func (l *Linux) Missing() []string {
	var miss []string
	for _, t := range []string{"pw-dump", "wpctl", "bluetoothctl"} {
		if _, ok := lookPath(t); !ok {
			miss = append(miss, t)
		}
	}
	return miss
}

type pwObject struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info *struct {
		Props map[string]any `json:"props"`
	} `json:"info"`
	Props    map[string]any `json:"props"`
	Metadata []struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	} `json:"metadata"`
}

// sinks lê as saídas e o nome da saída padrão do pw-dump.
func (l *Linux) sinks(ctx context.Context) (outs []Output, ids map[string]int, err error) {
	data, err := l.run(ctx, "pw-dump")
	if err != nil {
		return nil, nil, fmt.Errorf("pw-dump: %v", err)
	}
	return parsePWDump(data)
}

func parsePWDump(data []byte) ([]Output, map[string]int, error) {
	var objs []pwObject
	if err := json.Unmarshal(data, &objs); err != nil {
		return nil, nil, fmt.Errorf("pw-dump: %v", err)
	}
	var def string
	ids := map[string]int{}
	var outs []Output
	for _, o := range objs {
		if o.Type == "PipeWire:Interface:Metadata" && o.Props["metadata.name"] == "default" {
			for _, m := range o.Metadata {
				if m.Key == "default.audio.sink" {
					var v struct{ Name string }
					json.Unmarshal(m.Value, &v)
					def = v.Name
				}
			}
		}
		if o.Info == nil || o.Info.Props["media.class"] != "Audio/Sink" {
			continue
		}
		p := o.Info.Props
		name, _ := p["node.name"].(string)
		desc, _ := p["node.description"].(string)
		out := Output{ID: name, Name: desc, Kind: kindOf(name + " " + desc)}
		if p["device.api"] == "bluez5" {
			out.Kind = "bluetooth"
		}
		ids[name] = o.ID
		outs = append(outs, out)
	}
	for i := range outs {
		outs[i].Default = outs[i].ID == def
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].Name < outs[j].Name })
	return outs, ids, nil
}

func (l *Linux) Outputs(ctx context.Context) ([]Output, error) {
	outs, _, err := l.sinks(ctx)
	return outs, err
}

func (l *Linux) SetOutput(ctx context.Context, id string) error {
	_, ids, err := l.sinks(ctx)
	if err != nil {
		return err
	}
	n, ok := ids[id]
	if !ok {
		return fmt.Errorf("saída %q não encontrada", id)
	}
	if out, err := l.run(ctx, "wpctl", "set-default", fmt.Sprint(n)); err != nil {
		return fmt.Errorf("wpctl: %v %s", err, clean(out))
	}
	return nil
}

var btDevLine = regexp.MustCompile(`^Device ((?:[0-9A-F]{2}:){5}[0-9A-F]{2}) (.*)$`)

// Devices lista os aparelhos de áudio conhecidos pelo BlueZ.
func (l *Linux) Devices(ctx context.Context) ([]Device, error) {
	data, err := l.run(ctx, "bluetoothctl", "devices")
	if err != nil {
		return nil, fmt.Errorf("bluetoothctl: %v %s", err, clean(data))
	}
	var devs []Device
	for _, line := range strings.Split(clean(data), "\n") {
		m := btDevLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		info, err := l.run(ctx, "bluetoothctl", "info", m[1])
		if err != nil {
			continue
		}
		d, audio := parseBTInfo(m[1], clean(info))
		if audio {
			devs = append(devs, d)
		}
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].Name < devs[j].Name })
	return devs, nil
}

// parseBTInfo lê a saída de "bluetoothctl info"; audio informa se parece
// um aparelho de som (ícone audio-* ou perfil Audio Sink).
func parseBTInfo(addr, info string) (d Device, audio bool) {
	d = Device{Address: addr, Name: addr}
	for _, line := range strings.Split(info, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok {
			continue
		}
		switch k {
		case "Name":
			d.Name = v
		case "Alias":
			if d.Name == addr {
				d.Name = v
			}
		case "Paired":
			d.Paired = v == "yes"
		case "Connected":
			d.Connected = v == "yes"
		case "Icon":
			audio = audio || strings.HasPrefix(v, "audio-")
		case "UUID":
			audio = audio || strings.HasPrefix(v, "Audio Sink")
		}
	}
	return d, audio
}

func (l *Linux) Scan(ctx context.Context, d time.Duration) ([]Device, error) {
	secs := fmt.Sprint(int(d.Seconds()))
	// Caixas usam Bluetooth clássico; "bredr" evita a enxurrada de aparelhos BLE.
	l.run(ctx, "bluetoothctl", "--timeout", secs, "scan", "bredr")
	return l.Devices(ctx)
}

// Pair roda o bluetoothctl interativo (o agente de pareamento só existe
// nesse modo) e espera o resultado de cada passo.
func (l *Linux) Pair(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bluetoothctl")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	defer cmd.Wait()
	defer stdin.Close()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- clean(sc.Bytes())
		}
		close(lines)
	}()
	step := func(command string, ok, fail []string) error {
		fmt.Fprintln(stdin, command)
		for {
			select {
			case <-ctx.Done():
				return fmt.Errorf("%s: tempo esgotado", command)
			case line, open := <-lines:
				if !open {
					return fmt.Errorf("%s: bluetoothctl encerrou", command)
				}
				for _, s := range ok {
					if strings.Contains(line, s) {
						return nil
					}
				}
				for _, s := range fail {
					if strings.Contains(line, s) {
						return fmt.Errorf("%s: %s", command, strings.TrimSpace(line[strings.Index(line, s):]))
					}
				}
			}
		}
	}
	if err := step("pair "+addr, []string{"Pairing successful", "AlreadyExists"}, []string{"Failed to pair", "not available"}); err != nil {
		return err
	}
	if err := step("trust "+addr, []string{"trust succeeded"}, []string{"Failed"}); err != nil {
		return err
	}
	if err := step("connect "+addr, []string{"Connection successful"}, []string{"Failed to connect"}); err != nil {
		return err
	}
	io.WriteString(stdin, "quit\n")
	return nil
}

func (l *Linux) simple(ctx context.Context, verb, addr, okText string) error {
	out, err := l.run(ctx, "bluetoothctl", verb, addr)
	text := clean(out)
	if err != nil || !strings.Contains(text, okText) {
		return fmt.Errorf("%s %s: %s", verb, addr, lastLine(text, err))
	}
	return nil
}

func (l *Linux) Connect(ctx context.Context, addr string) error {
	return l.simple(ctx, "connect", addr, "Connection successful")
}

func (l *Linux) Disconnect(ctx context.Context, addr string) error {
	return l.simple(ctx, "disconnect", addr, "Successful disconnected")
}

func (l *Linux) Forget(ctx context.Context, addr string) error {
	return l.simple(ctx, "remove", addr, "Device has been removed")
}

func lastLine(text string, err error) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if s := strings.TrimSpace(lines[len(lines)-1]); s != "" {
		return s
	}
	if err != nil {
		return err.Error()
	}
	return "falhou"
}
