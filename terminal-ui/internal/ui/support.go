package ui

import (
	"fmt"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"midnight-royale/internal/edition"
)

// Version is the single source of truth for the build/version string shown in
// Help > About and reported by --version. It is selected at compile time by
// the build edition (internal/edition).
var Version = edition.Version

// EditionLabel returns a short human label for the compiled edition.
func EditionLabel() string {
	return "Open edition"
}

// checkoutPendingColor is the amber/orange used for the waiting/confirming
// status states; the shop theme has no amber in its palette.
const checkoutPendingColor = "#E8A33D"

// checkoutNetworkLabel returns a human label for the payment network, keeping
// the raw network name identifiable (future methods must distinguish e.g. ETH
// on Base from Ethereum mainnet).

func checkoutNetworkLabel(network string) string {
	if network == "" {
		return ""
	}
	runes := []rune(network)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// copyFeedback returns a brief confirmation while a clipboard copy result is
// still recent — success on a nil error, otherwise a retryable error message.
func (m *Model) copyFeedback() string {
	if m.copyFeedbackUntil.IsZero() || !time.Now().Before(m.copyFeedbackUntil) {
		return ""
	}
	if m.copyFeedbackErr != nil {
		return "Could not copy to clipboard. You can retry or open it directly."
	}
	return "Copied to clipboard."
}

// openWhereToBuyETN opens the ETN exchange listing in the user's default
// external browser. It never renders the page inside the game or terminal.
// openGetStablecoin opens a USDC/USDT exchange listing in the user's
// default external browser. Kraken is one example provider, not the only
// option, and Midnight Royale is not affiliated with it.

// checkoutFocusIndex returns the Tab index of a checkout control, or -1.
// checkoutFocusSet moves focus to a named control when it is present, leaving
// focus unchanged otherwise (so spatial navigation never lands on an absent
// control such as the primary action while a quote is being observed).
// checkoutFocusSetFirst moves focus to the first control of ids that is
// present, and reports whether one was found. Used where a column continues
// into optional controls.
type settingItem struct {
	id, label, description string
}

// settingsItems lists the settings rows.
func (m *Model) settingsItems() []settingItem {
	items := []settingItem{
		{id: "reduced-motion", label: fmt.Sprintf("Reduced motion: %t", m.Config.ReducedMotion), description: "Immediate drawer reveal when on. No idle animations."},
		{id: "scroll-step", label: fmt.Sprintf("Mouse wheel: %d rows per step", m.ScrollStep), description: "Keyboard arrows always move one row; portrait dimensions stay fixed."},
		{id: "ascii", label: fmt.Sprintf("ASCII UI fallback: %t", m.Config.ASCII), description: "Uses letters for suits and simpler UI marks. Artwork stays Unicode."},
		{id: "sound", label: fmt.Sprintf("Sound effects: %t", m.SoundEffects), description: "Card and chip sounds during play."},
	}
	return items
}

// updateChannelLabel renders the user-facing channel name.
func updateChannelLabel(channel string) string {
	if channel == "experimental" {
		return "Experimental"
	}
	return "Stable"
}

func (m *Model) activateSetting(id string) {
	switch id {
	case "reduced-motion":
		m.Config.ReducedMotion = !m.Config.ReducedMotion
		if m.Config.ReducedMotion {
			if m.DrawerOpen {
				m.Reveal = m.geometry().DrawerH
			} else {
				m.Reveal = 0
			}
		}
	case "scroll-step":
		m.ScrollStep = m.ScrollStep%5 + 1
	case "ascii":
		m.Config.ASCII = !m.Config.ASCII
	case "sound":
		m.SoundEffects = !m.SoundEffects
		if m.Audio != nil {
			m.Audio.SetEnabled(m.SoundEffects)
		}
	}
}

func (m *Model) settings(c *Canvas, t Theme) {
	c.put(8, 5, paint("Display, interaction, and sound.", t.Muted, t.BG, false))
	items := m.settingsItems()
	for i, item := range items {
		y := 7 + i*3
		c.button(8, y, item.label, fmt.Sprintf("setting:%d", i), m.Setting == i, t)
		c.put(9, y+1, paint(item.description, t.Muted, t.BG, false))
	}
	c.button(8, 7+len(items)*3+1, "Back", "back", m.settingsBackFocused(), t)
}

func (m *Model) modal(c *Canvas, t Theme) {
	if m.Modal == "pause" {
		m.pauseModal(c, t)
		return
	}
	if m.Modal == "victory" {
		m.victoryModal(c, t)
		return
	}
	if m.Modal == "quit" {
		m.quitModal(c, t)
		return
	}
	if m.Modal == "quittomenu" {
		m.quitToMenuModal(c, t)
		return
	}
	if m.Modal == "help" {
		x, y, w, h := m.helpGeometry()
		if m.helpBackdrop == nil {
			m.helpBackdrop = &Canvas{Lines: append([]string(nil), c.Lines...)}
			m.helpLeft, m.helpRight = make([]string, h), make([]string, h)
			for i := 0; i < h; i++ {
				m.helpLeft[i] = ansi.Cut(c.Lines[y+i], 0, x)
				m.helpRight[i] = ansi.Cut(c.Lines[y+i], x+w, c.W)
			}
		}
		// Compose on plain modal rows, not repeatedly through ANSI-heavy cards.
		overlay := newCanvas(w, h, t.Panel)
		overlay.put(0, 0, lipgloss.NewStyle().Border(t.Border).BorderForeground(lipgloss.Color(t.Accent)).Background(lipgloss.Color(t.Panel)).Width(w).Height(h).Render(""))
		pt := t
		pt.BG = t.Panel
		m.helpModal(overlay, 0, 0, w, h, t, pt)
		for i, line := range overlay.Lines {
			c.Lines[y+i] = m.helpLeft[i] + "\x1b[0m" + line + "\x1b[0m" + m.helpRight[i]
		}
		c.Hits = nil
		for _, hit := range overlay.Hits {
			hit.X += x
			hit.Y += y
			c.Hits = append(c.Hits, hit)
		}
		return
	}
	w, h := 82, 19
	x := (m.W - w) / 2
	y := (m.H - h) / 2
	c.Hits = nil
	c.panel(x, y, w, h, t)
	pt := t
	pt.BG = t.Panel
	title, body := "", ""
	switch m.Modal {
	case "levelresult":
		title, body = m.levelTransitionText()
	case "developer":
		title = "DEVELOPMENT INSPECTOR / DETERMINISTIC FIXTURES"
		body = fmt.Sprintf("Theme: %s   Screen: %s   Character: %s\nPhase: %s   Level: %d/5\nTerminal: %d x %d   Portrait: 96 x 72   Scroll: %d\n\nF3  Cycle screen\nF4  Cycle hand state / checkout status\nF5  Next character\nF6  Next image\nF7  Force opponent bust\nF8  Force player bust\nF9  Jump to level 4\n\nSERVER simulate paid: on a checkout screen, only with commerce\ndevelopment mode (--commerce-dev). Never in a release build.\n\nEsc closes this inspector. Launch with --dev for visible shortcuts.", t.Name, m.Screen, m.character().Name, m.Phase, m.Stage, m.W, m.H, m.Scroll)
	}
	c.put(x+3, y+2, paint(title, t.Accent, t.Panel, true))
	c.put(x+3, y+4, paint(body, t.Text, t.Panel, false))
	switch m.Modal {
	case "levelresult":
		c.button(x+3, y+h-2, "Continue", "level-continue", true, pt)
	default:
		c.button(x+w-10, y+h-2, "Close", "close", true, pt)
	}
}

// pauseModal sizes the actual border box from ANSI-aware wrapped content.
// Lip Gloss v2 Width/Height include borders; no second border subtraction.
func (m *Model) pauseModal(c *Canvas, t Theme) {
	const padding = 2
	w := min(82, m.W-4)
	inner := w - 2 - 2*padding
	title := ansi.Wrap("TAKE YOUR TIME", inner, "")
	body := ansi.Wrap("Your cards and portrait position are waiting right here.\n\nCompletion checkmarks and preferences are saved locally.", inner, "")
	items := pauseItems()
	titleH, bodyH := lipgloss.Height(title), lipgloss.Height(body)
	// Two borders, top padding, title/body separator, action separator,
	// every action, and a full blank row below Quit.
	h := 2 + 1 + titleH + 1 + bodyH + 1 + len(items) + 1
	x, y := (m.W-w)/2, (m.H-h)/2
	panel := lipgloss.NewStyle().Border(t.Border).BorderForeground(lipgloss.Color(t.Accent)).Background(lipgloss.Color(t.Panel)).Width(w).Height(h).Render("")
	c.put(x, y, panel)
	c.Hits = nil
	contentX, titleY := x+1+padding, y+2
	bodyY := titleY + titleH + 1
	c.put(contentX, titleY, paint(title, t.Accent, t.Panel, true))
	c.put(contentX, bodyY, paint(body, t.Text, t.Panel, false))
	pt := t
	pt.BG = t.Panel
	for i, item := range items {
		c.button(contentX, bodyY+bodyH+1+i, item.label, item.id, m.ModalFocus == i, pt)
	}
}

// quitModal renders the title-screen quit confirmation. It reuses the pause
// modal's panel chrome with a single question and two inline choices; "No"
// is focused by default so a stray Enter can never quit.
func (m *Model) quitModal(c *Canvas, t Theme) {
	const padding = 2
	title := "Do you want to quit Midnight Royale?"
	w := min(72, m.W-4)
	inner := w - 2 - 2*padding
	x, y := (m.W-w)/2, (m.H-9)/2
	c.Hits = nil
	c.panel(x, y, w, 9, t)
	pt := t
	pt.BG = t.Panel
	c.put(x+1+padding, y+2, paint(ansi.Wrap(title, inner, ""), t.Text, t.Panel, true))
	// Two inline choices centred under the question. quitConfirmItems returns
	// Yes then No; openQuitConfirm sets ModalFocus to 1 (No).
	cx := x + (w-12)/2
	c.button(cx, y+5, "Yes", "quit-yes", m.ModalFocus == 0, pt)
	c.button(cx+8, y+5, "No", "quit-no", m.ModalFocus == 1, pt)
}

// quitToMenuModal renders the Quit-to-Main-Menu confirmation shown from the
// pause menu. It mirrors the title-screen quit confirmation: one question, a
// one-line consequence, and Yes/No choices with "No" focused by default.
func (m *Model) quitToMenuModal(c *Canvas, t Theme) {
	const padding = 2
	title := "Quit current game?"
	body := "Your current run will be lost."
	w := min(72, m.W-4)
	inner := w - 2 - 2*padding
	x, y := (m.W-w)/2, (m.H-9)/2
	c.Hits = nil
	c.panel(x, y, w, 9, t)
	pt := t
	pt.BG = t.Panel
	c.put(x+1+padding, y+2, paint(ansi.Wrap(title, inner, ""), t.Text, t.Panel, true))
	c.put(x+1+padding, y+3, paint(body, t.Text, t.Panel, false))
	cx := x + (w-12)/2
	c.button(cx, y+5, "Yes", "quittomenu-yes", m.ModalFocus == 0, pt)
	c.button(cx+8, y+5, "No", "quittomenu-no", m.ModalFocus == 1, pt)
}
