package ui

import (
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type Theme struct {
	Name, BG, Panel, Text, Muted, Accent, Hot, Good, Bad, Card, Ink string
	Border                                                          lipgloss.Border
}

func (m *Model) theme() Theme {
	if m.Screen == "shop" || m.Screen == "detail" || m.Screen == "checkout" || m.Screen == "hosted-checkout" {
		return Theme{"MIDNIGHT ROYALE SHOP", "#07111F", "#0D2038", "#EAF4FF", "#8EACC8", "#4EA1FF", "#73C8FF", "#7ED7B5", "#FF838D", "#F2EBD9", "#142D4B", lipgloss.RoundedBorder()}
	}
	t := Theme{"MIDNIGHT ROYALE", "#0D101C", "#1B1B2C", "#F3EDDE", "#ABA6B7", defaultAccent, "#BE94C9", "#94C9AA", "#E07A7A", "#F2EBD9", "#25243B", lipgloss.RoundedBorder()}
	if accent := m.activeAccent(); accent != "" {
		t.Accent = accent
	}
	return t
}

// collectionAccentActive reports whether the current screen carries the active
// collection's accent. Shared screens (opening, adult-content, settings,
// purchases, commerce) and the shop theme keep their own accent.
func (m *Model) collectionAccentActive() bool {
	switch m.Screen {
	case "collection", "select", "intro", "game":
		return true
	}
	return false
}

// activeAccent returns the accent for the active collection on collection
// screens, or "" to keep the screen's default gold accent.
func (m *Model) activeAccent() string {
	if !m.collectionAccentActive() {
		return ""
	}
	return collectionAccent(m.SelectedCollection)
}

func paint(s, fg, bg string, bold bool) string {
	st := lipgloss.NewStyle().Foreground(lipgloss.Color(fg)).Background(lipgloss.Color(bg))
	if bold {
		st = st.Bold(true)
	}
	return st.Render(s)
}
func fill(w int, bg string) string { return paint(strings.Repeat(" ", max(0, w)), bg, bg, false) }

type Hit struct {
	ID         string
	X, Y, W, H int
}

func (h Hit) contains(x, y int) bool { return x >= h.X && x < h.X+h.W && y >= h.Y && y < h.Y+h.H }

type Canvas struct {
	W, H    int
	Lines   []string
	Hits    []Hit
	BG      string
	HoverID string
}

func newCanvas(w, h int, bg string) *Canvas {
	c := &Canvas{W: w, H: h, BG: bg}
	for i := 0; i < h; i++ {
		c.Lines = append(c.Lines, fill(w, bg))
	}
	return c
}

// put composes one final frame. Every splice reestablishes SGR state.
func (c *Canvas) put(x, y int, s string) {
	for dy, line := range strings.Split(s, "\n") {
		yy := y + dy
		if yy < 0 || yy >= c.H || x < 0 || x >= c.W {
			continue
		}
		line = ansi.Truncate(line, c.W-x, "")
		n := ansi.StringWidth(line)
		c.Lines[yy] = ansi.Cut(c.Lines[yy], 0, x) + "\x1b[0m" + line + "\x1b[0m" + ansi.Cut(c.Lines[yy], x+n, c.W)
	}
}
func (c *Canvas) text(x, y int, s string, t Theme) { c.put(x, y, paint(s, t.Text, t.BG, false)) }
func (c *Canvas) center(y int, s string)           { c.put(max(0, (c.W-ansi.StringWidth(s))/2), y, s) }
func (c *Canvas) hit(id string, x, y, w, h int)    { c.Hits = append(c.Hits, Hit{id, x, y, w, h}) }
func (c *Canvas) button(x, y int, label, id string, focus bool, t Theme) {
	fg, bg := t.Accent, t.Panel
	if focus || c.HoverID == id {
		fg, bg = t.BG, t.Accent
	}
	s := " " + label + " "
	c.put(x, y, paint(s, fg, bg, true))
	c.hit(id, x, y, ansi.StringWidth(s), 1)
}
func (c *Canvas) panel(x, y, w, h int, t Theme) {
	for j := 0; j < h; j++ {
		c.put(x, y+j, fill(w, t.Panel))
	}
	s := lipgloss.NewStyle().Border(t.Border).BorderForeground(lipgloss.Color(t.Accent)).Background(lipgloss.Color(t.Panel)).Width(w - 2).Height(h - 2).Render("")
	c.put(x, y, s)
}
func (c *Canvas) String() string { return strings.Join(c.Lines, "\n") }
func (m *Model) sym(u, a string) string {
	if m.Config.ASCII {
		return a
	}
	return u
}
func num(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return fmt.Sprint(n)
}
