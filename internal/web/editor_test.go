package web

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncastellani/DMXController/internal/show"
)

// addSong grava uma música mínima (timeline + WAV vazio) na pasta.
func addSong(t *testing.T, dir, name string) {
	t.Helper()
	tl := show.Timeline{Version: 1}
	tl.Song.Title, tl.Song.File, tl.Song.Duration = name, name+".wav", 20
	tl.Analysis.Duration, tl.Analysis.BPM = 20, 120
	for b := 0.5; b < 20; b += 0.5 {
		tl.Analysis.Beats = append(tl.Analysis.Beats, b)
	}
	tl.Analysis.Sections = []show.Section{{Start: 0, End: 10, Group: "A", Name: "verso"}, {Start: 10, End: 20, Group: "B", Name: "refrão"}}
	tl.Cues = []show.Cue{{T: 0, Layer: "base", Kind: "preset", Preset: "pista:azul-rosa", Rate: 1}}
	data, _ := json.Marshal(tl)
	os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)
	os.WriteFile(filepath.Join(dir, name+".wav"), nil, 0o644)
}

func TestReorderKeepsLoadedSong(t *testing.T) {
	_, sh, dir, _ := newImportServer(t)
	for _, n := range []string{"a", "b", "c"} {
		addSong(t, dir, n)
	}
	sh.Scan()
	sh.Load(0) // "a"
	if err := sh.Reorder([]string{"c.json", "a.json", "b.json"}); err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, so := range sh.Songs() {
		titles = append(titles, so.Title)
	}
	if strings.Join(titles, ",") != "c,a,b" || sh.Status().Current != 1 {
		t.Fatalf("ordem %v, atual %d", titles, sh.Status().Current)
	}
	addSong(t, dir, "0-nova") // fora da ordem: vai para o fim, mesmo vindo antes no alfabeto
	sh.Scan()
	if songs := sh.Songs(); songs[len(songs)-1].Title != "0-nova" {
		t.Fatalf("música nova deveria ir para o fim: %+v", songs)
	}
	if sh.Reorder([]string{"x.json"}) == nil {
		t.Fatal("música desconhecida deveria dar erro")
	}
}

func TestSaveAndRevertTimeline(t *testing.T) {
	srv, sh, dir, _ := newImportServer(t)
	addSong(t, dir, "m")
	sh.Scan()
	if err := srv.saveTimeline("", json.RawMessage(`{}`)); err == nil {
		t.Fatal("sem música carregada deveria dar erro")
	}
	sh.Load(0)
	path := filepath.Join(dir, "m.json")
	orig, _ := os.ReadFile(path)

	edited := sh.Timeline()
	var e show.Timeline
	data, _ := json.Marshal(edited)
	json.Unmarshal(data, &e)
	e.Cues = []show.Cue{
		{T: 12.25, Layer: "flash", Kind: "preset", Preset: "fritacao:branco", Duration: 99}, // fora de ordem, duração enorme
		{T: 0, Layer: "base", Kind: "preset", Preset: "pista:pop", Rate: 0},                 // preset trocado: reencaixa
		{T: 15, Layer: "base", Kind: "blackout"},
	}
	e.Analysis.Sections[1].Name = "Refrão final"
	e.Analysis.BPM = 999 // não editável: ignorado
	raw, _ := json.Marshal(e)
	if err := srv.saveTimeline("", raw); err != nil {
		t.Fatal(err)
	}
	saved, err := show.LoadTimeline(path)
	if err != nil {
		t.Fatal(err)
	}
	c := saved.Cues
	if c[0].Preset != "pista:pop" || c[0].Rate <= 0 || c[0].Rate == 1 || c[0].Name != "PISTA | POP" {
		t.Fatalf("cue de base: %+v", c[0])
	}
	if c[1].Layer != "flash" || c[1].Duration != 10 || saved.Analysis.BPM != 120 || saved.Analysis.Sections[1].Name != "Refrão final" {
		t.Fatalf("salvo: %+v %+v", c, saved.Analysis.Sections)
	}
	if got := sh.Timeline().Cues[0].Preset; got != "pista:pop" {
		t.Fatalf("a luz deveria seguir a edição na hora: %s", got)
	}
	if b, _ := os.ReadFile(path + ".orig"); string(b) != string(orig) {
		t.Fatal("a versão gerada deveria ficar guardada em .orig")
	}

	// Preset inválido: recusa e não mexe no arquivo.
	bad := strings.Replace(string(raw), `"pista:pop"`, `"qlc:9999"`, 1)
	if err := srv.saveTimeline("", json.RawMessage(bad)); err == nil {
		t.Fatal("preset inexistente deveria ser recusado")
	}
	if again, _ := show.LoadTimeline(path); again.Cues[0].Preset != "pista:pop" {
		t.Fatal("arquivo não deveria mudar")
	}

	if err := srv.revertTimeline(""); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(orig) || exists(path+".orig") {
		t.Fatal("restaurar deveria voltar ao original e apagar o .orig")
	}
	if sh.Timeline().Cues[0].Preset != "pista:azul-rosa" {
		t.Fatal("a luz deveria voltar à versão gerada")
	}
}

// A aba Músicas edita qualquer música da setlist sem carregá-la no player.
func TestEditSongNotLoaded(t *testing.T) {
	srv, sh, dir, hs := newImportServer(t)
	addSong(t, dir, "a")
	addSong(t, dir, "b")
	os.WriteFile(filepath.Join(dir, "b.wav"), []byte("RIFF....WAVEdados"), 0o644)
	sh.Scan()
	sh.Load(0) // "a" carregada; "b" é editada
	c := dial(t, hs)

	get := func(url string, hdr ...string) (*http.Response, []byte) {
		t.Helper()
		req, _ := http.NewRequest("GET", hs.URL+url, nil)
		if len(hdr) == 2 {
			req.Header.Set(hdr[0], hdr[1])
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, body
	}

	resp, body := get("/api/timeline?file=b.json")
	var got songTimelineMsg
	if json.Unmarshal(body, &got); resp.StatusCode != 200 || got.Timeline == nil || got.Timeline.Song.Title != "b" || got.Edited {
		t.Fatalf("timeline: %d %s", resp.StatusCode, body)
	}
	e := *got.Timeline
	e.Cues = []show.Cue{{T: 0, Layer: "base", Kind: "preset", Preset: "cor:#00ff00"}, {T: 5, Layer: "base", Kind: "blackout"}}
	raw, _ := json.Marshal(e)
	if err := srv.saveTimeline("b.json", raw); err != nil {
		t.Fatal(err)
	}
	c.waitFor("songTimeline de b", func(m map[string]any) bool {
		return m["type"] == "songTimeline" && m["file"] == "b.json" && m["edited"] == true
	})
	if saved, _ := show.LoadTimeline(filepath.Join(dir, "b.json")); saved.Cues[0].Preset != "cor:#00ff00" || saved.Cues[0].Name != "COR | #00ff00" {
		t.Fatalf("b salva: %+v", saved.Cues)
	}
	if !exists(filepath.Join(dir, "b.json.orig")) || sh.Timeline().Song.Title != "a" || sh.Timeline().Cues[0].Preset != "pista:azul-rosa" {
		t.Fatal("editar b não deveria mexer na música carregada")
	}

	resp, body = get("/api/preview?file=b.json&fps=5")
	var pv previewMsg
	if json.Unmarshal(body, &pv); resp.StatusCode != 200 || pv.FPS != 5 || len(pv.Positions) != 3 {
		t.Fatalf("prévia: %d %s", resp.StatusCode, body[:min(200, len(body))])
	}
	p := pv.Positions[1]
	if p.Name != "centro" || len(p.Colors) != 6*100 || len(p.FX) != 100 || p.Colors[:6] != "00ff00" || p.Colors[6*50:6*51] != "000000" {
		t.Fatalf("centro: %s %q…", p.Name, p.Colors[:12])
	}

	resp, body = get("/api/audio?file=b.json", "Range", "bytes=0-3")
	if resp.StatusCode != http.StatusPartialContent || string(body) != "RIFF" || resp.Header.Get("Content-Type") != "audio/wav" {
		t.Fatalf("áudio: %d %q %s", resp.StatusCode, body, resp.Header.Get("Content-Type"))
	}
	for _, u := range []string{"/api/timeline?file=x.json", "/api/preview?file=../a.json.orig", "/api/audio?file="} {
		if resp, _ := get(u); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", u, resp.StatusCode)
		}
	}
	if srv.saveTimeline("x.json", raw) == nil {
		t.Fatal("música fora da setlist deveria dar erro")
	}

	if err := srv.revertTimeline("b.json"); err != nil {
		t.Fatal(err)
	}
	if again, _ := show.LoadTimeline(filepath.Join(dir, "b.json")); again.Cues[0].Preset != "pista:azul-rosa" || exists(filepath.Join(dir, "b.json.orig")) {
		t.Fatal("restaurar deveria voltar à versão gerada")
	}
}
