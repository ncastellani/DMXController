package qlc

import (
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
)

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<Workspace xmlns="http://www.qlcplus.org/Workspace">
 <Engine>
  <Fixture><ID>0</ID><Universe>0</Universe><Address>0</Address><Channels>7</Channels></Fixture>
  <Fixture><ID>1</ID><Universe>0</Universe><Address>7</Address><Channels>7</Channels></Fixture>
  <Function ID="0" Type="Scene" Name="VERMELHO">
   <Speed FadeIn="0" FadeOut="0" Duration="0"/>
   <FixtureVal ID="0">0,255,1,255</FixtureVal>
   <FixtureVal ID="1">0,200,1,255</FixtureVal>
  </Function>
  <Function ID="1" Type="Sequence" Name="COMUM" BoundScene="0">
   <Speed FadeIn="0" FadeOut="0" Duration="100"/>
   <Direction>Forward</Direction>
   <RunOrder>Loop</RunOrder>
   <SpeedModes FadeIn="Default" FadeOut="Default" Duration="Common"/>
   <Step Number="0" FadeIn="0" Hold="999" FadeOut="0" Values="1">0:1,10</Step>
   <Step Number="1" FadeIn="0" Hold="999" FadeOut="0" Values="1">0:1,20:1:3,30</Step>
  </Function>
  <Function ID="2" Type="Sequence" Name="FADE" BoundScene="0">
   <Speed FadeIn="0" FadeOut="0" Duration="1000"/>
   <Direction>Forward</Direction>
   <RunOrder>Loop</RunOrder>
   <SpeedModes FadeIn="PerStep" FadeOut="PerStep" Duration="PerStep"/>
   <Step Number="0" FadeIn="0" Hold="300" FadeOut="0" Values="1">0:1,0</Step>
   <Step Number="1" FadeIn="200" Hold="500" FadeOut="0" Values="1">0:1,200</Step>
  </Function>
  <Function ID="3" Type="Sequence" Name="PINGPONG" BoundScene="0">
   <Direction>Forward</Direction>
   <RunOrder>PingPong</RunOrder>
  </Function>
  <Function ID="4" Type="EFX" Name="UM EFX"/>
 </Engine>
</Workspace>`

func render(src interface {
	Render(time.Duration, *dmx.Frame)
}, ms int) dmx.Frame {
	var f dmx.Frame
	src.Render(time.Duration(ms)*time.Millisecond, &f)
	return f
}

func TestScene(t *testing.T) {
	w, err := parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	f := render(w.Scenes[0], 0)
	// Fixture 1 começa no canal 8 (Address 7, base 0).
	if f[0] != 255 || f[1] != 255 || f[7] != 200 || f[8] != 255 {
		t.Fatalf("canais 1,2,8,9 = %d,%d,%d,%d", f[0], f[1], f[7], f[8])
	}
}

func TestSequenceCommon(t *testing.T) {
	w, err := parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	seq := w.Sequences[1]
	// Duration="Common": o Hold dos passos é ignorado.
	if seq.Cycle != 200*time.Millisecond {
		t.Fatalf("ciclo = %v, quero 200ms", seq.Cycle)
	}
	for _, tt := range []struct{ ms, ch2, ch11 int }{
		{0, 10, 0}, {99, 10, 0}, {100, 20, 30}, {199, 20, 30}, {200, 10, 0}, {350, 20, 30},
	} {
		f := render(seq, tt.ms)
		if int(f[1]) != tt.ch2 || int(f[10]) != tt.ch11 {
			t.Errorf("t=%dms: canal 2=%d 11=%d, quero %d %d", tt.ms, f[1], f[10], tt.ch2, tt.ch11)
		}
	}
}

func TestSequenceFade(t *testing.T) {
	w, err := parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	seq := w.Sequences[2]
	if seq.Cycle != time.Second {
		t.Fatalf("ciclo = %v, quero 1s", seq.Cycle)
	}
	for _, tt := range []struct{ ms, want int }{
		{0, 0}, {299, 0}, {300, 0}, {350, 50}, {400, 100}, {500, 200}, {999, 200},
	} {
		if f := render(seq, tt.ms); int(f[1]) != tt.want {
			t.Errorf("t=%dms: canal 2=%d, quero %d", tt.ms, f[1], tt.want)
		}
	}
}

func TestSkipped(t *testing.T) {
	w, err := parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Skipped) != 2 {
		t.Fatalf("skipped = %v, quero PINGPONG e EFX", w.Skipped)
	}
	if _, _, ok := w.Function(3); ok {
		t.Fatal("sequence PingPong não deveria carregar")
	}
}

func TestInvalidChannel(t *testing.T) {
	bad := `<Workspace><Engine>
  <Fixture><ID>0</ID><Universe>0</Universe><Address>0</Address><Channels>7</Channels></Fixture>
  <Function ID="0" Type="Scene" Name="X"><FixtureVal ID="0">7,255</FixtureVal></Function>
 </Engine></Workspace>`
	if _, err := parse([]byte(bad)); err == nil {
		t.Fatal("canal 7 de um fixture de 7 canais deveria dar erro")
	}
}

// TestShowWorkspace confere o workspace real do show contra os ciclos
// exportados pelo QLC+ (FUNCTIONS.csv do pacote original do Amós).
func TestShowWorkspace(t *testing.T) {
	w, err := Load("../../config/aniversario_STROBE.qxw")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Scenes) != 73 || len(w.Sequences) != 32 || len(w.Skipped) != 0 {
		t.Fatalf("scenes=%d sequences=%d skipped=%v", len(w.Scenes), len(w.Sequences), w.Skipped)
	}
	for id, want := range map[int]int{30: 4200, 47: 32700, 55: 4520, 73: 10520} {
		if got := w.Sequences[id].Cycle; got != time.Duration(want)*time.Millisecond {
			t.Errorf("sequence %d: ciclo %v, quero %dms", id, got, want)
		}
	}
	// FRITAÇÃO | BRANCO MÁXIMO: strobo nativo nos 3 canhões (CH6=50, CH7=255).
	f := render(w.Scenes[94], 0)
	for _, base := range []int{0, 7, 14} {
		if f[base] != 255 || f[base+5] != 50 || f[base+6] != 255 {
			t.Errorf("canhão no canal %d: %v", base+1, f[base:base+7])
		}
	}
}
