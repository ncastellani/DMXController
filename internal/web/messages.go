package web

import (
	"fmt"

	"github.com/ncastellani/DMXController/internal/control"
	"github.com/ncastellani/DMXController/internal/engine"
)

type configMsg struct {
	Type   string `json:"type"`
	Groups []struct {
		Name    string      `json:"name"`
		Layer   string      `json:"layer"`
		Buttons []buttonMsg `json:"buttons"`
	} `json:"groups"`
	FlashLimit  float64 `json:"flashLimit"`  // segundos
	StrobeLimit float64 `json:"strobeLimit"` // segundos
	// Library são cores fixas do workspace que não estão nos botões (para o
	// editor da timeline; não aparecem no console).
	Library []libraryMsg `json:"library"`
}

type libraryMsg struct {
	Preset  string `json:"preset"`
	Caption string `json:"caption"`
	Color   string `json:"color"`
}

type buttonMsg struct {
	ID      int    `json:"id"`
	Caption string `json:"caption"`
	Color   string `json:"color,omitempty"`
	Key     string `json:"key,omitempty"`
	Preset  string `json:"preset"` // id usado na timeline ("qlc:47")
}

type stateMsg struct {
	Type      string         `json:"type"`
	Blackout  bool           `json:"blackout"`
	Active    map[string]int `json:"active"` // camada → id do botão (-1 = nada)
	Identify  int            `json:"identify"`
	Show      string         `json:"show"` // música conduzindo a luz ("" = nenhuma)
	Health    healthMsg      `json:"health"`
	Positions []positionMsg  `json:"positions"`
}

type positionMsg struct {
	Name     string       `json:"name"`
	Address  int          `json:"address"`
	Color    string       `json:"color"` // cor estimada que o canhão está emitindo
	Strobe   bool         `json:"strobe"`
	Channels []channelMsg `json:"channels"`
}

type channelMsg struct {
	Role   string `json:"role"`
	Value  int    `json:"value"`
	Manual bool   `json:"manual"`
}

type eventMsg struct {
	Type string `json:"type"`
	Time string `json:"time"`
	Text string `json:"text"`
}

type eventsMsg struct {
	Type  string     `json:"type"`
	Items []eventMsg `json:"items"`
}

func (s *Server) config() configMsg {
	c := s.ctl.Console()
	m := configMsg{
		Type:        "config",
		FlashLimit:  c.Limits.Flash.Seconds(),
		StrobeLimit: c.Limits.Strobe.Seconds(),
	}
	for _, g := range c.Groups {
		var grp struct {
			Name    string      `json:"name"`
			Layer   string      `json:"layer"`
			Buttons []buttonMsg `json:"buttons"`
		}
		grp.Name = g
		for _, b := range c.Buttons {
			if b.Group != g {
				continue
			}
			grp.Layer = b.Layer.String()
			bm := buttonMsg{ID: b.ID, Caption: b.Caption, Color: b.Color, Preset: fmt.Sprintf("qlc:%d", b.Function)}
			if b.Key != 0 {
				bm.Key = string(b.Key)
			}
			grp.Buttons = append(grp.Buttons, bm)
		}
		m.Groups = append(m.Groups, grp)
	}
	m.Library = []libraryMsg{}
	if s.importer != nil {
		for _, p := range s.importer.Catalog {
			if p.Library {
				m.Library = append(m.Library, libraryMsg{Preset: p.ID, Caption: p.Caption, Color: p.Hex})
			}
		}
	}
	return m
}

func (s *Server) snapshot() stateMsg {
	st := s.ctl.State()
	frame := s.eng.Last()
	m := stateMsg{Type: "state", Blackout: st.Blackout, Identify: st.Identify, Active: map[string]int{}}
	m.Show = s.eng.Status().Names[engine.Show]
	m.Health = s.health.snapshot()
	for _, l := range []engine.Layer{engine.Base, engine.Strobe, engine.Flash} {
		m.Active[l.String()] = id(st.Active[l])
	}
	for _, p := range st.Positions {
		c, strobe := p.Logical.Look(&frame)
		pm := positionMsg{
			Name:    p.Name,
			Address: p.Address,
			Color:   fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B),
			Strobe:  strobe,
		}
		for ch, role := range p.Logical.Profile.Channels {
			pm.Channels = append(pm.Channels, channelMsg{
				Role:   string(role),
				Value:  int(frame[p.Logical.Address+ch-1]),
				Manual: p.Manual[ch],
			})
		}
		m.Positions = append(m.Positions, pm)
	}
	return m
}

func id(b *control.Button) int {
	if b == nil {
		return -1
	}
	return b.ID
}
