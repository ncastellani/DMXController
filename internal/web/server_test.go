package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
	"github.com/ncastellani/DMXController/internal/fixture"
	"github.com/ncastellani/DMXController/internal/show"
)

// newServer monta o show real (console.yaml + workspace do QLC+) com um
// patch temporário.
func newServer(t *testing.T, token string) (*httptest.Server, *control.Controller, *engine.Engine) {
	t.Helper()
	c, err := control.LoadConsole("../../config/console.yaml")
	if err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(t.TempDir(), "patch.yaml")
	src, _ := os.ReadFile("../../config/patch.yaml")
	os.WriteFile(patch, src, 0o644)
	fixtures, err := fixture.Load("../../config/fixtures.yaml", patch)
	if err != nil {
		t.Fatal(err)
	}
	rig, err := fixture.NewRig(fixtures, []int{1, 8, 15}, patch)
	if err != nil {
		t.Fatal(err)
	}
	eng := &engine.Engine{}
	ctl := control.NewController(c, eng, rig)
	sh := show.New(t.TempDir(), show.NewPresets(c.Workspace, nil), eng, nil, c.Limits.Flash, ctl.Notice)
	srv := New(ctl, eng, sh, nil, nil, nil, token)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go srv.Run(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
				var f dmx.Frame
				ctl.Tick(time.Now())
				eng.Render(time.Now(), &f) // a prévia lê eng.Last()
			}
		}
	}()
	hs := httptest.NewServer(srv.Handler())
	t.Cleanup(hs.Close)
	return hs, ctl, eng
}

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
}

func dial(t *testing.T, hs *httptest.Server) *wsClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(hs.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return &wsClient{t, conn}
}

func (c *wsClient) send(m map[string]any) {
	data, _ := json.Marshal(m)
	if err := c.conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		c.t.Fatal(err)
	}
}

// waitFor lê mensagens até uma satisfazer ok.
func (c *wsClient) waitFor(what string, ok func(map[string]any) bool) map[string]any {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			c.t.Fatalf("esperando %s: %v", what, err)
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		if ok(m) {
			return m
		}
	}
}

func isState(f func(map[string]any) bool) func(map[string]any) bool {
	return func(m map[string]any) bool { return m["type"] == "state" && f(m) }
}

func active(m map[string]any, layer string) int {
	return int(m["active"].(map[string]any)[layer].(float64))
}

func position(m map[string]any, i int) map[string]any {
	return m["positions"].([]any)[i].(map[string]any)
}

func TestConfigAndPress(t *testing.T) {
	hs, _, _ := newServer(t, "")
	c := dial(t, hs)
	cfg := c.waitFor("config", func(m map[string]any) bool { return m["type"] == "config" })
	if n := len(cfg["groups"].([]any)); n != 4 {
		t.Fatalf("grupos: %d", n)
	}
	// Botão 0 = MODOS DE PISTA | AZUL ROSA.
	c.send(map[string]any{"type": "press", "id": 0})
	c.waitFor("base ativa", isState(func(m map[string]any) bool { return active(m, "base") == 0 }))
	c.send(map[string]any{"type": "blackout"})
	st := c.waitFor("blackout", isState(func(m map[string]any) bool { return m["blackout"] == true }))
	if position(st, 0)["color"] != "#000000" {
		t.Fatalf("blackout deveria apagar a prévia: %v", position(st, 0))
	}
}

func TestTwoClientsSeeSameState(t *testing.T) {
	hs, _, _ := newServer(t, "")
	ipad, pc := dial(t, hs), dial(t, hs)
	ipad.send(map[string]any{"type": "press", "id": 32}) // CORES FIXAS | BRANCO
	// Evento e estado podem chegar em qualquer ordem.
	var sawState, sawEvent bool
	pc.waitFor("cor e evento no outro cliente", func(m map[string]any) bool {
		switch m["type"] {
		case "state":
			sawState = sawState || active(m, "base") == 32 && position(m, 1)["color"] == "#ffffff"
		case "event":
			sawEvent = sawEvent || strings.Contains(m["text"].(string), "BRANCO")
		}
		return sawState && sawEvent
	})
}

func TestFlashReleasedOnDisconnect(t *testing.T) {
	hs, ctl, _ := newServer(t, "")
	c := dial(t, hs)
	var flashID int
	for _, b := range ctl.Console().Buttons {
		if b.Layer == engine.Flash {
			flashID = b.ID
			break
		}
	}
	c.send(map[string]any{"type": "flash", "id": flashID, "down": true})
	c.waitFor("flash", isState(func(m map[string]any) bool { return active(m, "flash") == flashID }))
	c.conn.CloseNow()

	deadline := time.Now().Add(2 * time.Second)
	for ctl.State().Active[engine.Flash] != nil {
		if time.Now().After(deadline) {
			t.Fatal("flash continuou depois da conexão cair")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManualAndSwap(t *testing.T) {
	hs, ctl, _ := newServer(t, "")
	c := dial(t, hs)
	c.send(map[string]any{"type": "manual", "pos": 2, "ch": 2, "value": 200})
	c.waitFor("manual", isState(func(m map[string]any) bool {
		ch := position(m, 2)["channels"].([]any)[2].(map[string]any)
		return ch["manual"] == true && ch["value"] == 200.0
	}))
	c.send(map[string]any{"type": "swap", "a": 0, "b": 1})
	c.waitFor("troca", isState(func(m map[string]any) bool {
		return position(m, 0)["address"] == 8.0 && position(m, 1)["address"] == 1.0
	}))
	if ctl.State().Positions[0].Name != "esquerda" {
		t.Fatal("o nome da posição não deveria mudar")
	}
}

func TestToken(t *testing.T) {
	hs, _, _ := newServer(t, "segredo")
	get := func(url string, cookie *http.Cookie) *http.Response {
		req, _ := http.NewRequest("GET", url, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp
	}
	if r := get(hs.URL+"/", nil); r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sem token: %d", r.StatusCode)
	}
	r := get(hs.URL+"/?t=segredo", nil)
	if r.StatusCode != http.StatusOK || len(r.Cookies()) == 0 {
		t.Fatalf("com token: %d %v", r.StatusCode, r.Cookies())
	}
	if r := get(hs.URL+"/", r.Cookies()[0]); r.StatusCode != http.StatusOK {
		t.Fatalf("com cookie: %d", r.StatusCode)
	}
}
