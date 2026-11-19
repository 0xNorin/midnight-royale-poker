package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"midnight-royale/internal/art"
	"midnight-royale/internal/game"
	"strings"
)

const completedGold = "#DDB979"

// Availability badge colours: free and owned share a light green; locked
// is a light blue. Fixed values keep the status readable against the dark
// background and identical on every opponent-selection surface.
const (
	badgeFreeGreen  = "#9BE3B8"
	badgeLockedBlue = "#82C7E8"
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render().String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.ReportFocus = true
	v.WindowTitle = "Midnight Royale"
	return v
}
func (m *Model) render() *Canvas {
	t := m.theme()
	c := newCanvas(m.W, m.H, t.BG)
	defer func() { m.cacheHoverHits(c) }()
	if m.hoverEnabled() {
		c.HoverID = m.HoverID
	}
	if m.Screen == exitScreen {
		m.exitScreen(c, t)
		return c
	}
	if m.W < 100 || m.H < 35 {
		m.resizeHint(c, t)
		return c
	}
	if m.Modal != "help" {
		m.helpBackdrop = nil
	}
	if m.Modal == "help" && m.helpBackdrop != nil {
		copy(c.Lines, m.helpBackdrop.Lines)
		m.modal(c, t)
		return c
	}
	if m.Screen == "opening" {
		m.opening(c, t)
	} else {
		m.header(c, t)
		if m.Screen == adultContentScreen {
			m.adultContent(c, t)
		} else {
			switch m.Screen {
			case "collection":
				m.collectionScreen(c, t)
			case "select":
				m.picker(c, t)
			case "gallery":
				m.gallery(c, t)
			case "intro":
				m.portrait(c, t)
				m.intro(c, t)
			case "game":
				m.portrait(c, t)
				if m.Phase == "victory" {
					m.victoryPlaque(c, t)
				}
				if m.Phase != "victory" && m.Phase != "match" && m.Reveal > 0 {
					m.drawer(c, t)
				}
			case "settings":
				m.settings(c, t)
			}
			m.footer(c, t)
		}
	}
	if m.Modal != "" {
		m.modal(c, t)
	}
	return c
}
func (m *Model) opening(c *Canvas, t Theme) {
	a := m.Assets.Intro
	if a == nil {
		c.center(8, paint("M I D N I G H T   R O Y A L E", t.Accent, t.BG, true))
		c.center(11, paint("HEADS-UP FIVE-CARD DRAW", t.Text, t.BG, true))
		c.center(14, paint("Press Enter or Space to take your seat", t.Hot, t.BG, true))
		m.helpControl(c, t, m.OpeningFocus == 1)
		return
	}
	x := (m.W - a.Width) / 2
	y := 2 + max(0, (m.H-4-len(a.Rows))/2)
	m.openingFrame(c, x, y, a.Width, len(a.Rows), t)
	for i, row := range a.Rows {
		c.put(x, y+i, row)
	}
	m.openingSparkles(c, x, y, t)
	// The "ENTER THE NIGHT" line is baked into the logo (row 28). Make it a real
	// focusable control without redrawing the logo: a hit region over the exact
	// text, plus an inverse highlight when it holds keyboard focus.
	const enterRow = 28
	label := "[ ENTER THE NIGHT ]"
	ex := x + (a.Width-ansi.StringWidth(label))/2
	ey := y + enterRow
	c.hit("activate", ex, ey, ansi.StringWidth(label), 1)
	if m.OpeningFocus == 0 {
		c.put(ex, ey, paint(label, t.BG, t.Accent, true))
	}
	m.helpControl(c, t, m.OpeningFocus == 1)
}

func (m *Model) header(c *Canvas, t Theme) {
	c.put(0, 0, paint(" MIDNIGHT ROYALE", t.Accent, t.Panel, true))
	if m.Screen == "game" && m.Phase != "victory" {
		label := "Menu"
		x := (m.W - ansi.StringWidth(" "+label+" ")) / 2
		c.button(x, 0, label, "menu", m.HeaderMenuFocus, t)
	}
	if ctx := m.headerContext(); ctx != "" {
		right := ctx + "  "
		c.put(m.W-ansi.StringWidth(right), 0, paint(right, t.Muted, t.BG, false))
	}
	rule := "─"
	if m.Config.ASCII {
		rule = "-"
	}
	c.put(0, 1, paint(strings.Repeat(rule, m.W), t.Accent, t.BG, false))
}
func (m *Model) footer(c *Canvas, t Theme) {
	y := m.H - 1
	// Right-aligned controls: Space toggle during play, plus the shared F1 Help.
	if m.Screen == "game" && m.Phase != "victory" {
		space := "Space: Show cards"
		if m.DrawerOpen {
			space = "Space: Hide cards"
		}
		x := m.W - 2
		x -= ansi.StringWidth(" F1: Help ")
		c.button(x, y, "F1: Help", "help", false, t)
		x -= 2
		x -= ansi.StringWidth(" " + space + " ")
		c.button(x, y, space, "drawer", false, t)
	} else {
		m.helpControl(c, t, false)
	}
	// Left-aligned live status; the DEV hint takes priority over status.
	if m.Config.Dev {
		c.put(0, y, paint(" DEV  F3: Screen  F4: State  F5: Character  F6: Image  F12: Inspector  Q: Quit", t.Muted, t.BG, false))
	} else if m.Screen == "game" {
		if m.Phase == "victory" {
			c.put(0, y, paint(" You won! · "+m.character().Name+" defeated", t.Text, t.Panel, false))
		} else if m.Game != nil {
			c.put(0, y, m.realGameSummary(t))
		} else {
			c.put(0, y, paint(" You 975 · Pot 50 · "+m.character().Name+" 975", t.Text, t.Panel, false))
		}
	} else if m.Screen == "collection" {
		c.put(0, y, paint("← → choose collection · Enter open · Esc back", t.Muted, t.BG, false))
	} else if m.Screen == "select" {
		text := " ← → choose · ↓ actions · Tab next · Enter/Space open · Esc back"
		c.put(0, y, paint(text, t.Muted, t.BG, false))
	}
}

// resizeHint renders a compact, welcoming message when the terminal is
// below the minimum 100x35. The whole block is centered horizontally and
// vertically; on extremely small windows it collapses to a minimal
// instruction instead of clipping. It recenters on every resize event.
func (m *Model) resizeHint(c *Canvas, t Theme) {
	w, h := m.W, m.H
	type line struct {
		text  string
		color string
		bold  bool
	}
	var lines []line
	if w < 30 || h < 10 {
		lines = []line{
			{"MIDNIGHT ROYALE", t.Accent, true},
			{"Resize the terminal to continue", t.Text, false},
			{fmt.Sprintf("Current: %d × %d · Q: Quit", w, h), t.Muted, false},
		}
	} else {
		lines = []line{
			{"MIDNIGHT ROYALE", t.Accent, true},
			{m.sym("↔", "<->") + "  A little more room  " + m.sym("↕", "|"), t.Hot, true},
			{"Drag the window edges or maximize it to enter the night.", t.Text, false},
			{"Minimum: 100 columns × 35 rows", t.Muted, false},
			{fmt.Sprintf("Current: %d × %d", w, h), t.Muted, false},
			{"Q: Quit", t.Hot, true},
		}
	}
	// Preferred content width ~54 columns, constrained by the viewport.
	contentW := min(54, max(20, w-6))
	var rendered []string
	for _, ln := range lines {
		for _, part := range strings.Split(ansi.Wrap(ln.text, contentW, ""), "\n") {
			rendered = append(rendered, paint(part, ln.color, t.BG, ln.bold))
		}
	}
	startY := max(0, (h-len(rendered))/2)
	for i, r := range rendered {
		x := max(0, (w-ansi.StringWidth(r))/2)
		c.put(x, startY+i, r)
	}
}

func (m *Model) portrait(c *Canvas, t Theme) {
	g := m.geometry()
	a := m.Assets.Art[artKey(m.character().ID, m.Stage)]
	if a == nil {
		c.text(3, 5, "Artwork unavailable. Check the asset bundle; no substitute is displayed.", t)
		return
	}
	m.paintPortrait(c, t, a, m.Scroll, g)
}

// paintPortrait is the shared, read-only renderer used by gameplay and the
// Gallery. Callers own their cursor/scroll state and availability decision.
func (m *Model) paintPortrait(c *Canvas, t Theme, a *art.Art, scroll int, g Geometry) {
	for row := 0; row < g.ArtH && row+scroll < len(a.Rows); row++ {
		if row+scroll >= 0 && row+scroll < len(a.Rows) {
			c.put(g.ArtX, g.ArtY+row, a.Rows[row+scroll])
		}
	}
	contentH := len(a.Rows)
	thumb := max(1, g.ArtH*g.ArtH/max(1, contentH))
	maxScroll := max(0, contentH-g.ArtH)
	top := 0
	if maxScroll > 0 {
		top = scroll * max(0, g.ArtH-thumb) / maxScroll
	}
	for row := 0; row < g.ArtH; row++ {
		s := m.sym("│", "|")
		color := t.Muted
		if row >= top && row < top+thumb {
			s = m.sym("┃", "#")
			color = t.Accent
		}
		c.put(m.W-2, g.ArtY+row, paint(s, color, t.BG, false))
	}
}
func (m *Model) drawer(c *Canvas, t Theme) {
	if m.Showdown != nil {
		m.showdownDrawer(c, t)
		return
	}
	g := m.geometry()
	d := newCanvas(m.W-2, g.DrawerH, t.Panel)
	d.HoverID = m.HoverID
	pt := t
	pt.BG = t.Panel
	d.put(0, 0, paint(strings.Repeat(m.sym("─", "-"), d.W), t.Accent, t.Panel, false))
	// Row 1–2: contextual instruction. A settled hand renders a prominent
	// banner with its explanation (fold, winning hand) directly below it.
	if text, color, ok := m.settledOutcome(t); ok {
		d.put(0, 1, m.resultBanner(text, color, d.W, t))
		d.put(3, 2, paint(m.phaseText(), t.Accent, t.Panel, true))
	} else {
		d.put(3, 1, paint(m.phaseText(), t.Accent, t.Panel, true))
	}
	// Row 2 is blank in the normal case; a result banner uses it for the explanation.
	cw := 13
	gap := 3
	start := (d.W - (cw*5 + gap*4)) / 2
	for i := 0; i < 5; i++ {
		x := start + i*(cw+gap)
		card := m.card(i, cw, t)
		d.put(x, 3, card)
		if m.Phase == "new" || m.Phase == "hold" {
			d.hit(fmt.Sprintf("card:%d", i), x, 3, cw, 7)
		}
		label, color := m.cardLabel(i, t)
		if i == m.SelectedCard && m.Focus == "cards" {
			label = m.sym("▸ ", "> ") + label
			color = t.Accent
		}
		d.put(x, 10, paint(fmt.Sprintf("%-*s", cw, label), color, t.Panel, true))
	}
	actionY := 12
	actions := m.actions()
	x := m.betControls(d, d.W-3, actionY, t)
	for i := len(actions) - 1; i >= 0; i-- {
		label := actions[i]
		w := ansi.StringWidth(label) + 2
		x -= w
		d.button(x, actionY, label, fmt.Sprintf("action:%d", i), m.Focus == "actions" && m.Action == i, pt)
		x -= 2
	}
	m.blitDrawer(c, d, g)
}
func (m *Model) card(i, w int, t Theme) string {
	if m.cardFaceDown(i) {
		return m.cardBack(w, m.DrawerOpen && m.Focus == "cards" && i == m.SelectedCard, t)
	}
	ranks := []string{"A", "K", "Q", "J", "9"}
	suitIDs := [5]game.Suit{game.Spades, game.Hearts, game.Clubs, game.Diamonds, game.Spades}
	suits := []string{m.sym("♠", "S"), m.sym("♥", "H"), m.sym("♣", "C"), m.sym("♦", "D"), m.sym("♠", "S")}
	if m.Game != nil {
		card := m.Game.Hand(0)[i]
		ranks[i] = "--23456789TJQKA"[card.Rank : card.Rank+1]
		suitIDs[i] = card.Suit
		suits[i] = []string{m.sym("♣", "C"), m.sym("♦", "D"), m.sym("♥", "H"), m.sym("♠", "S")}[card.Suit]
	}
	if m.Phase == "win" && m.Game == nil {
		ranks = []string{"A", "A", "A", "K", "K"}
	}
	bg := t.Card
	ink := suitInk(suitIDs[i], t)
	border := t.Muted
	focus := m.DrawerOpen && m.Focus == "cards" && i == m.SelectedCard
	if focus {
		border = t.Accent
	} else if m.Replaced[i] {
		border = t.Good
	}
	inner := []string{
		fmt.Sprintf("%-*s", w-2, ranks[i]+" "+suits[i]),
		strings.Repeat(" ", w-2),
		strings.Repeat(" ", (w-3)/2) + suits[i] + strings.Repeat(" ", (w-2)-(w-3)/2-1),
		strings.Repeat(" ", w-2),
		fmt.Sprintf("%*s", w-2, suits[i]+" "+ranks[i]),
	}
	b := lipgloss.RoundedBorder()
	if m.Config.ASCII {
		b = lipgloss.ASCIIBorder()
	}
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(ink)).Background(lipgloss.Color(bg)).Border(b).BorderForeground(lipgloss.Color(border)).BorderBackground(lipgloss.Color(bg)).Bold(focus || m.Replaced[i])
	return s.Render(strings.Join(inner, "\n"))
}

// cardFaceDown reports whether slot i should render as a face-down card back:
// either it is selected for discard during the draw, or it is a freshly dealt
// replacement still waiting its turn in the reveal sequence.
func (m *Model) cardFaceDown(i int) bool {
	if m.Game == nil {
		return false
	}
	if m.dealReveal && i >= m.dealRevealed {
		return true
	}
	if m.Game.Phase() == game.Drawing && m.Game.Actor() == game.Human && !m.Held[i] {
		return true
	}
	if m.Replaced[i] && m.revealPending {
		for step, slot := range m.revealOrder {
			if slot == i && step >= m.revealStep {
				return true
			}
		}
	}
	return false
}

// cardBack renders a recognizable face-down card with a hatch pattern and a
// centered pip, so a discard selection is unmistakable at a glance.
func (m *Model) cardBack(w int, focused bool, t Theme) string {
	border := t.Muted
	if focused {
		border = t.Accent
	}
	bg, ink := "#141127", "#5C5875"
	pattern := m.sym("▚", "#")
	pip := m.sym("◆", "o")
	width := w - 2
	fill := strings.Repeat(pattern, width)
	half := max(0, (width-1)/2)
	center := strings.Repeat(pattern, half) + pip + strings.Repeat(pattern, max(0, width-half-1))
	inner := []string{fill, fill, center, fill, fill}
	b := lipgloss.RoundedBorder()
	if m.Config.ASCII {
		b = lipgloss.ASCIIBorder()
	}
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(ink)).Background(lipgloss.Color(bg)).Border(b).BorderForeground(lipgloss.Color(border)).BorderBackground(lipgloss.Color(bg))
	return s.Render(strings.Join(inner, "\n"))
}

// inDrawPhase reports whether the human is currently choosing cards to discard.
func (m *Model) inDrawPhase() bool {
	if m.Game != nil {
		return m.Game.Phase() == game.Drawing && m.Game.Actor() == game.Human
	}
	return m.Phase == "new" || m.Phase == "hold"
}

// cardLabel returns the fixed label and color beneath a card. NEW marks a
// replacement card, DISCARD a face-down selection, KEEP the default retention.
func (m *Model) cardLabel(i int, t Theme) (string, string) {
	if m.Replaced[i] {
		return "NEW", t.Good
	}
	if m.inDrawPhase() {
		if m.Held[i] {
			return "KEEP", t.Muted
		}
		return "DISCARD", t.Accent
	}
	return "", t.Muted
}
func (m *Model) picker(c *Canvas, t Theme) {
	heading, sub := "Choose your opponent", "A seat is waiting for you."
	headingY, top := m.collectionBadge(c, t)
	c.center(headingY, paint(heading, t.Accent, t.BG, true))
	c.center(headingY+1, paint(sub, t.Muted, t.BG, false))
	per := m.perPage()
	page := m.selectedIndex() / per
	roster := m.roster()
	tileW, tileH, gap := 30, 20, 2
	start := (m.W - (tileW*per + gap*(per-1))) / 2
	for j := 0; j < per; j++ {
		i := page*per + j
		if i >= len(roster) {
			break
		}
		x := start + j*(tileW+gap)
		m.pickerTile(c, x, top, tileW, tileH, roster[i], i+1, i == m.selectedIndex(), i == m.selectedIndex() && m.SelectFocus == 0, t)
	}
	m.centeredSelectorFooter(c, top+tileH+1, t, m.selectButtons(), func(i int) bool { return m.SelectFocus == i+1 })
}

// collection renders the collection-selection screen: one framed artwork at a
// time, centred horizontally in the viewport, with a status marker above for
// unreleased collections and a compact navigation row just above the footer.
// The collection title lives on a nameplate overlaid on the artwork itself.
func (m *Model) collectionScreen(c *Canvas, t Theme) {
	col := m.collection()
	m.collectionStatus(c, 2, t)

	if art := m.collectionArt(col.ArtworkKey()); art != nil {
		x := (m.W - art.Width) / 2
		for i, row := range art.Rows {
			c.put(x, 3+i, row)
		}
		m.collectionFrame(c, x, 3, art.Width, len(art.Rows), t)
		m.collectionPlaque(c, x, 3, art.Width, len(art.Rows), t)
		c.hit("collection-art", x, 3, art.Width, len(art.Rows))
		m.collectionArrows(c, x, 3, art.Width, len(art.Rows), t)
	} else {
		c.center(17, paint("Artwork unavailable", t.Muted, t.BG, false))
	}
	m.collectionNav(c, m.H-2, t)
}

// collectionStatus renders the COMING SOON marker above the artwork for
// unreleased collections. Available collections show nothing here: the title
// now lives on the nameplate over the artwork.
func (m *Model) collectionStatus(c *Canvas, y int, t Theme) {
	if !m.collection().isAvailable() {
		c.center(y, paint("COMING SOON", t.Hot, t.BG, true))
		return
	}
	if hs, ok := m.collectionHighScoreLine(); ok {
		c.center(y, paint(hs, t.Muted, t.BG, false))
	}
}

// collectionFrame recolors the artwork's outer frame with the active
// collection accent. The approved ANSI artwork bakes a warm-gold frame; this
// overlay redraws the border in the accent without touching the embedded
// bytes, so the frame matches the plaque and the rest of the collection's
// chrome. For Grand Riviera (gold) it is a visual no-op.
func (m *Model) collectionFrame(c *Canvas, ax, ay, aw, ah int, t Theme) {
	gold := t.Accent
	c.put(ax, ay, paint("╭"+strings.Repeat("─", aw-2)+"╮", gold, t.BG, false))
	c.put(ax, ay+ah-1, paint("╰"+strings.Repeat("─", aw-2)+"╯", gold, t.BG, false))
	for i := 1; i < ah-1; i++ {
		c.put(ax, ay+i, paint("│", gold, t.BG, false))
		c.put(ax+aw-1, ay+i, paint("│", gold, t.BG, false))
	}
}

// collectionPlaque draws the vintage hotel nameplate overlaid on the centre of
// the framed artwork. It is terminal text (never baked into the ANSI), framed
// in the same gold as the artwork border, with a solid dark background that
// fully covers the image cells beneath it. The plaque is a fixed size derived
// from the longest title, so every collection shares identical dimensions and
// the overlay never shifts or leaves residue when switching collections.
func (m *Model) collectionPlaque(c *Canvas, ax, ay, aw, ah int, t Theme) {
	title, subtitle := m.collection().PlaqueTitle()
	const padX, padY = 2, 1
	inner := collectionPlaqueInnerWidth()
	pw := inner + 2*padX + 2
	ph := 2*padY + 2 + 2

	px := ax + (aw-pw)/2
	py := ay + (ah-ph)/2
	gold, bg := t.Accent, t.BG

	rule := func(l, m, r string) string {
		return paint(l+strings.Repeat(m, pw-2)+r, gold, bg, false)
	}
	c.put(px, py, rule("╭", "─", "╮"))
	c.put(px, py+1, rule("│", " ", "│"))
	c.put(px, py+2, plaqueTextRow(title, inner, padX, gold, bg))
	c.put(px, py+3, plaqueTextRow(subtitle, inner, padX, gold, bg))
	c.put(px, py+4, rule("│", " ", "│"))
	c.put(px, py+5, rule("╰", "─", "╯"))
}

// plaqueTextRow composes one nameplate text line: gold side borders, dark
// padding, and the text centred within the fixed inner width.
func plaqueTextRow(s string, inner, padX int, gold, bg string) string {
	left := (inner - ansi.StringWidth(s)) / 2
	right := inner - left - ansi.StringWidth(s)
	return paint("│", gold, bg, false) +
		paint(strings.Repeat(" ", padX+left), bg, bg, false) +
		paint(s, gold, bg, true) +
		paint(strings.Repeat(" ", padX+right), bg, bg, false) +
		paint("│", gold, bg, false)
}

// collectionPlaqueInnerWidth returns the fixed inner text width for the
// nameplate, sized to the longest collection title so every plaque shares the
// same width and position.
func collectionPlaqueInnerWidth() int {
	w := 0
	for _, col := range Collections() {
		title, subtitle := col.PlaqueTitle()
		w = max(w, ansi.StringWidth(title), ansi.StringWidth(subtitle))
	}
	return w
}

// collectionArrows draws the left/right collection-navigation arrows outside
// the framed artwork, vertically centred on the frame, using the same padded
// button style (and hover highlight) as the character-selection pager, so the
// click target is a comfortable button rather than a thin glyph.
func (m *Model) collectionArrows(c *Canvas, ax, ay, aw, ah int, t Theme) {
	arrowY := ay + ah/2
	c.button(ax-3, arrowY, "<", "collection-prev", false, t)
	c.button(ax+aw, arrowY, ">", "collection-next", false, t)
}

// collectionNav renders the centred n / m indicator and, only for an
// available collection, an "Enter: Open" action. The left/right navigation
// arrows now live beside the framed artwork (see collectionArrows).
func (m *Model) collectionNav(c *Canvas, y int, t Theme) {
	cols := Collections()
	idx := m.collectionIndex()
	mid := fmt.Sprintf(" %d / %d ", idx+1, len(cols))
	const gap = 2
	midW := ansi.StringWidth(mid)

	action := ""
	if m.collection().isAvailable() {
		action = "Enter: Open"
	}
	actionW := 0
	if action != "" {
		actionW = ansi.StringWidth(action) + 2
	}

	total := midW
	if action != "" {
		total += gap + actionW
	}
	x := (m.W - total) / 2

	c.put(x, y, paint(mid, t.Muted, t.BG, false))
	x += midW
	if action != "" {
		x += gap
		c.button(x, y, action, "collection-open", false, t)
	}
}

// centeredSelectorFooter lays out the selected character's description, the
// action-button row, and pagination on their own centred rows below the card
// group, sharing the card group's horizontal axis. When vertical space is tight
// the blank separators are dropped first so controls stay above the footer.
func (m *Model) centeredSelectorFooter(c *Canvas, y int, t Theme, buttons []pauseItem, focus func(int) bool) {
	contentWidth := min(m.W-8, 94)
	lines := strings.Split(ansi.Wrap(m.character().Intro, contentWidth, ""), "\n")
	descH := len(lines)
	for i, line := range lines {
		c.center(y+i, paint(line, t.Text, t.BG, false))
	}
	descGap, actionGap := 1, 1
	needed := descH + 4 // desc + gap + action + gap + pagination
	footerY := m.H - 1
	if y+needed > footerY {
		actionGap = 0
		if y+descH+3 > footerY {
			descGap = 0
		}
	}
	actionY := y + descH + descGap
	centeredButtonRow(c, actionY, buttons, focus, t)
	m.centeredPagination(c, actionY+1+actionGap, t)
}

// centeredButtonRow renders a list of buttons as one centred group with a
// consistent inter-button gap.
func centeredButtonRow(c *Canvas, y int, items []pauseItem, focus func(int) bool, t Theme) {
	const gap = 3
	total := 0
	for _, it := range items {
		total += ansi.StringWidth(" " + it.label + " ")
	}
	if n := len(items); n > 1 {
		total += (n - 1) * gap
	}
	x := (c.W - total) / 2
	for i, it := range items {
		c.button(x, y, it.label, it.id, focus(i), t)
		x += ansi.StringWidth(" "+it.label+" ") + gap
	}
}

// centeredPagination renders the < N / M > pager as one centred group.
func (m *Model) centeredPagination(c *Canvas, y int, t Theme) {
	roster := m.roster()
	sel := m.selectedIndex()
	page := sel / m.perPage()
	total := (len(roster) + m.perPage() - 1) / m.perPage()
	mid := fmt.Sprintf(" %d / %d ", page+1, total)
	const gap = 2
	prevW := ansi.StringWidth(" < ")
	nextW := ansi.StringWidth(" > ")
	totalW := prevW + gap + ansi.StringWidth(mid) + gap + nextW
	x := (m.W - totalW) / 2
	c.button(x, y, "<", "page-prev", false, t)
	x += prevW + gap
	c.put(x, y, paint(mid, t.Muted, t.BG, false))
	x += ansi.StringWidth(mid) + gap
	c.button(x, y, ">", "page-next", false, t)
}

// pickerTile draws one character tile: an enlarged face crop inside a border
// that turns the theme gold when selected. Border space is reserved identically
// on selected and unselected tiles so navigation never moves or resizes them.
func (m *Model) pickerTile(c *Canvas, x, y, tileW, tileH int, ch Character, num int, sel, active bool, t Theme) {
	border := t.Muted
	if sel {
		border = t.Accent
		if m.Screen == "shop" {
			border = "#FF4FCE"
		}
	}
	horiz := m.sym("─", "-")
	vert := m.sym("│", "|")
	c.put(x, y, paint(m.sym("╭", "+")+strings.Repeat(horiz, tileW-2)+m.sym("╮", "+"), border, t.BG, false))
	for r := 1; r < tileH-1; r++ {
		c.put(x, y+r, paint(vert, border, t.BG, false))
		c.put(x+tileW-1, y+r, paint(vert, border, t.BG, false))
	}
	c.put(x, y+tileH-1, paint(m.sym("╰", "+")+strings.Repeat(horiz, tileW-2)+m.sym("╯", "+"), border, t.BG, false))
	if ch.Completed {
		marker := " COMPLETED "
		c.put(x+tileW-ansi.StringWidth(marker)-1, y+1, paint(marker, completedGold, t.BG, true))
	}
	if face := m.Assets.Faces[ch.ID]; face != nil {
		for k, row := range face.Rows {
			c.put(x+1, y+2+k, row)
		}
	}
	nameColor := t.Text
	if active {
		nameColor = t.Accent
	} else if sel {
		nameColor = t.Muted
	}
	c.put(x+1, y+2+faceRows, paint(fmt.Sprintf("%-*s", tileW-2, fmt.Sprintf("%02d · %s", num, ch.Name)), nameColor, t.BG, sel))
	c.put(x+1, y+3+faceRows, paint(fmt.Sprintf("%-*s", tileW-2, characterBandFor(ch.ID)), t.Muted, t.BG, false))
	c.put(x+1, y+4+faceRows, paint(fmt.Sprintf("%-*s", tileW-2, ch.Tag), t.Muted, t.BG, false))
	c.hit("character:"+ch.ID, x, y, tileW, tileH)
}

// introPanel describes the layout of the character panel shared by the intro
// and detail screens. It is computed before anything is drawn so wrapped text,
// status lines and the action row can never overlap the panel border or each
// other.
type introPanel struct {
	x, y, w, h int
	lines      []string // wrapped character description
	status     []string // loading / commerce status lines
	actions    []introAction
	back       introAction
	oneRow     bool
	actionY    int
}

// introAction is one focusable control on the panel's action row.
type introAction struct {
	label string
	id    string
	focus bool
}

// introLayout measures the panel for the current screen and state.
func (m *Model) introLayout() introPanel {
	w := m.W - 8
	best := m.bestLabel()
	bestLabel := ""
	if best != "" {
		bestLabel = " " + best + " "
	}
	introW := w - 8
	if bestLabel != "" {
		introW -= ansi.StringWidth(bestLabel) + 2
	}
	lines := strings.Split(ansi.Wrap(m.character().Intro, max(1, introW), ""), "\n")

	p := introPanel{x: 4, w: w, lines: lines}
	p.actions = append(p.actions, introAction{"Start game", "activate", m.IntroFocus == 0})
	p.back = introAction{"Back", "intro-back", m.IntroFocus == 1}

	const inset, gap = 7, 3
	buttonW := func(label string) int { return ansi.StringWidth(" " + label + " ") }
	x := inset
	for i, action := range p.actions {
		if i > 0 {
			x += gap
		}
		x += buttonW(action.label)
	}
	p.oneRow = (x-inset)+gap+buttonW(p.back.label) <= w-6
	actionRows := 1
	if !p.oneRow {
		actionRows = 2
	}
	// rows: 1 pad + description + name + scenes + status + gap + actions + pad
	rows := 1 + len(lines) + 1 + 1 + len(p.status) + 1 + actionRows + 1
	p.h = max(8, rows+2)
	p.y = m.H - 1 - p.h
	p.actionY = p.y + 1 + len(lines) + 2 + len(p.status) + 1
	return p
}

func (m *Model) intro(c *Canvas, t Theme) {
	p := m.introLayout()
	w := p.w
	pt := t
	pt.BG = t.Panel
	c.panel(p.x, p.y, w, p.h, t)
	c.Hits = nil

	c.put(7, p.y+1, paint(strings.Join(p.lines, "\n"), t.Text, t.Panel, false))
	row := p.y + 1 + len(p.lines)
	if best := m.bestLabel(); best != "" {
		// The BEST marker keeps the panel's top-right interior corner; the
		// description was wrapped narrower to leave room for it.
		bestLabel := " " + best + " "
		c.put(m.W-7-ansi.StringWidth(bestLabel), p.y+1, paint(bestLabel, t.Good, t.Panel, false))
	}
	if m.Screen == "detail" {
		c.put(7, row, paint(m.character().Name+" · Difficulty: "+m.characterBand(), t.Accent, t.Panel, true))
		c.put(7, row+1, paint("Includes all 5 approved scenes · scenes reveal through normal play.", t.Muted, t.Panel, false))
		row += 2
	} else {
		m.difficultyLabel(c, 7, row, pt)
		row++
	}
	for _, line := range p.status {
		c.put(7, row, paint(line, t.Muted, t.Panel, false))
		row++
	}

	buttonW := func(label string) int { return ansi.StringWidth(" " + label + " ") }
	const inset, gap = 7, 3
	x := inset
	for _, action := range p.actions {
		c.button(x, p.actionY, action.label, action.id, action.focus, pt)
		x += buttonW(action.label) + gap
	}
	backX := p.x + w - 2 - buttonW(p.back.label)
	if p.oneRow {
		c.button(backX, p.actionY, p.back.label, p.back.id, p.back.focus, pt)
	} else {
		c.button(inset, p.actionY+1, p.back.label, p.back.id, p.back.focus, pt)
	}
	c.hit("shield", p.x, p.y, w, p.h)
}

// headerContext returns the right-hand header context: a character name for
// character-focused screens, otherwise a screen name.
func (m *Model) headerContext() string {
	switch m.Screen {
	case "select", "intro", "game":
		return m.character().Name
	case "gallery":
		return "Gallery"
	case "settings":
		return "Settings"
	}
	return ""
}

// helpControl draws the discreet, clickable F1: Help control at the standard
// bottom-right position.
func (m *Model) helpControl(c *Canvas, t Theme, focused bool) {
	c.button(m.W-ansi.StringWidth(" F1: Help ")-1, m.H-1, "F1: Help", "help", focused, t)
}

// difficultyLabel renders a fixed, non-interactive difficulty label. The
// difficulty is fixed per character and never selectable, so there are no
// arrows or keyboard instructions.
func (m *Model) difficultyLabel(c *Canvas, x, y int, t Theme) {
	prefix := "Difficulty: "
	c.put(x, y, paint(prefix, t.Text, t.Panel, false))
	c.put(x+ansi.StringWidth(prefix), y, paint(" "+m.characterBand()+" ", t.Accent, t.Panel, true))
}

// helpContextTitle names the originating screen for the Help header.
func (m *Model) helpContextTitle() string {
	switch m.Screen {
	case "opening":
		return "Welcome"
	case "collection":
		return "Collections"
	case "select":
		return "Characters"
	case "gallery":
		return "Gallery"
	case "intro":
		return "Before you play"
	case "game":
		if m.Phase == "victory" {
			return "Victory"
		}
		return "At the table"
	case "settings":
		return "Settings"
	}
	return "Help"
}

// helpContentWidth is the usable width of the Help body: the panel width
// minus the left inset and the scrollbar column, so a wrapped line can never be
// drawn over the bar or past the frame.
func (m *Model) helpContentWidth() int {
	_, _, w, _ := m.helpGeometry()
	return max(20, w-6)
}

// wrapHelpLines wraps authored help lines to the panel body width. Section
// headings are kept on one line; blank lines are preserved as section breaks.
func wrapHelpLines(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "§") {
			out = append(out, line)
			continue
		}
		for _, wrapped := range strings.Split(ansi.Wrap(line, width, ""), "\n") {
			out = append(out, wrapped)
		}
	}
	return out
}

// helpLines builds the contextual Help content. Section headings are prefixed
// with "§" and rendered distinctly; everything else is body text. The commerce
// and settings sections are build-split (help_copy_v1.go / help_copy_v2.go) so
// each build describes only the product it ships, and every line is wrapped to
// the panel body before it is drawn.
func (m *Model) helpLines() []string {
	var out []string
	switch m.Screen {
	case "opening":
		out = append(out, "§This screen", "", "Enter / Space — take your seat", "Tab — move between the action and Help", "Q — quit", "")
	case "collection":
		out = append(out, "§Collections", "", "← → — choose a collection", "Enter — open an available collection", "Esc — back to the title screen", "")
	case "select":
		out = append(out, "§Choose your opponent", "", "← → — choose a character", "↓ — focus the actions · ↑ — back to the character", "← → — move between actions (when focused)", "Enter — open a character / activate action", "Tab — move between the character and actions", "Gallery — revisit artwork earned through play", "Title screen — return to the neon intro", "PgUp / PgDn — next page · Esc — back", "")
	case "gallery":
		out = append(out, "§Gallery", "", "Only images 2–5 earned by reaching their stages appear.", "Locked slots never preview unearned artwork.", "← → — previous / next earned image", "Enter — inspect the selected image", "Tab — move to the visible Back / Overview control", "Esc — detail to overview, then back to characters", "")
	case "intro":
		out = append(out, "§Before you play", "", "Difficulty is fixed per character.", "Enter — start the game", "Esc — back to characters", "")
	case "game":
		if m.Phase == "victory" {
			out = append(out, "§Victory", "", "Esc — inspect image 5", "Enter — Close window (default) · Tab to Play again / Character selection", "Space — reopen victory actions", "")
		} else {
			out = append(out, "§At the table", "", "← → — select cards or actions", "Enter — confirm", "1–5 — toggle a card to discard", "Space — show / hide your cards", "= / - — bet amount · A — all-in", "Tab — switch focus · P — pause", "")
		}
	}
	// The commerce, settings, recovery and licence sections describe the
	// product this build ships, so they are defined per build.
	out = append(out,
		"§Navigation", "",
		"← → ↑ ↓ move · Enter confirm · Esc back · Tab focus",
		"Mouse: click buttons and cards; the wheel scrolls artwork.", "",
		"§Artwork", "",
		"↑ ↓ scroll · PgUp / PgDn faster · Home / End top and bottom", "",
		"§The hand", "",
		"Both players ante 25 chips, then receive five cards.",
		"Bet, exchange 0–5 cards, then bet again.", "",
		"§Betting", "",
		"Check, call, bet, raise to a total, or fold.",
		"Minimum bet is 50; a raise must be at least the previous raise.", "",
		"§Hand ranking", "",
		"Straight flush · Four of a kind · Full house · Flush · Straight",
		"Three of a kind · Two pair · Pair · High card",
		"A-2-3-4-5 is the low straight.", "",
		"§Progression", "",
		"Levels 1–4 are playable; image 5 is the victory reward.",
		"Bust your opponent to advance; lose and you fall back a level.",
		"Level 1 retries are free. A run always starts at image 1.",
		"Each level starts with 500 chips.", "",
		"§Opponents", "",
		"Ordered by challenge: Easy (1–4), Medium (5–8), Challenging (9–12).",
		"Later characters bluff more selectively and read bets more carefully.",
		"All cards are dealt fairly — difficulty never changes the deal.", "",
		"§Sound & motion", "",
		"Toggle sound effects and reduced motion in Settings.", "",
		"§About", "",
		"Midnight Royale · 0xNorin.app · "+EditionLabel(),
		"Version "+Version+"", "",
	)
	return wrapHelpLines(out, m.helpContentWidth())
}

// helpModal renders the scrollable contextual Help overlay.
func (m *Model) helpModal(c *Canvas, x, y, w, h int, t, pt Theme) {
	c.put(x+3, y+2, paint("Help — "+m.helpContextTitle(), t.Accent, t.Panel, true))
	lines := m.helpLines()
	viewH := m.helpViewHeight()
	maxScroll := max(0, len(lines)-viewH)
	m.HelpScroll = max(0, min(m.HelpScroll, maxScroll))
	for i := 0; i < viewH && m.HelpScroll+i < len(lines); i++ {
		line := lines[m.HelpScroll+i]
		if strings.HasPrefix(line, "§") {
			c.put(x+3, y+4+i, paint(strings.TrimPrefix(line, "§"), t.Accent, t.Panel, true))
		} else {
			c.put(x+3, y+4+i, paint(line, t.Text, t.Panel, false))
		}
	}
	if maxScroll > 0 {
		barH := max(1, viewH*viewH/len(lines))
		top := m.HelpScroll * (viewH - barH) / maxScroll
		for i := 0; i < viewH; i++ {
			s := m.sym("│", "|")
			col := t.Muted
			if i >= top && i < top+barH {
				s = m.sym("┃", "#")
				col = t.Accent
			}
			c.put(x+w-2, y+4+i, paint(s, col, t.Panel, false))
		}
	}
	c.put(x+3, y+h-3, paint("F1 / Esc: Close   ↑ ↓ PgUp PgDn: scroll", t.Muted, t.Panel, false))
	c.button(x+w-10, y+h-3, "Close", "close", true, pt)
}
