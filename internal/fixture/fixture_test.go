package fixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncastellani/DMXController/internal/dmx"
)

var mmT114 = &Profile{
	Name:     "MM-T114",
	Channels: []Role{Dimmer, Red, Green, Blue, Empty, Mode, Speed},
	Fixed:    map[Role]uint8{Mode: 0, Speed: 0},
}

func TestRender(t *testing.T) {
	var frame dmx.Frame
	f := &Fixture{Name: "a", Address: 8, Profile: mmT114}
	f.Render(&frame, State{Color: Color{255, 128, 0}, Intensity: 0.5})

	want := []byte{128, 255, 128, 0, 0, 0, 0}
	got := frame[7:14]
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("canais 8..14 = %v, quero %v", got, want)
		}
	}
	if frame[6] != 0 || frame[14] != 0 {
		t.Fatalf("escreveu fora dos canais do fixture")
	}
}

func TestRenderFixedAndNoDimmer(t *testing.T) {
	p := &Profile{Channels: []Role{Red, Green, Blue, Mode}, Fixed: map[Role]uint8{Mode: 42}}
	var frame dmx.Frame
	(&Fixture{Address: 1, Profile: p}).Render(&frame, State{Color: Color{200, 100, 50}, Intensity: 0.5})

	want := []byte{100, 50, 25, 42}
	for i := range want {
		if frame[i] != want[i] {
			t.Fatalf("canais 1..4 = %v, quero %v", frame[:4], want)
		}
	}
}

func TestRenderClampsIntensity(t *testing.T) {
	var frame dmx.Frame
	f := &Fixture{Address: 1, Profile: mmT114}
	f.Render(&frame, State{Intensity: 7})
	if frame[0] != 255 {
		t.Fatalf("dimmer = %d, quero 255", frame[0])
	}
	f.Render(&frame, State{Intensity: -1})
	if frame[0] != 0 {
		t.Fatalf("dimmer = %d, quero 0", frame[0])
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	profiles := write(t, dir, "fixtures.yaml", `
profiles:
  mm:
    channels: [dimmer, red, green, blue, empty, mode, speed]
`)
	tests := []struct {
		name, patch, wantErr string
	}{
		{"ok", "fixtures:\n- {name: a, profile: mm, address: 1}\n- {name: b, profile: mm, address: 8}\n", ""},
		{"sobreposição", "fixtures:\n- {name: a, profile: mm, address: 1}\n- {name: b, profile: mm, address: 7}\n", "sobrepõem"},
		{"fora do universo", "fixtures:\n- {name: a, profile: mm, address: 507}\n", "fora do universo"},
		{"perfil inexistente", "fixtures:\n- {name: a, profile: xx, address: 1}\n", "não existe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(profiles, write(t, dir, "patch.yaml", tt.patch))
			if tt.wantErr == "" && err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("erro = %v, quero algo com %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadUnknownRole(t *testing.T) {
	dir := t.TempDir()
	profiles := write(t, dir, "fixtures.yaml", "profiles:\n  x:\n    channels: [dimmer, purple]\n")
	_, err := Load(profiles, write(t, dir, "patch.yaml", "fixtures: []\n"))
	if err == nil || !strings.Contains(err.Error(), "purple") {
		t.Fatalf("erro = %v, quero função desconhecida", err)
	}
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
