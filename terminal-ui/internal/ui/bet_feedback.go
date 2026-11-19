package ui

import (
	tea "charm.land/bubbletea/v2"
	"time"
)

const betFeedbackDuration = 120 * time.Millisecond
const betFocusGold = "#F1D39B"

type betFeedbackMsg struct{ ID uint64 }

func (m *Model) flashBetAdjustment(delta int, now time.Time) {
	index := 0
	if delta < 0 {
		index = 1
	}
	m.betFeedbackUntil[index] = now.Add(betFeedbackDuration)
}

// One pending wakeup serves both buttons. Rapid activation extends deadlines,
// never queues more timers. The current wakeup reschedules only what remains.
func (m *Model) betFeedbackCommand(now time.Time) tea.Cmd {
	if m.Screen != "game" || m.Phase != "bet" || m.Modal != "" || !m.DrawerOpen {
		m.betFeedbackUntil = [2]time.Time{}
	}
	var next time.Time
	for i, until := range m.betFeedbackUntil {
		if !until.After(now) {
			m.betFeedbackUntil[i] = time.Time{}
			continue
		}
		if next.IsZero() || until.Before(next) {
			next = until
		}
	}
	if m.betFeedbackPending || next.IsZero() {
		return nil
	}
	m.betFeedbackPending = true
	m.betFeedbackID++
	id := m.betFeedbackID
	return tea.Tick(next.Sub(now), func(time.Time) tea.Msg { return betFeedbackMsg{ID: id} })
}
func (m *Model) betFeedbackTick(id uint64, now time.Time) tea.Cmd {
	if !m.betFeedbackPending || id != m.betFeedbackID {
		return nil
	}
	m.betFeedbackPending = false
	return m.betFeedbackCommand(now)
}
func (m *Model) betAdjustmentBackground(index int, id string, t Theme) string {
	if !m.betFeedbackUntil[index].IsZero() {
		return t.Good
	}
	if m.Focus == id {
		return betFocusGold
	}
	return t.Accent
}
