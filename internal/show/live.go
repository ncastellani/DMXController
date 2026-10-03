package show

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ncastellani/DMXController/internal/dmx"
	"github.com/ncastellani/DMXController/internal/engine"
)

// Prévia nos canhões: a prévia da aba Músicas (o .wav tocando no navegador)
// levada à luz de verdade. O navegador manda a posição da prévia; a camada
// Live do engine desenha o Program da música (o mesmo do show) nessa
// posição, e as edições salvas valem na hora (Update). Tocando, a luz anda
// com a prévia; pausada, fica parada no quadro daquele instante, sem strobo.
// Sem notícia do navegador por LiveTTL, a luz sai (aparelho dormiu, Wi-Fi
// caiu).

// LiveTTL é quanto a prévia nos canhões sobrevive sem notícia do navegador.
const LiveTTL = 2 * time.Second

// liveDrift é o quanto a posição do navegador pode se afastar do relógio
// interno antes de ser corrigida (evita saltos pequenos a cada aviso).
const liveDrift = 60 * time.Millisecond

// LiveStatus diz qual música a prévia está levando aos canhões.
type LiveStatus struct {
	File  string // X.json da setlist ("" = desligada)
	Owner string // quem conduz (o navegador que ligou)
}

// live guarda a prévia nos canhões. Tem lock próprio: o engine chama Render
// com o lock dele, e live nunca chama o engine segurando l.mu.
type live struct {
	mu      sync.Mutex
	file    string // X.json
	path    string // caminho do JSON
	title   string
	owner   string
	prog    *Program
	dur     time.Duration
	pos     time.Duration // posição em since
	since   time.Time
	playing bool
	seen    time.Time // último aviso do navegador
}

// at devolve a posição agora. Requer l.mu.
func (l *live) at(now time.Time) time.Duration {
	if !l.playing {
		return l.pos
	}
	return min(l.pos+now.Sub(l.since), l.dur)
}

// liveSource é a camada Live do engine.
type liveSource struct{ l *live }

func (src liveSource) Render(_ time.Duration, f *dmx.Frame) {
	l := src.l
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if l.prog == nil || now.Sub(l.seen) > LiveTTL {
		return
	}
	pos := l.at(now)
	l.prog.Render(pos, f)
	if !l.playing || pos >= l.dur {
		// Parada num instante: o quadro fica, mas sem strobo (não pisca
		// indefinidamente numa fritação congelada).
		for _, fx := range l.prog.positions {
			fx.Steady(f)
		}
	}
}

// Live liga a prévia nos canhões (ou atualiza a posição dela): a música file
// (X.json da setlist), na posição t (s), tocando ou parada. owner identifica
// o navegador. Com sync, é só o aviso periódico de posição: de quem não
// conduz a prévia, é ignorado (outro aparelho assumiu). Recusada com a
// música do show tocando.
func (s *Show) Live(owner, file string, t float64, playing, sync bool) error {
	l := &s.live
	l.mu.Lock()
	on := l.prog != nil
	if sync && (!on || l.owner != owner) {
		l.mu.Unlock()
		return nil
	}
	same := on && l.file == file
	l.mu.Unlock()

	var prog *Program
	var song Song
	if !same {
		if s.showPlaying() {
			return errors.New("a música do show está tocando: pare o show para ver a prévia nos canhões")
		}
		var ok bool
		if song, ok = s.Song(file); !ok {
			return fmt.Errorf("música %q não está na setlist", file)
		}
		if song.Err != "" {
			return errors.New(song.Err)
		}
		tl, err := LoadTimeline(song.JSON)
		if err != nil {
			return err
		}
		if prog, err = s.Compile(tl); err != nil {
			return err
		}
		song.Duration = timelineDur(tl)
	}

	now := time.Now()
	pos := min(max(0, secs(t)), secs(1e6))
	l.mu.Lock()
	started := false
	if prog != nil {
		l.file, l.path, l.title, l.prog, l.dur = file, song.JSON, song.Title, prog, secs(song.Duration)
		l.pos, l.since, l.playing = min(pos, l.dur), now, playing
		started = true
	} else if l.prog == nil || l.file != file {
		l.mu.Unlock()
		return nil // desligada enquanto compilava
	} else {
		pos = min(pos, l.dur)
		// Corrige o relógio só se mudou de estado ou se afastou da prévia.
		if d := l.at(now) - pos; playing != l.playing || d > liveDrift || d < -liveDrift {
			l.pos, l.since = pos, now
		} else if playing {
			l.pos, l.since = l.at(now), now
		}
		l.playing = playing
	}
	changed := l.owner != owner
	l.owner, l.seen = owner, now
	title := l.title
	l.mu.Unlock()

	if started {
		s.eng.Play(engine.Live, "prévia: "+title, liveSource{l}, now)
		s.notice("◉ prévia nos canhões: %s", title)
	} else if changed {
		s.notice("◉ prévia nos canhões: %s (outro aparelho assumiu)", title)
	}
	return nil
}

// StopLive tira a prévia dos canhões (why aparece no log; vazio = não avisa).
func (s *Show) StopLive(why string) {
	l := &s.live
	l.mu.Lock()
	on := l.prog != nil
	title := l.title
	l.file, l.path, l.title, l.owner, l.prog, l.playing = "", "", "", "", nil, false
	l.mu.Unlock()
	if !on {
		return
	}
	s.eng.Stop(engine.Live)
	if why != "" {
		s.notice("◉ prévia saiu dos canhões: %s (%s)", title, why)
	} else {
		s.notice("◉ prévia saiu dos canhões: %s", title)
	}
}

// LiveStatus diz se a prévia está nos canhões, e qual música.
func (s *Show) LiveStatus() LiveStatus {
	l := &s.live
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.prog == nil {
		return LiveStatus{}
	}
	return LiveStatus{File: l.file, Owner: l.owner}
}

// ExpireLive tira a prévia dos canhões se o navegador que a conduz sumiu
// (sem aviso por LiveTTL) ou, com owner, se é dele (desconectou).
func (s *Show) ExpireLive(owner string, now time.Time) {
	l := &s.live
	l.mu.Lock()
	gone := l.prog != nil && (now.Sub(l.seen) > LiveTTL || (owner != "" && l.owner == owner))
	l.mu.Unlock()
	if gone {
		s.StopLive("o aparelho parou de responder")
	}
}

// liveProgram troca o programa da prévia nos canhões, se ela mostra a
// música path (edição salva).
func (s *Show) liveProgram(path string, prog *Program, t *Timeline) {
	l := &s.live
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.prog != nil && l.path == path {
		l.prog, l.dur = prog, secs(timelineDur(t))
	}
}

// liveSong diz se a prévia nos canhões mostra a música path.
func (s *Show) liveSong(path string) bool {
	l := &s.live
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.prog != nil && l.path == path
}

// showPlaying diz se a música do show (ou a calibração) está tocando.
func (s *Show) showPlaying() bool {
	return s.loaded() && s.head.playing()
}

// timelineDur é a duração da música da timeline.
func timelineDur(t *Timeline) float64 {
	if t.Song.Duration > 0 {
		return t.Song.Duration
	}
	return t.Analysis.Duration
}
