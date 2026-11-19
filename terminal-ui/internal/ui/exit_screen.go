package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const exitScreen = "exit"

// Fixed VGA-inspired palette for the native exit screen.
const (
	exitBG     = "#000000" // black full-screen background
	exitPanel  = "#AA0000" // deep-red message panel
	exitBorder = "#550000" // darker red border
	exitBody   = "#FFFF55" // warm off-white body text
	exitHi     = "#FFFFFF" // bright white highlights
	exitURL    = "#55FFFF" // cyan URL / selected accent
	exitTri    = "#FF5555" // red triangular accent
)

// exitMaxText is the display width of the longest unwrapped copy line.
const exitMaxText = 63

const exitFooter = "Press Enter to return to your terminal..."

type exitStyle int

const (
	exitTextStyle exitStyle = iota
	exitTitleStyle
	exitLinkStyle
)

type exitLine struct {
	text  string
	style exitStyle
}

// exitCopy is the exact farewell copy. Blank lines are paragraph breaks that
// the compact layout collapses when the terminal is too short to show them.
var exitCopy = []exitLine{
	{"MIDNIGHT ROYALE: A terminal game by 0xNorin.", exitTitleStyle},
	{"", exitTextStyle},
	{"Quit already?", exitTitleStyle},
	{"", exitTextStyle},
	{"That's alright. Midnight Royale has released its grip on your", exitTextStyle},
	{"terminal and returned you safely to the operating system.", exitTextStyle},
	{"", exitTextStyle},
	{"For now.", exitTextStyle},
	{"", exitTextStyle},
	{"Somewhere at 0xNorin, another game is being compiled. Perhaps", exitTextStyle},
	{"another card table. Perhaps something entirely different.", exitTextStyle},
	{"We rarely know ourselves until it starts working.", exitTextStyle},
	{"", exitTextStyle},
	{"0xNorin makes small games, terminal oddities, pixel experiments", exitTextStyle},
	{"and software for people who still think a blinking cursor is", exitTextStyle},
	{"an invitation rather than an error message.", exitTextStyle},
	{"", exitTextStyle},
	{"Midnight Royale will continue to receive new collections and", exitTextStyle},
	{"surprises. Other games are coming too.", exitTextStyle},
	{"", exitTextStyle},
	{"You can keep track of all this unnecessary activity here:", exitTextStyle},
	{"", exitTextStyle},
	{"https://0xnorin.app/", exitLinkStyle},
	{"", exitTextStyle},
	{"Tell your friends. Or don't.", exitTextStyle},
	{"They'll probably find it eventually.", exitTextStyle},
	{"", exitTextStyle},
	{"Thanks for playing Midnight Royale.", exitTitleStyle},
	{"", exitTextStyle},
	{"-- 0xNorin", exitTextStyle},
}

// requestExit routes a deliberate quit action to the native exit screen. The
// first request opens the screen (consuming the input that triggered it); a
// request that arrives while the screen is already open terminates the program
// cleanly. The update-restart path (applyUpdateCommit) bypasses this and
// returns tea.Quit directly, so a restart never shows the farewell screen.
func (m *Model) requestExit() tea.Cmd {
	if m.Screen == exitScreen {
		return tea.Quit
	}
	m.goScreen(exitScreen)
	m.ExitScroll = 0
	return nil
}

// openQuitConfirm opens the global-title-screen quit confirmation, defaulting focus
// to "No" so a stray Enter can never quit.
func (m *Model) openQuitConfirm() {
	m.openModal("quit")
	m.ModalFocus = 1 // "No"
}

// quitConfirmChoice resolves the quit confirmation. Yes routes through the
// existing clean shutdown flow (requestExit -> farewell screen); No closes
// the dialog and returns to the title screen.
func (m *Model) quitConfirmChoice(quit bool) {
	if quit {
		m.Modal = ""
		m.QuitRequested = true
		return
	}
	m.Modal = ""
}

// openQuitToMenuConfirm opens the Quit-to-Main-Menu confirmation from the
// pause menu, defaulting focus to "No" so a stray Enter can never abandon the
// run.
func (m *Model) openQuitToMenuConfirm() {
	m.openModal("quittomenu")
	m.ModalFocus = 1 // "No"
}

// quitToMenuChoice resolves the Quit-to-Main-Menu confirmation. Yes abandons
// the active run and returns to character selection; No returns to the pause
// menu without touching the run.
func (m *Model) quitToMenuChoice(abandon bool) {
	if abandon {
		m.abandonRun()
		m.goScreen("select")
		return
	}
	m.Modal = "pause"
	m.ModalFocus = 0 // Resume
}

// exitScreenKey handles keyboard input while the exit screen is visible.
// Enter, Escape and q are deliberate exits; the arrow and page keys scroll the
// panel when it overflows a small terminal. Ctrl+C is handled earlier in
// Update and never reaches this method.
func (m *Model) exitScreenKey(k string, repeat bool) {
	switch k {
	case "enter", "esc", "q":
		m.QuitRequested = true
	case "up":
		m.ExitScroll--
	case "down":
		m.ExitScroll++
	case "pgup":
		m.ExitScroll -= 5
	case "pgdown":
		m.ExitScroll += 5
	}
}

// exitTextWidth returns the body text width that fits the terminal, clamped so
// the panel always stays on screen.
func (m *Model) exitTextWidth() int {
	tw := exitMaxText
	if fit := m.W - 8; fit < tw {
		tw = fit
	}
	if tw < 8 {
		tw = 8
	}
	return tw
}

// exitBodyRows wraps the copy to the current text width and decides how much
// of it to show. It returns the full list (with paragraph breaks), a compact
// list (breaks collapsed), or a scrollable window when even the compact list
// is taller than the terminal.
func (m *Model) exitBodyRows(textW int) (rows []exitLine, viewH int, scrollable bool) {
	var all []exitLine
	for _, ln := range exitCopy {
		for _, part := range strings.Split(ansi.Wrap(ln.text, textW, ""), "\n") {
			all = append(all, exitLine{part, ln.style})
		}
	}
	// panel chrome: top/bottom border; below the panel: footer.
	avail := m.H - 3
	if avail < 1 {
		avail = 1
	}
	if len(all) <= avail {
		return all, len(all), false
	}
	compact := make([]exitLine, 0, len(all))
	for _, ln := range all {
		if ln.text != "" {
			compact = append(compact, ln)
		}
	}
	if len(compact) <= avail {
		return compact, len(compact), false
	}
	return compact, avail, true
}

func exitFill(c *Canvas, x, y, w, h int, bg string) {
	for j := 0; j < h; j++ {
		c.put(x, y+j, fill(w, bg))
	}
}

// exitScreen renders the native DOS-style farewell screen. It is drawn at any
// terminal size (bypassing the gameplay resize hint) and never truncates
// essential copy: below the compact threshold the panel scrolls.
func (m *Model) exitScreen(c *Canvas, t Theme) {
	for i := range c.Lines {
		c.Lines[i] = fill(c.W, exitBG)
	}
	if m.W < 30 || m.H < 8 {
		m.exitScreenMinimal(c)
		return
	}
	m.exitScreenPanel(c)
}

func (m *Model) exitScreenMinimal(c *Canvas) {
	type row struct {
		s, fg string
		bold  bool
	}
	rows := []row{
		{"MIDNIGHT ROYALE", exitHi, true},
		{"Thanks for playing. https://0xnorin.app/", exitURL, false},
		{exitFooter, exitBody, false},
	}
	var rendered []string
	for _, r := range rows {
		for _, part := range strings.Split(ansi.Wrap(r.s, max(8, m.W-4), ""), "\n") {
			rendered = append(rendered, paint(part, r.fg, exitBG, r.bold))
		}
	}
	startY := max(0, (m.H-len(rendered))/2)
	for i, r := range rendered {
		c.put(max(0, (m.W-ansi.StringWidth(r))/2), startY+i, r)
	}
}

func (m *Model) exitScreenPanel(c *Canvas) {
	textW := m.exitTextWidth()
	body, viewH, scrollable := m.exitBodyRows(textW)

	if scrollable {
		maxScroll := len(body) - viewH
		if m.ExitScroll < 0 {
			m.ExitScroll = 0
		}
		if m.ExitScroll > maxScroll {
			m.ExitScroll = maxScroll
		}
	} else {
		m.ExitScroll = 0
	}
	start := m.ExitScroll
	visible := body[start : start+viewH]

	pad := 2
	panelW := textW + 2*pad + 2
	if panelW > m.W {
		panelW = m.W
	}
	panelX := (m.W - panelW) / 2
	if panelX < 0 {
		panelX = 0
	}
	panelH := viewH + 2 // top/bottom border
	total := panelH + 1 // footer below the panel
	blockY := 0
	if total < m.H {
		blockY = (m.H - total) / 2
	}
	panelY := blockY

	m.exitPanel(c, panelX, panelY, panelW, panelH, pad, visible)

	if footerY := panelY + panelH; footerY < m.H {
		c.center(footerY, paint(exitFooter, exitBody, exitBG, false))
	}

	if scrollable {
		if start > 0 {
			c.put(panelX+panelW/2, panelY, paint(m.sym("▲", "^"), exitTri, exitBorder, false))
		}
		if start+viewH < len(body) {
			c.put(panelX+panelW/2, panelY+panelH-1, paint(m.sym("▼", "v"), exitTri, exitBorder, false))
		}
	}
}

func (m *Model) exitPanel(c *Canvas, x, y, panelW, panelH, pad int, visible []exitLine) {
	horiz := m.sym("─", "-")
	vert := m.sym("│", "|")
	tl, tr, bl, br := m.sym("┌", "+"), m.sym("┐", "+"), m.sym("└", "+"), m.sym("┘", "+")

	exitFill(c, x, y, panelW, panelH, exitPanel)

	// Border in the darker red.
	c.put(x, y, paint(tl+strings.Repeat(horiz, panelW-2)+tr, exitBorder, exitPanel, false))
	c.put(x, y+panelH-1, paint(bl+strings.Repeat(horiz, panelW-2)+br, exitBorder, exitPanel, false))
	for j := 1; j < panelH-1; j++ {
		c.put(x, y+j, paint(vert, exitBorder, exitPanel, false))
		c.put(x+panelW-1, y+j, paint(vert, exitBorder, exitPanel, false))
	}

	// Body text, left-aligned with a two-cell inset.
	for i, ln := range visible {
		fg, bold := exitBody, false
		switch ln.style {
		case exitTitleStyle:
			fg, bold = exitHi, true
		case exitLinkStyle:
			fg, bold = exitURL, true
		}
		c.put(x+1+pad, y+1+i, paint(ln.text, fg, exitPanel, bold))
	}
}
