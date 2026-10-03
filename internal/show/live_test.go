package show

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
)

func TestLive(t *testing.T) {
	dir := t.TempDir()
	tl := timeline(
		Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:vermelho"},
		Cue{T: 10, Layer: "base", Kind: "preset", Preset: "todos:azul"},
		Cue{T: 12, Layer: "flash", Kind: "preset", Preset: "fritacao:branco", Duration: 2},
	)
	writeTimeline(t, dir, "teste", tl)
	eng := &engine.Engine{}
	sh := New(dir, NewPresets(workspace(t), rig(t)), eng, nil, 10*time.Second, func(string, ...any) {})
	if err := sh.Scan(); err != nil {
		t.Fatal(err)
	}
	render := func() dmx.Frame {
		var f dmx.Frame
		eng.Render(time.Now(), &f)
		return f
	}

	if err := sh.Live("web1", "x.json", 0, false, false); err == nil {
		t.Fatal("música fora da setlist deveria ser recusada")
	}
	if err := sh.Live("web1", "teste.json", 0, false, true); err != nil || sh.LiveStatus().File != "" {
		t.Fatal("aviso de posição sem prévia ligada deveria ser ignorado")
	}

	// Pausada em 5 s: o quadro daquele instante (vermelho).
	if err := sh.Live("web1", "teste.json", 5, false, false); err != nil {
		t.Fatal(err)
	}
	if st := sh.LiveStatus(); st.File != "teste.json" || st.Owner != "web1" {
		t.Fatalf("status: %+v", st)
	}
	if f := render(); f[1] != 255 || f[3] != 0 {
		t.Fatalf("pausada em 5 s deveria mostrar o vermelho: %v", f[:7])
	}
	// Pausada na fritação: cor parada, sem strobo.
	sh.Live("web1", "teste.json", 12.5, false, true)
	if f := render(); f[5] != 0 || f[6] != 0 || f[1] == 0 {
		t.Fatalf("pausada na fritação deveria ficar sem strobo: %v", f[:7])
	}
	// Tocando, a fritação pisca de verdade.
	sh.Live("web1", "teste.json", 12.5, true, true)
	if f := render(); f[5] == 0 {
		t.Fatalf("tocando na fritação deveria ligar o strobo: %v", f[:7])
	}
	// Tocando, a posição anda sozinha entre os avisos.
	sh.Live("web1", "teste.json", 9.95, true, true)
	time.Sleep(80 * time.Millisecond)
	if f := render(); f[3] != 255 {
		t.Fatalf("passou de 10 s: deveria estar azul: %v", f[:7])
	}

	// Outro aparelho: aviso periódico é ignorado; ligar assume.
	sh.Live("web2", "teste.json", 1, false, true)
	if sh.LiveStatus().Owner != "web1" {
		t.Fatal("aviso de outro aparelho não deveria assumir")
	}
	sh.Live("web2", "teste.json", 1, false, false)
	if sh.LiveStatus().Owner != "web2" {
		t.Fatal("ligar de outro aparelho deveria assumir")
	}

	// Edição salva vale na hora.
	tl2 := timeline(Cue{T: 0, Layer: "base", Kind: "preset", Preset: "todos:verde"})
	if err := sh.Update(sh.live.path, tl2); err != nil {
		t.Fatal(err)
	}
	if f := render(); f[2] != 255 || f[1] != 0 {
		t.Fatalf("depois da edição deveria estar verde: %v", f[:7])
	}

	// Desconectar o dono tira a prévia (e o engine apaga).
	sh.ExpireLive("web1", time.Now())
	if sh.LiveStatus().File == "" {
		t.Fatal("desconectar outro aparelho não deveria tirar a prévia")
	}
	sh.ExpireLive("web2", time.Now())
	if sh.LiveStatus().File != "" || render() != (dmx.Frame{}) {
		t.Fatal("desconectar o dono deveria tirar a prévia dos canhões")
	}

	// Sem aviso por LiveTTL, sai sozinha.
	sh.Live("web1", "teste.json", 5, false, false)
	sh.ExpireLive("", time.Now().Add(LiveTTL+time.Second))
	if sh.LiveStatus().File != "" {
		t.Fatal("sem aviso por LiveTTL a prévia deveria sair")
	}

	// O show tocando: recusa ligar; o show começar tira a prévia.
	sh.Live("web1", "teste.json", 5, false, false)
	if err := sh.Load(0); err != nil {
		t.Fatal(err)
	}
	if err := sh.Play(); err != nil {
		t.Fatal(err)
	}
	if sh.LiveStatus().File != "" {
		t.Fatal("o show começar deveria tirar a prévia dos canhões")
	}
	if err := sh.Live("web1", "teste.json", 5, false, false); err == nil {
		t.Fatal("com o show tocando, a prévia nos canhões deveria ser recusada")
	}
	sh.Pause()
	if err := sh.Live("web1", "teste.json", 5, false, false); err != nil {
		t.Fatalf("com o show pausado, a prévia deveria ligar: %v", err)
	}
	sh.StopLive("")
	if eng.Status().Names[engine.Live] != "" {
		t.Fatal("StopLive deveria liberar a camada Live")
	}
}

// writeTimeline grava a timeline tl como name.json (com um WAV vazio).
func writeTimeline(t *testing.T, dir, name string, tl *Timeline) {
	t.Helper()
	tl.Song.Title, tl.Song.File = name, name+".wav"
	data, _ := json.Marshal(tl)
	os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)
	os.WriteFile(filepath.Join(dir, name+".wav"), nil, 0o644)
}
