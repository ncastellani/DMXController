package control

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/library"

	"gopkg.in/yaml.v3"
)

type fill byte

func (v fill) Render(_ time.Duration, f *dmx.Frame) { f[0] = byte(v) }

func btn(caption string, l engine.Layer, key byte, v byte) *Button {
	return &Button{Caption: caption, Group: "G", Layer: l, Key: key, Name: caption, Src: fill(v)}
}

func newController(t *testing.T) (*Controller, *engine.Engine, *[]string) {
	c := &Console{}
	c.Limits.Flash, c.Limits.Strobe = 5*time.Second, 30*time.Second
	c.Buttons = []*Button{
		btn("POP", engine.Base, 0, 10),
		btn("AZUL", engine.Base, 'p', 11),
		btn("STROBE", engine.Strobe, 'z', 20),
		btn("FRITA", engine.Flash, 'a', 30),
		btn("FRITA2", engine.Flash, 's', 31),
	}
	for i, b := range c.Buttons {
		b.ID = i
	}
	eng := &engine.Engine{}
	ctl := NewController(c, eng, testRig(t))
	var log []string
	ctl.OnEvent(func(s string) { log = append(log, s) })
	return ctl, eng, &log
}

// testRig tem 3 canhões de 7 canais nos endereços lógicos 1, 8 e 15.
func testRig(t *testing.T) *fixture.Rig {
	t.Helper()
	p := &fixture.Profile{
		Channels: []fixture.Role{fixture.Dimmer, fixture.Red, fixture.Green, fixture.Blue, fixture.Empty, fixture.Mode, fixture.Speed},
	}
	var fs []*fixture.Fixture
	for i, name := range []string{"esquerda", "centro", "direita"} {
		fs = append(fs, &fixture.Fixture{Name: name, Address: 1 + 7*i, ProfileID: "mm", Profile: p})
	}
	rig, err := fixture.NewRig(fs, []int{1, 8, 15}, filepath.Join(t.TempDir(), "patch.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return rig
}

func out(eng *engine.Engine, now time.Time) byte {
	var f dmx.Frame
	eng.Render(now, &f)
	return f[0]
}

func TestLoadShowConsole(t *testing.T) {
	c, err := LoadConsole("../../config/console.yaml", Positions(testRig(t)))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[engine.Layer]int{}
	for _, b := range c.Buttons {
		counts[b.Layer]++
	}
	if counts[engine.Base] != 44 || counts[engine.Strobe] != 4 || counts[engine.Flash] != 7 {
		t.Fatalf("botões por camada: %v", counts)
	}
}

func TestBuildErrors(t *testing.T) {
	for _, tt := range []struct{ yaml, want string }{
		{`groups: [{name: X, layer: base, buttons: [{caption: A, preset: "todos:nada"}]}]`, "não existe"},
		{`groups: [{name: X, layer: foo, buttons: []}]`, "layer"},
		{`groups: [{name: X, layer: base, buttons: [{caption: A, preset: "todos:s", key: b}]}]`, "reservada"},
		{`groups: [{name: X, layer: base, buttons: [{caption: A, preset: "todos:s", key: ab}]}]`, "inválida"},
		{`groups: [{name: X, layer: base, buttons: [{caption: A, preset: "todos:s", key: a}, {caption: B, preset: "todos:s", key: a}]}]`, "já usada"},
	} {
		_, err := buildFromYAML(t, tt.yaml)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: erro = %v, quero %q", tt.yaml, err, tt.want)
		}
	}
}

func buildFromYAML(t *testing.T, groups string) (*Console, error) {
	t.Helper()
	var cf consoleFile
	src := "limits: {flash: 1s, strobe: 1s}\n" + groups
	if err := yaml.Unmarshal([]byte(src), &cf); err != nil {
		t.Fatal(err)
	}
	lib := &library.Library{Colors: []library.Color{{ID: "todos:s", Name: "S", Hex: "#ff0000", Alias: "qlc:0"}}}
	if err := lib.Check(); err != nil {
		t.Fatal(err)
	}
	return build(&cf, lib, nil)
}

func frame(eng *engine.Engine, now time.Time) dmx.Frame {
	var f dmx.Frame
	eng.Render(now, &f)
	return f
}

func TestFlashSeveralHolders(t *testing.T) {
	ctl, eng, _ := newController(t)
	c := ctl.Console()
	a, s := c.Buttons[3], c.Buttons[4]
	now := time.Now()
	ctl.FlashDown("ipad", a, 0, now)
	ctl.FlashDown("pc", s, 0, now)
	if out(eng, now) != 31 {
		t.Fatal("o flash mais recente deveria vencer")
	}
	ctl.FlashUp("pc", s, now)
	if out(eng, now) != 30 {
		t.Fatal("soltar o mais recente deveria voltar ao outro que continua segurado")
	}
	ctl.FlashUp("ipad", nil, now)
	if out(eng, now) != 0 {
		t.Fatal("soltar tudo do holder deveria apagar")
	}
}

func TestFlashTTL(t *testing.T) {
	ctl, eng, _ := newController(t)
	a := ctl.Console().Buttons[3]
	t0 := time.Now()
	ctl.FlashDown("web1", a, time.Second, t0)
	ctl.Refresh("web1", time.Second, t0.Add(800*time.Millisecond))
	ctl.Tick(t0.Add(1500 * time.Millisecond))
	if out(eng, t0) != 30 {
		t.Fatal("ping deveria manter o flash")
	}
	ctl.Tick(t0.Add(1900 * time.Millisecond))
	if out(eng, t0) != 0 {
		t.Fatal("sem ping, o flash deveria expirar (conexão caiu)")
	}
}

func TestManual(t *testing.T) {
	ctl, eng, _ := newController(t)
	now := time.Now()
	ctl.Press(ctl.Console().Buttons[0], now) // escreve 10 no canal 1
	if err := ctl.SetManual(1, 2, 99); err != nil {
		t.Fatal(err)
	}
	if err := ctl.SetManual(0, 0, 5); err != nil {
		t.Fatal(err)
	}
	f := frame(eng, now)
	if f[0] != 5 || f[9] != 99 {
		t.Fatalf("manual: canal 1=%d (quero 5), canal 10=%d (quero 99)", f[0], f[9])
	}
	if st := ctl.State(); !st.Positions[1].Manual[2] || st.Positions[1].Manual[1] {
		t.Fatalf("máscara manual errada: %v", st.Positions[1].Manual)
	}
	ctl.ClearManual(0)
	if f := frame(eng, now); f[0] != 10 || f[9] != 99 {
		t.Fatalf("soltar posição 0: canal 1=%d canal 10=%d", f[0], f[9])
	}
	ctl.ClearManual(-1)
	if f := frame(eng, now); f[9] != 0 {
		t.Fatal("soltar tudo deveria liberar o canal 10")
	}
	if ctl.SetManual(3, 0, 1) == nil || ctl.SetManual(0, 7, 1) == nil {
		t.Fatal("posição/canal inválidos deveriam dar erro")
	}
}

func TestSwapSavesPatch(t *testing.T) {
	ctl, _, log := newController(t)
	if err := ctl.Swap(0, 1); err != nil {
		t.Fatal(err)
	}
	st := ctl.State()
	if st.Positions[0].Address != 8 || st.Positions[1].Address != 1 || st.Positions[0].Name != "esquerda" {
		t.Fatalf("posições após troca: %+v", st.Positions)
	}
	data, err := os.ReadFile(ctl.rig.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: esquerda\n      profile: mm\n      address: 8") {
		t.Fatalf("patch gravado:\n%s", data)
	}
	if !strings.Contains(strings.Join(*log, "\n"), "esquerda = d008") {
		t.Fatalf("eventos: %v", *log)
	}
	if ctl.Swap(2, 3) == nil {
		t.Fatal("troca com posição inexistente deveria dar erro")
	}
}

func TestIdentify(t *testing.T) {
	ctl, eng, _ := newController(t)
	t0 := time.Now()
	ctl.Press(ctl.Console().Buttons[0], t0)
	ctl.SetManual(0, 0, 77)
	if err := ctl.Identify(1, t0); err != nil {
		t.Fatal(err)
	}
	f := frame(eng, t0)
	if f[0] != 0 || f[7] != 255 || f[8] != 255 || f[14] != 0 {
		t.Fatalf("identificar centro: %v", f[:21])
	}
	ctl.Tick(t0.Add(IdentifyTime + time.Millisecond))
	if f := frame(eng, t0); f[0] != 77 || f[7] != 0 {
		t.Fatalf("depois de identificar deveria voltar: %v", f[:21])
	}
}

func TestBaseToggle(t *testing.T) {
	ctl, eng, _ := newController(t)
	now := time.Now()
	pop, azul := ctl.Console().Buttons[0], ctl.Console().Buttons[1]
	ctl.Press(pop, now)
	if out(eng, now) != 10 {
		t.Fatal("POP deveria tocar")
	}
	ctl.Press(azul, now)
	if out(eng, now) != 11 {
		t.Fatal("cor fixa deveria substituir o modo de pista")
	}
	ctl.Press(azul, now)
	if out(eng, now) != 0 {
		t.Fatal("apertar de novo deveria apagar")
	}
}

func TestFlashCap(t *testing.T) {
	ctl, eng, log := newController(t)
	a := ctl.Console().Buttons[3]
	t0 := time.Now()
	ctl.FlashDown("web1", a, 0, t0)
	ctl.Tick(t0.Add(4 * time.Second))
	if out(eng, t0) != 30 {
		t.Fatal("flash deveria continuar antes do teto")
	}
	ctl.Tick(t0.Add(5 * time.Second))
	if out(eng, t0) != 0 {
		t.Fatal("flash deveria desligar no teto mesmo segurado")
	}
	if !strings.Contains(strings.Join(*log, "\n"), "teto") {
		t.Fatalf("deveria avisar do teto: %v", *log)
	}
	// Continua segurado: não volta. Soltou e apertou de novo: volta.
	ctl.FlashDown("web1", a, 0, t0.Add(6*time.Second))
	if out(eng, t0) != 0 {
		t.Fatal("repetir o down sem soltar não deveria religar")
	}
	ctl.FlashUp("web1", a, t0.Add(7*time.Second))
	ctl.FlashDown("web1", a, 0, t0.Add(8*time.Second))
	if out(eng, t0) != 30 {
		t.Fatal("flash deveria voltar depois de soltar")
	}
}

func TestStrobeCapAndStop(t *testing.T) {
	ctl, eng, _ := newController(t)
	b := ctl.Console().Buttons
	t0 := time.Now()
	ctl.Press(b[0], t0)
	ctl.Press(b[2], t0)
	if out(eng, t0) != 20 {
		t.Fatal("strobe deveria sobrepor a base")
	}
	ctl.StopStrobe()
	if out(eng, t0) != 10 {
		t.Fatal("parar strobe deveria voltar à base")
	}
	ctl.Press(b[2], t0)
	ctl.Tick(t0.Add(31 * time.Second))
	if out(eng, t0) != 10 {
		t.Fatal("strobe deveria desligar no teto")
	}
}

func TestBlackoutAndStopAll(t *testing.T) {
	ctl, eng, _ := newController(t)
	now := time.Now()
	ctl.Press(ctl.Console().Buttons[0], now)
	ctl.ToggleBlackout()
	if out(eng, now) != 0 {
		t.Fatal("blackout deveria apagar")
	}
	ctl.ToggleBlackout()
	if out(eng, now) != 10 {
		t.Fatal("desligar o blackout deveria voltar")
	}
	ctl.FlashDown("web1", ctl.Console().Buttons[3], 0, now)
	ctl.SetManual(0, 0, 5)
	ctl.StopAll()
	if f := frame(eng, now); f[0] != 0 {
		t.Fatalf("parar tudo deveria apagar tudo, inclusive manual: %d", f[0])
	}
	if st := ctl.State(); st.Active[engine.Base] != nil || st.Active[engine.Flash] != nil {
		t.Fatalf("parar tudo deveria limpar o estado: %+v", st.Active)
	}
}
