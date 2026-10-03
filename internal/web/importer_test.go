package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"luzes/internal/analysis"
	"luzes/internal/audio"
	"luzes/internal/control"
	"luzes/internal/engine"
	"luzes/internal/fixture"
	"luzes/internal/show"
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
	c, err := control.LoadConsole("../../config/console.yaml")
	if err != nil {
		t.Fatal(err)
	}
	fixtures, _ := fixture.Load("../../config/fixtures.yaml", "../../config/patch.yaml")
	rig, err := control.NewRig(fixtures, c, filepath.Join(t.TempDir(), "patch.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pos []*fixture.Fixture
	for i := range rig.Fixtures {
		pos = append(pos, rig.Logical(i))
	}
	eng := &engine.Engine{}
	ctl := control.NewController(c, eng, rig)
	dir := t.TempDir()
	sh := show.New(dir, show.NewPresets(c.Workspace, pos), eng, nil, c.Limits.Flash, ctl.Notice)
	srv := New(ctl, eng, sh, nil, nil, nil, "")
	srv.SetImporter(&Importer{Dir: dir, Catalog: analysis.Catalog(c, pos)})
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

func TestAnalyzeWithLyrics(t *testing.T) {
	srv, sh, dir, hs := newImportServer(t)
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
	letra := "Teu vestido verde-limão\r\nbrilhando no salão"
	if err := srv.handle(c, &inMsg{Type: "analyze", Str: "Balada.wav", Temp: "fria", AI: true, Lyrics: &letra}, time.Now()); err != nil {
		t.Fatal(err)
	}
	j := waitJobs(1)
	if j.Error != "" || j.Temp != "fria" || j.AI || j.Lyrics { // sem chave: sem IA, e a letra fica só guardada
		t.Fatalf("job: %+v", j)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "Balada.letra.txt"))
	if string(got) != "Teu vestido verde-limão\nbrilhando no salão\n" {
		t.Fatalf("letra guardada: %q", got)
	}
	resp, _ = http.Get(hs.URL + "/api/lyrics?file=Balada.wav")
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if body.String() != string(got) {
		t.Fatalf("GET letra: %q", body)
	}
	sl := srv.setlist()
	if len(sl.Songs) != 1 || len(sl.Orphans) != 0 {
		t.Fatalf("setlist: %+v", sl)
	}
	if so := sl.Songs[0]; so.WAV != "Balada.wav" || !so.Lyrics || so.Temp != "fria" || so.AI || so.Edited {
		t.Fatalf("música: %+v", so)
	}

	// Reanálise da música carregada: a timeline nova vale na hora; letra nil
	// mantém a guardada, "" apaga.
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
	if !exists(filepath.Join(dir, "Balada.letra.txt")) {
		t.Fatal("letra nil não deveria apagar")
	}
	vazia := ""
	srv.handle(c, &inMsg{Type: "analyze", Str: "Balada.wav", Lyrics: &vazia}, time.Now())
	waitJobs(3)
	if exists(filepath.Join(dir, "Balada.letra.txt")) {
		t.Fatal("letra vazia deveria apagar o arquivo")
	}
	if err := srv.handle(c, &inMsg{Type: "analyze", Str: "naoexiste.wav"}, time.Now()); err == nil {
		t.Fatal("faixa inexistente deveria falhar")
	}
	if cfg := srv.config(); len(cfg.Library) != 17 || cfg.Library[0].Color == "" {
		t.Fatalf("biblioteca de cores: %+v", cfg.Library)
	}
}

// TestUploadVoice: a versão cantada vira X.voz.wav, não aparece como música
// nova, toca na prévia com voz=1 e sai com noVoice.
func TestUploadVoice(t *testing.T) {
	srv, _, dir, hs := newImportServer(t)
	post := func(query string) int {
		resp, err := http.Post(hs.URL+"/api/upload?"+query, "audio/wav", bytes.NewReader(wavBytes(3*time.Second)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := post("name=Doce.wav&voz=1"); c != http.StatusBadRequest {
		t.Fatalf("versão cantada sem o playback: %d", c)
	}
	if c := post("name=Doce.voz.wav&analisar=0"); c != http.StatusBadRequest {
		t.Fatalf("playback com nome de versão cantada: %d", c)
	}
	if c := post("name=Doce.wav&analisar=0"); c != http.StatusAccepted {
		t.Fatalf("playback: %d", c)
	}
	if c := post("name=Doce.wav&voz=1"); c != http.StatusAccepted {
		t.Fatalf("versão cantada: %d", c)
	}
	if c := post("name=Doce.wav&voz=1"); c != http.StatusAccepted { // troca a anterior
		t.Fatalf("versão cantada de novo: %d", c)
	}
	if !exists(filepath.Join(dir, "Doce.voz.wav")) {
		t.Fatal("Doce.voz.wav não foi gravado")
	}
	if m := srv.setlist(); len(m.Orphans) != 1 || m.Orphans[0] != "Doce.wav" {
		t.Fatalf("órfãs: %v", m.Orphans)
	}

	// Com a timeline (só regras: sem chave), a setlist mostra a versão cantada
	// e a prévia a toca.
	if err := srv.analyze("Doce.wav", true, "", nil, false); err != nil {
		t.Fatal(err)
	}
	if j := waitJob(t, srv, 1); j.Error != "" || j.Voice { // sem chave: sem IA, a versão cantada não entra
		t.Fatalf("análise: %+v", j)
	}
	m := srv.setlist()
	if len(m.Songs) != 1 || !m.Songs[0].Voice || len(m.Orphans) != 0 {
		t.Fatalf("setlist: %+v", m)
	}
	resp, err := http.Get(hs.URL + "/api/audio?file=Doce.json&voz=1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "audio/wav" {
		t.Fatalf("áudio com voz: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	if err := srv.analyze("Doce.wav", false, "", nil, true); err != nil {
		t.Fatal(err)
	}
	waitJob(t, srv, 2)
	if exists(filepath.Join(dir, "Doce.voz.wav")) || srv.setlist().Songs[0].Voice {
		t.Fatal("noVoice não apagou a versão cantada")
	}
}
