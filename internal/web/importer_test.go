package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ncastellani/DMXController/internal/analysis"
	"github.com/ncastellani/DMXController/internal/audio"
	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/show"
)

// waitJob espera a n-ésima análise da fila terminar.
func waitJob(t *testing.T, srv *Server, n int) job {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		srv.mu.Lock()
		m := srv.jobsMsg()
		srv.mu.Unlock()
		if len(m.Items) == n && m.Items[n-1].Done {
			return m.Items[n-1]
		}
		if time.Now().After(deadline) {
			t.Fatalf("fila: %+v", m)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"Pra Ficar Comigo.wav":     "Pra Ficar Comigo.wav",
		"../../etc/passwd.wav":     "passwd.wav",
		`C:\\Users\\x\\Balada.WAV`: "Balada.wav",
		"música: ação?.wav":        "música_ ação_.wav",
		".escondido.wav":           "escondido.wav",
	} {
		if got, err := cleanName(in); err != nil || got != want {
			t.Errorf("cleanName(%q) = %q, %v; quero %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"musica.mp3", ".wav", "", "../.wav"} {
		if _, err := cleanName(bad); err == nil {
			t.Errorf("cleanName(%q) deveria falhar", bad)
		}
	}
}

// wavBytes gera um WAV de cliques (o formato que o player usa).
func wavBytes(d time.Duration) []byte {
	pcm := audio.Clicks(120, d).PCM
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + len(pcm)))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(2))
	w(uint32(audio.SampleRate))
	w(uint32(audio.SampleRate * 4))
	w(uint16(4))
	w(uint16(16))
	b.WriteString("data")
	w(uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

// newImportServer monta o servidor com envio de músicas numa pasta temporária.
func newImportServer(t *testing.T) (*Server, *show.Show, string, *httptest.Server) {
	t.Helper()
	fixtures, _ := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	rig, err := control.NewRig(fixtures, filepath.Join(t.TempDir(), "patch.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pos := control.Positions(rig)
	c, err := control.LoadConsole("../../config/console.yaml", pos)
	if err != nil {
		t.Fatal(err)
	}
	eng := &engine.Engine{}
	ctl := control.NewController(c, eng, rig)
	dir := t.TempDir()
	sh := show.New(dir, show.NewPresets(c.Library, pos), eng, nil, c.Limits.Flash, ctl.Notice)
	srv := New(ctl, eng, sh, nil, nil, nil, "")
	srv.SetImporter(&Importer{Dir: dir, Catalog: analysis.Catalog(c), Library: c.Library})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx)
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return srv, sh, dir, hs
}

func TestUpload(t *testing.T) {
	srv, sh, dir, hs := newImportServer(t)

	post := func(query string, body []byte, origin string) *http.Response {
		req, _ := http.NewRequest("POST", hs.URL+"/api/upload?"+query, bytes.NewReader(body))
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	wav := wavBytes(12 * time.Second)

	if r := post("name=x.mp3", wav, ""); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("extensão errada: %d", r.StatusCode)
	}
	if r := post("name=x.wav", []byte("ID3 não é wav"), ""); r.StatusCode != http.StatusBadRequest {
		t.Fatalf("conteúdo não-WAV: %d", r.StatusCode)
	}
	if r := post("name=x.wav", wav, "http://outro-site.com"); r.StatusCode != http.StatusForbidden {
		t.Fatalf("outra origem: %d", r.StatusCode)
	}
	if r := post("name=Nova+Faixa.wav", wav, hs.URL); r.StatusCode != http.StatusAccepted {
		t.Fatalf("envio válido: %d", r.StatusCode)
	}

	// A análise roda em segundo plano e a música entra na setlist.
	deadline := time.Now().Add(20 * time.Second)
	for {
		songs := sh.Songs()
		if len(songs) == 1 && songs[0].Title == "Nova Faixa" && songs[0].Err == "" {
			break
		}
		if time.Now().After(deadline) {
			srv.mu.Lock()
			t.Fatalf("a timeline não apareceu: %+v %+v", songs, srv.jobsMsg())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(filepath.Join(dir, "Nova Faixa.json")); err != nil {
		t.Fatal(err)
	}
	if err := sh.Load(0); err != nil {
		t.Fatalf("a timeline gerada deveria tocar: %v", err)
	}
	srv.mu.Lock()
	jm := srv.jobsMsg()
	srv.mu.Unlock()
	if len(jm.Items) != 1 || !jm.Items[0].Done || jm.Items[0].Error != "" || jm.AI {
		t.Fatalf("fila: %+v", jm)
	}

	if r := post("name=Nova+Faixa.wav", wav, ""); r.StatusCode != http.StatusConflict {
		t.Fatalf("repetido sem replace: %d", r.StatusCode)
	}
	if r := post("name=Nova+Faixa.wav&replace=1", wav, ""); r.StatusCode != http.StatusAccepted {
		t.Fatalf("repetido com replace: %d", r.StatusCode)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			t.Fatalf("temporário esquecido: %s", e.Name())
		}
	}
}

// TestAnalyzeStored: a faixa enviada sem análise fica como órfã até a
// mensagem analyze; a reanálise da música carregada vale na hora.
func TestAnalyzeStored(t *testing.T) {
	srv, sh, _, hs := newImportServer(t)
	resp, err := http.Post(hs.URL+"/api/upload?name=Balada.wav&analisar=0", "audio/wav", bytes.NewReader(wavBytes(12*time.Second)))
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("envio: %v %v", resp, err)
	}
	resp.Body.Close()
	srv.mu.Lock()
	jobs := len(srv.jobs)
	srv.mu.Unlock()
	if jobs != 0 {
		t.Fatal("analisar=0 não deveria pôr na fila")
	}
	if sl := srv.setlist(); len(sl.Orphans) != 1 || sl.Orphans[0] != "Balada.wav" {
		t.Fatalf("faixa sem timeline: %+v", sl.Orphans)
	}

	waitJobs := func(n int) job { t.Helper(); return waitJob(t, srv, n) }
	c := &client{id: "teste"}
	if err := srv.handle(c, &inMsg{Type: "analyze", Str: "Balada.wav", Temp: "fria", AI: true}, time.Now()); err != nil {
		t.Fatal(err)
	}
	j := waitJobs(1)
	if j.Error != "" || j.Temp != "fria" || j.AI { // sem chave: sem IA
		t.Fatalf("job: %+v", j)
	}
	sl := srv.setlist()
	if len(sl.Songs) != 1 || len(sl.Orphans) != 0 {
		t.Fatalf("setlist: %+v", sl)
	}
	if so := sl.Songs[0]; so.WAV != "Balada.wav" || so.Temp != "fria" || so.AI || so.Edited {
		t.Fatalf("música: %+v", so)
	}

	// Reanálise da música carregada: a timeline nova vale na hora.
	if err := sh.Load(0); err != nil {
		t.Fatal(err)
	}
	before := sh.Timeline()
	if err := srv.handle(c, &inMsg{Type: "analyze", Str: "Balada.wav", Temp: "quente"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	waitJobs(2)
	if sh.Timeline() == before || !strings.Contains(string(sh.Timeline().Generated), `"temperature":"quente"`) {
		t.Fatalf("a música carregada não recebeu a timeline nova: %s", sh.Timeline().Generated)
	}
	if err := srv.handle(c, &inMsg{Type: "analyze", Str: "naoexiste.wav"}, time.Now()); err == nil {
		t.Fatal("faixa inexistente deveria falhar")
	}
	if cfg := srv.config(); len(cfg.Library) != 17 || cfg.Library[0].Color == "" {
		t.Fatalf("biblioteca de cores: %+v", cfg.Library)
	}
}

// TestRemoveSong: a música sai da setlist (e do setlist.txt) e os arquivos
// dela vão para a lixeira; faixa sem timeline também; tocando, é recusada.
func TestRemoveSong(t *testing.T) {
	srv, sh, dir, _ := newImportServer(t)
	for _, name := range []string{"A.wav", "B.wav", "C.wav"} {
		os.WriteFile(filepath.Join(dir, name), wavBytes(3*time.Second), 0o644)
		if err := srv.analyze(name, false, "", false, false); err != nil {
			t.Fatal(err)
		}
	}
	waitJob(t, srv, 3)
	os.WriteFile(filepath.Join(dir, "B.json.orig"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "Nova.wav"), wavBytes(time.Second), 0o644)
	if err := sh.Reorder([]string{"C.json", "B.json", "A.json"}); err != nil {
		t.Fatal(err)
	}
	if err := sh.SetLink("B.json", true, time.Second); err != nil {
		t.Fatal(err)
	}

	where, err := srv.removeSong("B.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"B.json", "B.json.orig", "B.wav"} {
		if exists(filepath.Join(dir, f)) || !exists(filepath.Join(dir, where, f)) {
			t.Errorf("%s não foi para a lixeira (%s)", f, where)
		}
	}
	var got []string
	for _, so := range sh.Songs() {
		got = append(got, so.File)
	}
	order, _ := os.ReadFile(filepath.Join(dir, show.OrderFile))
	if strings.Join(got, ",") != "C.json,A.json" || strings.Contains(string(order), "B.json") {
		t.Fatalf("setlist %v, setlist.txt:\n%s", got, order)
	}

	// Faixa sem timeline; o áudio de uma música não sai sozinho.
	if _, err := srv.removeSong("Nova.wav"); err != nil || exists(filepath.Join(dir, "Nova.wav")) {
		t.Fatalf("faixa sem timeline: %v", err)
	}
	if _, err := srv.removeSong("A.wav"); err == nil {
		t.Fatal("removeu o áudio de uma música da setlist")
	}
	if _, err := srv.removeSong("X.json"); err == nil {
		t.Fatal("removeu uma música que não existe")
	}

	// Carregada e tocando: recusa; parada: descarrega e remove.
	if err := sh.Load(0); err != nil {
		t.Fatal(err)
	}
	sh.Play()
	if _, err := srv.removeSong("C.json"); err == nil || !exists(filepath.Join(dir, "C.json")) {
		t.Fatalf("removeu a música tocando: %v", err)
	}
	sh.Pause()
	if _, err := srv.removeSong("C.json"); err != nil {
		t.Fatal(err)
	}
	if st := sh.Status(); st.Current != -1 || sh.Timeline() != nil || len(sh.Songs()) != 1 {
		t.Fatalf("depois de remover a carregada: %+v", st)
	}
}

// TestFreeMode: a análise no modo livre gera efeitos (aqui pelas regras), a
// setlist lembra a opção e o editor troca o efeito, recusando o inválido.
func TestFreeMode(t *testing.T) {
	srv, sh, dir, _ := newImportServer(t)
	os.WriteFile(filepath.Join(dir, "L.wav"), wavBytes(3*time.Second), 0o644)
	if err := srv.analyze("L.wav", false, "fria", true, false); err != nil {
		t.Fatal(err)
	}
	waitJob(t, srv, 1)
	path := filepath.Join(dir, "L.json")
	tl, err := show.LoadTimeline(path)
	if err != nil {
		t.Fatal(err)
	}
	c := tl.Cues[0]
	if c.Kind != "effect" || c.Effect == nil || c.Preset != "" || !strings.HasPrefix(c.Name, "EFEITO | ") { // pelas regras: um modo de pista frio da biblioteca, como efeito
		t.Fatalf("troca gerada: %+v", c)
	}
	if songs := srv.setlist().Songs; len(songs) != 1 || !songs[0].Free || songs[0].Temp != "fria" {
		t.Fatalf("setlist: %+v", songs)
	}
	if _, err := sh.Compile(tl); err != nil {
		t.Fatal(err)
	}

	tl.Cues[0].Name, tl.Cues[0].Rate = "", 3
	tl.Cues[0].Effect = &show.Effect{Type: show.FXChase, Colors: []string{"#FF00AA", "#0000ff", "#00ff00"}, Beats: 3}
	raw, _ := json.Marshal(tl)
	if err := srv.saveTimeline("L.json", raw); err != nil {
		t.Fatal(err)
	}
	saved, _ := show.LoadTimeline(path)
	if c := saved.Cues[0]; c.Name != "LIVRE | CORRIDA" || c.Rate != 0 || len(c.Effect.Colors) != 2 || c.Effect.Colors[0] != "#ff00aa" || c.Effect.Beats != 4 {
		t.Fatalf("efeito salvo: %+v %+v", c, c.Effect)
	}
	bad := strings.Replace(string(raw), `"chase"`, `"laser"`, 1)
	if err := srv.saveTimeline("L.json", json.RawMessage(bad)); err == nil {
		t.Fatal("efeito desconhecido deveria ser recusado")
	}
}
