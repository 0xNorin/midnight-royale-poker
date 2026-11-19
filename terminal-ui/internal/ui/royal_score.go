package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"midnight-royale/internal/run"
)

// resetRunScore clears the run-scoped Royal Score deductions at the start of a
// new character run.
func (m *Model) resetRunScore() {
	m.runLostHands = 0
	m.runLostStages = 0
	m.thisRunScore = 0
	m.runNewBest = false
	m.runCompletedCollection = false
}

// currentRoyalScore computes the live run score from the accumulated lost-hand
// and lost-stage deductions.
func (m *Model) currentRoyalScore() int {
	return run.RoyalScore(m.runLostHands, m.runLostStages)
}

// countLostHand records one lost hand (fold or showdown loss) against the
// current run, when progress is eligible. Ties are never counted here.
func (m *Model) countLostHand() {
	if m.progressEligible {
		m.runLostHands++
	}
}

// countLostStage records one lost level (retreat or retry) against the current
// run, when progress is eligible. A single bust hand that also drops a level
// therefore costs both deductions, matching the scoring rule.
func (m *Model) countLostStage() {
	if m.progressEligible {
		m.runLostStages++
	}
}

// beginRun resets run-scoped scoring at the start of a genuine character run.
// It is distinct from startRealMatch, which also runs on level transitions
// (advance/retreat/retry) that must preserve accumulated deductions.
func (m *Model) beginRun() {
	m.resetRunScore()
}

// recordRunScore persists a completed run's Royal Score as the character's
// best (highest wins) and captures the result for the victory presentation.
// It runs only when the run is progress-eligible; demo/dev fixtures never
// write a score.
func (m *Model) recordRunScore() {
	m.thisRunScore = m.currentRoyalScore()
	m.runNewBest = false
	m.runCompletedCollection = false
	if !m.progressEligible {
		return
	}
	ns := progressNamespace(m.character().Collection)
	id := m.character().ID
	wasComplete := m.collectionComplete()
	if m.Save.RecordBest(ns, id, m.thisRunScore) {
		m.runNewBest = true
		m.persistProgress()
	}
	if !wasComplete && m.collectionComplete() {
		m.runCompletedCollection = true
	}
}

// collectionComplete reports whether every character in the selected
// collection has a recorded best score.
func (m *Model) collectionComplete() bool {
	roster := m.roster()
	if len(roster) == 0 {
		return false
	}
	ns := progressNamespace(m.SelectedCollection)
	for _, ch := range roster {
		if _, ok := m.Save.Best(ns, ch.ID); !ok {
			return false
		}
	}
	return true
}

// collectionScore returns the sum of recorded bests for the selected
// collection and the number of characters with a recorded result.
func (m *Model) collectionScore() (sum, recorded int) {
	roster := m.roster()
	ids := make([]string, 0, len(roster))
	for _, ch := range roster {
		ids = append(ids, ch.ID)
	}
	return m.Save.CollectionScoreFor(progressNamespace(m.SelectedCollection), ids)
}

// collectionHighScoreLine returns the complete-collection "HIGH SCORE: N" line
// and whether it should be shown. It is shown only once every character in the
// collection has a recorded result.
func (m *Model) collectionHighScoreLine() (string, bool) {
	if !m.collectionComplete() {
		return "", false
	}
	sum, _ := m.collectionScore()
	return "HIGH SCORE: " + num(sum), true
}

// bestLabel returns a discreet "BEST 8,750" label for the selected character,
// or an empty string when no best score is recorded yet.
func (m *Model) bestLabel() string {
	best, ok := m.Save.Best(progressNamespace(m.character().Collection), m.character().ID)
	if !ok {
		return ""
	}
	return "BEST " + num(best)
}

// collectionCompletionBase returns the collection display name without its
// leading "The" or trailing "Collection", for prose like "You completed the
// Grand Riviera Hotel collection.".
func (m *Model) collectionCompletionBase() string {
	name, _ := m.collection().PlaqueTitle()
	return strings.TrimPrefix(name, "The ")
}

// victoryPlaque draws the small nameplate on the victory image showing the
// completed run's Royal Score. It is a discreet framed sign overlaid on the
// portrait, visible even after the victory dialog is dismissed.
func (m *Model) victoryPlaque(c *Canvas, t Theme) {
	label := "ROYAL SCORE"
	value := num(m.thisRunScore)
	inner := max(ansi.StringWidth(label), ansi.StringWidth(value))
	const pad = 2
	w := inner + 2*pad + 2
	x := (m.W - w) / 2
	y := m.geometry().ArtY + m.geometry().ArtH - 5
	if y < 2 {
		y = 2
	}
	horiz := m.sym("─", "-")
	top := m.sym("╭", "+") + strings.Repeat(horiz, w-2) + m.sym("╮", "+")
	bot := m.sym("╰", "+") + strings.Repeat(horiz, w-2) + m.sym("╯", "+")
	c.put(x, y, paint(top, t.Accent, t.BG, false))
	c.put(x, y+1, m.victoryPlaqueRow(label, inner, pad, t, false))
	c.put(x, y+2, m.victoryPlaqueRow(value, inner, pad, t, true))
	c.put(x, y+3, paint(bot, t.Accent, t.BG, false))
}

// victoryPlaqueRow composes one nameplate line for the victory plaque: accent
// side borders, dark padding, and centred text (label muted, value accent).
func (m *Model) victoryPlaqueRow(s string, inner, pad int, t Theme, bold bool) string {
	left := (inner - ansi.StringWidth(s)) / 2
	right := inner - left - ansi.StringWidth(s)
	color := t.Muted
	if bold {
		color = t.Accent
	}
	return paint(m.sym("│", "|"), t.Accent, t.BG, false) +
		paint(strings.Repeat(" ", pad+left), t.BG, t.BG, false) +
		paint(s, color, t.BG, bold) +
		paint(strings.Repeat(" ", pad+right), t.BG, t.BG, false) +
		paint(m.sym("│", "|"), t.Accent, t.BG, false)
}

// victoryModal renders the victory dialog with the run's Royal Score, the
// character's best, and the collection total. It keeps the three existing
// choices (Close window, Play again, Character selection).
func (m *Model) victoryModal(c *Canvas, t Theme) {
	const padding = 2
	w := min(80, m.W-4)
	inner := w - 2 - 2*padding
	pt := t
	pt.BG = t.Panel

	name := m.character().Name
	ns := progressNamespace(m.character().Collection)
	charBest, _ := m.Save.Best(ns, m.character().ID)
	total, recorded := m.collectionScore()
	charCount := len(m.roster())

	rows := []string{
		paint("You won!", t.Accent, t.Panel, true),
		paint(name+" / DEFEATED", t.Text, t.Panel, false),
		"",
	}

	rows = append(rows, victoryRow(t, inner, "THIS RUN", num(m.thisRunScore), t.Text, false))
	rows = append(rows, victoryRow(t, inner, "CHARACTER BEST", num(charBest), t.Text, false))
	if m.runNewBest {
		rows = append(rows, paint("NEW HIGH SCORE!", t.Good, t.Panel, true))
	}
	totalColor := t.Accent
	if m.runCompletedCollection {
		totalColor = t.Good
	}
	rows = append(rows, victoryRow(t, inner, "COLLECTION TOTAL", num(total)+fmt.Sprintf(" (%d/%d)", recorded, charCount), totalColor, true))

	rows = append(rows, "")
	ded := fmt.Sprintf("-250 × %d lost hand · -1,000 × %d lost stage", m.runLostHands, m.runLostStages)
	if m.runLostHands == 0 && m.runLostStages == 0 {
		ded = "No deductions — a perfect run"
	}
	rows = append(rows, paint(ded, t.Muted, t.Panel, false))

	if m.runCompletedCollection {
		rows = append(rows, "")
		rows = append(rows, paint("Congratulations! You completed the "+m.collectionCompletionBase()+" collection.", t.Good, t.Panel, true))
	}

	items := victoryItems()
	h := 2 + 1 + len(rows) + 1 + len(items) + 1
	x, y := (m.W-w)/2, (m.H-h)/2
	panel := lipgloss.NewStyle().Border(t.Border).BorderForeground(lipgloss.Color(t.Accent)).Background(lipgloss.Color(t.Panel)).Width(w).Height(h).Render("")
	c.put(x, y, panel)
	c.Hits = nil
	contentX := x + 1 + padding
	ry := y + 2
	for i, row := range rows {
		if row != "" {
			c.put(contentX, ry+i, row)
		}
	}
	buttonY := ry + len(rows) + 1
	for i, item := range items {
		c.button(contentX, buttonY+i, item.label, item.id, m.ModalFocus == i, pt)
	}
}

// victoryRow composes one label/value row for the victory modal within a fixed
// inner width: a muted label on the left and the value right-aligned.
func victoryRow(t Theme, inner int, label, value string, valueColor string, valueBold bool) string {
	labelPart := paint(label, t.Muted, t.Panel, false)
	valuePart := paint(value, valueColor, t.Panel, valueBold)
	gap := inner - ansi.StringWidth(label) - ansi.StringWidth(value)
	if gap < 1 {
		gap = 1
	}
	return labelPart + strings.Repeat(" ", gap) + valuePart
}
