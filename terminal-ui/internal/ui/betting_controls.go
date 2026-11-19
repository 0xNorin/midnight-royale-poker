package ui

import (
	"github.com/charmbracelet/x/ansi"
)

// Keep the engine's action indexes unchanged; the two UI-only controls follow
// them in keyboard order and never dispatch a poker action on Enter.
func (m *Model) moveBetControl(delta int) bool {
	if m.Phase != "bet" {
		return false
	}
	if _, _, ok := m.betAmountRange(); !ok {
		return false
	}
	count := len(m.actions())
	index := m.Action
	switch m.Focus {
	case "bet-plus":
		index = count
	case "bet-minus":
		index = count + 1
	case "actions":
	default:
		return false
	}
	n := count + 2
	index = (index + delta + n) % n
	switch index {
	case count:
		m.Focus = "bet-plus"
	case count + 1:
		m.Focus = "bet-minus"
	default:
		m.Focus = "actions"
		m.Action = index
	}
	return true
}

func (m *Model) betControls(c *Canvas, rightX, y int, t Theme) int {
	if m.Phase != "bet" {
		return rightX
	}
	if _, _, ok := m.betAmountRange(); !ok {
		return rightX
	}
	minus := m.sym("−", "-")
	minusX := rightX - ansi.StringWidth(" "+minus+" ")
	plusX := minusX - 1 - ansi.StringWidth(" + ")
	for index, button := range []struct {
		x         int
		label, id string
	}{{plusX, "+", "bet-plus"}, {minusX, minus, "bet-minus"}} {
		label := " " + button.label + " "
		bg := m.betAdjustmentBackground(index, button.id, t)
		if c.HoverID == button.id && bg == t.Accent {
			bg = betFocusGold
		}
		c.put(button.x, y, paint(label, t.BG, bg, m.Focus == button.id))
		c.hit(button.id, button.x, y, ansi.StringWidth(label), 1)
	}
	return plusX - 3
}
