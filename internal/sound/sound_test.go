package sound

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParsePWDump(t *testing.T) {
	data, err := os.ReadFile("testdata/pw-dump.json")
	if err != nil {
		t.Fatal(err)
	}
	outs, ids, err := parsePWDump(data)
	if err != nil {
		t.Fatal(err)
	}
	var bt *Output
	for i := range outs {
		if outs[i].Kind == "bluetooth" {
			bt = &outs[i]
		}
	}
	if bt == nil || bt.Name != "MONDIAL CM" || !bt.Default || ids[bt.ID] == 0 {
		t.Fatalf("saídas: %+v", outs)
	}
	for _, o := range outs {
		if strings.Contains(o.ID, "hdmi") && o.Kind != "hdmi" {
			t.Errorf("HDMI classificado como %q", o.Kind)
		}
	}
}

func TestParseBTInfo(t *testing.T) {
	data, err := os.ReadFile("testdata/bt-info.txt")
	if err != nil {
		t.Fatal(err)
	}
	d, audio := parseBTInfo("12:34:56:78:9A:BC", clean(data))
	if !audio || d.Name != "MONDIAL CM" || !d.Paired {
		t.Fatalf("info: %+v audio=%v", d, audio)
	}
	if _, audio := parseBTInfo("X", "\tName: Mouse\n\tIcon: input-mouse\n"); audio {
		t.Fatal("mouse não é aparelho de som")
	}
}

func TestParseMac(t *testing.T) {
	sw := []byte(`{"name": "Alto-falantes do MacBook Pro", "type": "output", "id": "73", "uid": "BuiltInSpeakerDevice"}
{"name": "Fones de ouvido externos", "type": "output", "id": "80", "uid": "BuiltInHeadphoneOutputDevice"}
{"name": "MONDIAL CM", "type": "output", "id": "91", "uid": "12-34-56-78-9A-BC:output"}
`)
	devs := parseSwitchJSON(sw)
	if len(devs) != 3 || devs[2].UID != "12-34-56-78-9A-BC:output" {
		t.Fatalf("SwitchAudioSource: %+v", devs)
	}
	if kindOf(devs[0].Name) != "builtin" || kindOf(devs[1].Name) != "wired" {
		t.Fatalf("tipos: %q %q", kindOf(devs[0].Name), kindOf(devs[1].Name))
	}
	bu := []byte(`[{"address":"12-34-56-78-9a-bc","name":"MONDIAL CM","connected":true,"paired":true},{"address":"aa-bb-cc-dd-ee-ff","name":"","connected":false,"paired":false}]`)
	bt, err := parseBlueutil(bu)
	if err != nil || len(bt) != 2 || !bt[0].Connected || bt[1].Name != "aa-bb-cc-dd-ee-ff" {
		t.Fatalf("blueutil: %+v %v", bt, err)
	}
}

// fake simula o sistema: uma caixa que pode cair.
type fake struct {
	mu       sync.Mutex
	up       bool
	connects int
	output   string
	failPair bool
}

func (f *fake) Missing() []string { return nil }
func (f *fake) Outputs(context.Context) ([]Output, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []Output{{ID: "p2", Name: "Fone", Kind: "wired", Default: f.output == "p2"}, {ID: "bt", Name: "CAIXA", Kind: "bluetooth", Default: f.output == "bt"}}, nil
}
func (f *fake) SetOutput(_ context.Context, id string) error {
	f.mu.Lock()
	f.output = id
	f.mu.Unlock()
	return nil
}
func (f *fake) Devices(context.Context) ([]Device, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []Device{{Address: "AA", Name: "CAIXA", Paired: true, Connected: f.up}}, nil
}
func (f *fake) Scan(ctx context.Context, _ time.Duration) ([]Device, error) { return f.Devices(ctx) }
func (f *fake) Pair(ctx context.Context, a string) error {
	if f.failPair {
		return errors.New("Failed to pair")
	}
	return f.Connect(ctx, a)
}
func (f *fake) Connect(context.Context, string) error {
	f.mu.Lock()
	f.up = true
	f.connects++
	f.mu.Unlock()
	return nil
}
func (f *fake) Disconnect(context.Context, string) error {
	f.mu.Lock()
	f.up = false
	f.mu.Unlock()
	return nil
}
func (f *fake) Forget(ctx context.Context, a string) error { return f.Disconnect(ctx, a) }

type fakePlayer struct {
	lat time.Duration
	vol float64
}

func (p *fakePlayer) SetLatency(d time.Duration) { p.lat = d }
func (p *fakePlayer) SetVolume(v float64)        { p.vol = v }

func waitIdle(t *testing.T, m *Manager) {
	t.Helper()
	for i := 0; i < 200 && m.State().Busy != ""; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if m.State().Busy != "" {
		t.Fatal("ação não terminou")
	}
}

func TestManagerSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "som.yaml")
	os.WriteFile(path, []byte("latency_ms: 180\nvolume: 0.7\n"), 0o644)
	p := &fakePlayer{}
	m, err := NewManager(&fake{}, path, p, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if p.lat != 180*time.Millisecond || p.vol != 0.7 {
		t.Fatalf("preferências não aplicadas: %+v", p)
	}
	m.SetLatency(5000) // limitado a 1000
	m.save()
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "latency_ms: 1000") {
		t.Fatalf("som.yaml:\n%s", data)
	}
	m.SetLatency(-120) // som adiantado: a luz é que adianta
	m.save()
	data, _ = os.ReadFile(path)
	if p.lat != -120*time.Millisecond || !strings.Contains(string(data), "latency_ms: -120") {
		t.Fatalf("atraso negativo: player %v, som.yaml:\n%s", p.lat, data)
	}
}

func TestManagerReconnectsSpeaker(t *testing.T) {
	f := &fake{}
	var log []string
	var mu sync.Mutex
	m, _ := NewManager(f, filepath.Join(t.TempDir(), "som.yaml"), nil, func(s string, a ...any) {
		mu.Lock()
		log = append(log, s)
		mu.Unlock()
	})
	ctx := context.Background()
	if err := m.Do(ctx, "pair", "AA"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, m)
	if m.Settings().Speaker != "AA" || !f.up {
		t.Fatal("parear deveria conectar e tornar a caixa preferida")
	}
	m.Do(ctx, "output", "bt")
	waitIdle(t, m)
	if m.Settings().Speaker != "AA" {
		t.Fatal("escolher a saída Bluetooth deveria manter a caixa como preferida")
	}

	// A caixa cai sozinha: aviso + reconexão (e a saída volta para ela).
	f.Disconnect(ctx, "AA")
	f.SetOutput(ctx, "p2")
	m.refresh(ctx)
	if !strings.Contains(m.State().Warning, "desconectada") {
		t.Fatalf("deveria avisar: %+v", m.State())
	}
	m.keepSpeaker(ctx)
	if !f.up || f.output != "bt" || m.State().Warning != "" {
		t.Fatalf("deveria reconectar e voltar a saída: up=%v saída=%s %+v", f.up, f.output, m.State())
	}

	// Desconectar de propósito: não reconecta.
	m.Do(ctx, "disconnect", "AA")
	waitIdle(t, m)
	m.lastTry = time.Time{}
	m.keepSpeaker(ctx)
	if f.up || m.Settings().Speaker != "" {
		t.Fatal("desconexão pedida não deveria reconectar")
	}
}

func TestManagerBusyAndErrors(t *testing.T) {
	f := &fake{failPair: true}
	m, _ := NewManager(f, filepath.Join(t.TempDir(), "som.yaml"), nil, func(string, ...any) {})
	if err := m.Do(context.Background(), "voar", ""); err == nil {
		t.Fatal("ação desconhecida deveria dar erro")
	}
	m.Do(context.Background(), "pair", "AA")
	waitIdle(t, m)
	if !strings.Contains(m.State().Error, "Failed to pair") || m.Settings().Speaker != "" {
		t.Fatalf("erro de pareamento: %+v", m.State())
	}
}
