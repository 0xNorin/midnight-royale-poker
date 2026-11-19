package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"time"
)

const sparkleInterval = 500 * time.Millisecond
const introPurple = "#9474ED"

type sparkleMsg struct{ Epoch uint64 }

func (m *Model) sparklesEnabled() bool {
	return m.Screen == "opening" && m.Modal == "" && m.sized && m.fitsMinimum() && !m.Config.ReducedMotion && m.Assets.Intro != nil
}
func (m *Model) stopSparkles() {
	if m.sparkleActive || m.sparklePending {
		m.sparkleEpoch++
		m.sparkleActive, m.sparklePending = false, false
	}
}
func (m *Model) sparkleCommand() tea.Cmd {
	if !m.sparklesEnabled() {
		m.stopSparkles()
		return nil
	}
	if !m.sparkleActive {
		m.sparkleActive = true
		m.sparkleEpoch++
		m.sparkleStep = 0
	}
	if m.sparklePending {
		return nil
	}
	m.sparklePending = true
	epoch := m.sparkleEpoch // never capture mutable model state in a timer closure
	return tea.Tick(sparkleInterval, func(time.Time) tea.Msg { return sparkleMsg{Epoch: epoch} })
}
func (m *Model) advanceSparkles(epoch uint64) bool {
	if epoch != m.sparkleEpoch || !m.sparklePending || !m.sparklesEnabled() {
		return false
	}
	m.sparklePending = false
	m.sparkleStep++
	return true
}

func (m *Model) openingFrame(c *Canvas, x, y, w, h int, t Theme) {
	// Preserve the authored logo placement. At minimum height, use the existing
	// outer margin instead of squeezing or clipping the logo, button or credit.
	padY := min(1, max(0, (m.H-h-4)/2))
	left, right := max(1, x-4), min(m.W-2, x+w+3)
	top, bottom := max(0, y-padY-1), min(m.H-1, y+h+padY)
	edge := strings.Repeat(m.sym("─", "-"), right-left-1)
	c.put(left, top, paint(m.sym("┌", "+")+edge+m.sym("┐", "+"), introPurple, t.BG, false))
	c.put(left, bottom, paint(m.sym("└", "+")+edge+m.sym("┘", "+"), introPurple, t.BG, false))
	side := strings.TrimSuffix(strings.Repeat(paint(m.sym("│", "|"), introPurple, t.BG, false)+"\n", bottom-top-1), "\n")
	c.put(left, top+1, side)
	c.put(right, top+1, side)
}

func (m *Model) openingSparkles(c *Canvas, x, y int, t Theme) {
	if m.Config.ReducedMotion {
		return
	}
	a := m.Assets.Intro
	// Two authored ornaments animate; the + and . stay fixed. Two other tiny
	// sparkles occupy whitespace beside the logo. Original asset bytes never change.
	stars := []struct {
		x, y            int
		original, glyph string
		colors          [3]string
	}{
		{9, 2, "·", "·", [3]string{"#599BF1", "#38DCE8", "#72F5FF"}},
		{47, 2, "✦", "✦", [3]string{"#92265F", "#DF3C99", "#FA65C7"}},
		{3, 12, " ", "+", [3]string{"#6345B8", introPurple, "#CA60E4"}},
		{54, 13, " ", "·", [3]string{"#92265F", "#DF3C99", "#FA65C7"}},
	}
	cell := func(px, py int) string {
		if py < 0 || py >= len(a.Rows) || px < 0 || px >= a.Width {
			return ""
		}
		return ansi.Strip(ansi.Cut(a.Rows[py], px, px+1))
	}
	for i, s := range stars {
		if cell(s.x, s.y) != s.original {
			continue
		} // never overwrite unexpected artwork
		if s.original != " " {
			c.put(x+s.x, y+s.y, fill(1, t.BG))
		}
		phase := (m.sparkleStep + i*4) % 16
		// Eight-second, staggered fade: hidden -> dim -> medium -> bright -> dim.
		level := -1
		switch {
		case phase >= 4 && phase < 6:
			level = 0
		case phase >= 6 && phase < 8:
			level = 1
		case phase >= 8 && phase < 10:
			level = 2
		case phase >= 10 && phase < 12:
			level = 1
		case phase >= 12 && phase < 14:
			level = 0
		}
		if level < 0 {
			continue
		}
		px, py := s.x, s.y
		// Move only between complete fades, about once every 24 seconds, and only
		// into verified empty cells. The next fade never moves an already lit star.
		switch ((m.sparkleStep + i*4) / 16 / 3) % 3 {
		case 1:
			px--
			py++
		case 2:
			px++
		}
		if (px != s.x || py != s.y) && cell(px, py) != " " {
			px, py = s.x, s.y
		}
		c.put(x+px, y+py, paint(m.sym(s.glyph, "*"), s.colors[level], t.BG, false))
	}
}

// splitCollectionBadge upper-cases a collection display name and splits it
// into two word-balanced lines for the decorative two-line selector badge.
func splitCollectionBadge(name string) [2]string {
	words := strings.Fields(strings.ToUpper(name))
	if len(words) == 0 {
		return [2]string{name, ""}
	}
	mid := (len(words) + 1) / 2
	return [2]string{strings.Join(words[:mid], " "), strings.Join(words[mid:], " ")}
}

// Returns the heading row and card top; rendering and mouse hit registration
// share these positions through the existing picker, never a second hit table.
func (m *Model) collectionBadge(c *Canvas, t Theme) (headingY, cardTop int) {
	ornament := m.sym("✦", "*")
	offset := 0
	if hs, ok := m.collectionHighScoreLine(); ok {
		c.center(2, paint(hs, t.Accent, t.BG, true))
		offset = 1
	}
	if m.H < 38 {
		c.center(2+offset, paint(ornament+" "+m.collection().Name+" "+ornament, t.Accent, t.BG, true))
		return 3 + offset, 5 + offset // consume the old spacer; keep minimum-size cards/actions in place
	}
	lines := splitCollectionBadge(m.collection().Name)
	inner := max(ansi.StringWidth(lines[0]), ansi.StringWidth(lines[1])) + 4
	if inner%2 == 0 {
		inner++
	} // a single centered ornament with equal solid arms
	edge := strings.Repeat(m.sym("═", "="), (inner-1)/2)
	c.center(2+offset, paint(m.sym("╔", "+")+edge+ornament+edge+m.sym("╗", "+"), t.Accent, t.BG, false))
	for i, line := range lines {
		left := (inner - ansi.StringWidth(line)) / 2
		row := m.sym("║", "|") + strings.Repeat(" ", left) + line + strings.Repeat(" ", inner-left-ansi.StringWidth(line)) + m.sym("║", "|")
		c.center(3+offset+i, paint(row, t.Accent, t.BG, true))
	}
	c.center(5+offset, paint(m.sym("╚", "+")+edge+ornament+edge+m.sym("╝", "+"), t.Accent, t.BG, false))
	return 6 + offset, 8 + offset
}
