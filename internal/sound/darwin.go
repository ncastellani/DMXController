package sound

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Darwin usa SwitchAudioSource (saídas) e blueutil (Bluetooth), do Homebrew.
type Darwin struct {
	run      runner
	switcher string
	blueutil string
}

// NewDarwin cria o backend do macOS.
func NewDarwin() *Darwin {
	sw, _ := lookPath("SwitchAudioSource")
	bu, _ := lookPath("blueutil")
	return &Darwin{run: execRun, switcher: sw, blueutil: bu}
}

func (d *Darwin) Missing() []string {
	var miss []string
	if _, ok := lookPath("SwitchAudioSource"); !ok {
		miss = append(miss, "switchaudio-osx")
	}
	if _, ok := lookPath("blueutil"); !ok {
		miss = append(miss, "blueutil")
	}
	return miss
}

type switchDevice struct {
	Name string `json:"name"`
	Type string `json:"type"`
	ID   any    `json:"id"`
	UID  string `json:"uid"`
}

// parseSwitchJSON lê a saída de "SwitchAudioSource -f json" (um objeto JSON
// por linha).
func parseSwitchJSON(data []byte) []switchDevice {
	var devs []switchDevice
	for _, line := range strings.Split(string(data), "\n") {
		var sd switchDevice
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &sd) == nil && sd.Name != "" {
			devs = append(devs, sd)
		}
	}
	return devs
}

func (d *Darwin) Outputs(ctx context.Context) ([]Output, error) {
	all, err := d.run(ctx, d.switcher, "-a", "-t", "output", "-f", "json")
	if err != nil {
		return nil, fmt.Errorf("SwitchAudioSource: %v %s", err, all)
	}
	cur, _ := d.run(ctx, d.switcher, "-c", "-t", "output", "-f", "json")
	current := parseSwitchJSON(cur)
	// Saídas com nome de aparelho Bluetooth pareado são Bluetooth.
	bt := map[string]bool{}
	if devs, err := d.Devices(ctx); err == nil {
		for _, dv := range devs {
			bt[dv.Name] = true
		}
	}
	var outs []Output
	for _, sd := range parseSwitchJSON(all) {
		o := Output{ID: sd.UID, Name: sd.Name, Kind: kindOf(sd.Name)}
		if o.ID == "" {
			o.ID = sd.Name
		}
		if bt[sd.Name] {
			o.Kind = "bluetooth"
		}
		if len(current) > 0 && (current[0].UID == sd.UID && sd.UID != "" || current[0].Name == sd.Name) {
			o.Default = true
		}
		outs = append(outs, o)
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].Name < outs[j].Name })
	return outs, nil
}

func (d *Darwin) SetOutput(ctx context.Context, id string) error {
	args := []string{"-t", "output", "-u", id}
	if outs, err := d.Outputs(ctx); err == nil {
		for _, o := range outs {
			if o.ID == id && o.ID == o.Name { // sem UID: seleciona por nome
				args = []string{"-t", "output", "-s", id}
			}
		}
	}
	if out, err := d.run(ctx, d.switcher, args...); err != nil {
		return fmt.Errorf("SwitchAudioSource: %v %s", err, out)
	}
	return nil
}

type blueutilDevice struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Paired    bool   `json:"paired"`
	Connected bool   `json:"connected"`
}

func parseBlueutil(data []byte) ([]Device, error) {
	var bds []blueutilDevice
	if err := json.Unmarshal(data, &bds); err != nil {
		return nil, fmt.Errorf("blueutil: %v", err)
	}
	devs := make([]Device, 0, len(bds))
	for _, b := range bds {
		name := b.Name
		if name == "" {
			name = b.Address
		}
		devs = append(devs, Device{Address: b.Address, Name: name, Paired: b.Paired, Connected: b.Connected})
	}
	sort.Slice(devs, func(i, j int) bool { return devs[i].Name < devs[j].Name })
	return devs, nil
}

func (d *Darwin) Devices(ctx context.Context) ([]Device, error) {
	out, err := d.run(ctx, d.blueutil, "--paired", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("blueutil: %v %s", err, out)
	}
	return parseBlueutil(out)
}

func (d *Darwin) Scan(ctx context.Context, dur time.Duration) ([]Device, error) {
	out, err := d.run(ctx, d.blueutil, "--inquiry", fmt.Sprint(int(dur.Seconds())), "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("blueutil: %v %s", err, out)
	}
	found, err := parseBlueutil(out)
	if err != nil {
		return nil, err
	}
	paired, _ := d.Devices(ctx)
	seen := map[string]bool{}
	for _, p := range paired {
		seen[p.Address] = true
	}
	for _, f := range found {
		if !seen[f.Address] {
			paired = append(paired, f)
		}
	}
	return paired, nil
}

func (d *Darwin) blue(ctx context.Context, args ...string) error {
	if out, err := d.run(ctx, d.blueutil, args...); err != nil {
		return fmt.Errorf("blueutil %s: %v %s", strings.Join(args, " "), err, out)
	}
	return nil
}

func (d *Darwin) Pair(ctx context.Context, addr string) error {
	if err := d.blue(ctx, "--pair", addr); err != nil {
		return err
	}
	return d.blue(ctx, "--connect", addr)
}

func (d *Darwin) Connect(ctx context.Context, addr string) error {
	return d.blue(ctx, "--connect", addr)
}

func (d *Darwin) Disconnect(ctx context.Context, addr string) error {
	return d.blue(ctx, "--disconnect", addr)
}

func (d *Darwin) Forget(ctx context.Context, addr string) error {
	return d.blue(ctx, "--unpair", addr)
}
