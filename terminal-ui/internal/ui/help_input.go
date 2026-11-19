package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"time"
)

// One geometry/input path for Help over selection, Shop and gameplay.
func (m *Model) helpGeometry() (x, y, w, h int) {
	w, h = min(82, m.W-4), min(21, m.H-4)
	return (m.W - w) / 2, (m.H - h) / 2, w, h
}
func (m *Model) helpViewHeight() int {
	_, _, _, h := m.helpGeometry()
	return max(1, h-8) // title, separators, footer, borders and bottom padding
}
func (m *Model) helpCloseArea() Hit {
	x, y, w, h := m.helpGeometry()
	return Hit{"close", x + w - 10, y + h - 3, ansi.StringWidth(" Close "), 1}
}
func (m *Model) scrollHelp(delta int) {
	m.HelpScroll = max(0, min(m.HelpScroll+delta, max(0, len(m.helpLines())-m.helpViewHeight())))
}
func (m *Model) helpMouse(p tea.Mouse, wheel bool) {
	if !m.fitsMinimum() {
		return
	}
	if wheel {
		switch p.Button {
		case tea.MouseWheelUp:
			m.scrollHelp(-m.ScrollStep)
		case tea.MouseWheelDown:
			m.scrollHelp(m.ScrollStep)
		}
		return
	}
	close := m.helpCloseArea()
	if p.Button == tea.MouseLeft && close.contains(p.X, p.Y) {
		now := time.Now()
		m.clickHit(close.ID, now)
		if m.Modal == "" {
			m.helpCloseHit = close
			m.helpCloseUntil = now.Add(doubleClickInterval)
		}
	}
}
