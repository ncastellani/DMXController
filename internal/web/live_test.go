package web

import (
	"testing"
	"time"
)

// A prévia da aba Músicas nos canhões: o navegador recebe o id dele (hello),
// liga, vê no state que conduz, e a prévia sai quando ele desconecta.
func TestLivePreview(t *testing.T) {
	_, sh, dir, hs := newImportServer(t)
	addSong(t, dir, "m")
	sh.Scan()

	a := dial(t, hs)
	hello := a.waitFor("hello", func(m map[string]any) bool { return m["type"] == "hello" })
	me := hello["client"].(string)
	liveOf := func(m map[string]any) map[string]any { return m["live"].(map[string]any) }

	a.send(map[string]any{"type": "live", "str": "x.json", "down": true, "t": 0})
	a.waitFor("liveError", func(m map[string]any) bool {
		return m["type"] == "liveError" && m["file"] == "x.json" && m["error"] != ""
	})

	a.send(map[string]any{"type": "live", "str": "m.json", "down": true, "t": 3.5, "playing": true})
	a.waitFor("state com a prévia nos canhões", isState(func(m map[string]any) bool {
		return liveOf(m)["file"] == "m.json" && liveOf(m)["by"] == me
	}))

	// Outro aparelho não desliga a prévia de quem conduz.
	b := dial(t, hs)
	b.waitFor("hello", func(m map[string]any) bool { return m["type"] == "hello" && m["client"] != me })
	b.send(map[string]any{"type": "live", "down": false})
	time.Sleep(50 * time.Millisecond)
	if sh.LiveStatus().Owner != me {
		t.Fatal("outro aparelho não deveria desligar a prévia de quem conduz")
	}

	a.conn.CloseNow()
	b.waitFor("prévia fora dos canhões", isState(func(m map[string]any) bool { return liveOf(m)["file"] == "" }))
}
