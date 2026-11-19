package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"midnight-royale/internal/audio"
	"midnight-royale/internal/game"
)

// showdownState holds an immutable copy of a settled hand for the result
// inspection screen. The engine is authoritative; this snapshot only freezes
// what the player is looking at so no later transition can overwrite it.
type showdownState struct {
	Token      uint64
	Tie        bool
	Winner     game.Player
	HumanHand  [5]game.Card
	OppHand    [5]game.Card
	HumanScore game.Score
	OppScore   game.Score
	MatchOver  bool

	ShowMine       bool
	ShufflePending bool
	ShufflePlayed  bool
}

// dealMsg and shuffleMsg are one-shot presentation ticks. dealMsg advances the
// initial left-to-right deal; shuffleMsg fires the post-showdown shuffle beat.
type dealMsg struct{}
type shuffleMsg struct{ Token uint64 }

var rankNames = map[int]string{14: "Ace", 13: "King", 12: "Queen", 11: "Jack", 10: "Ten", 9: "Nine", 8: "Eight", 7: "Seven", 6: "Six", 5: "Five", 4: "Four", 3: "Three", 2: "Two"}
var rankPlural = map[int]string{14: "Aces", 13: "Kings", 12: "Queens", 11: "Jacks", 10: "Tens", 9: "Nines", 8: "Eights", 7: "Sevens", 6: "Sixes", 5: "Fives", 4: "Fours", 3: "Threes", 2: "Twos"}

// describeScore formats an engine-evaluated Score into readable text. It is a
// pure formatter over the authoritative evaluation, not a second evaluator.
func describeScore(s game.Score) string {
	switch s.Category {
	case game.HighCard:
		return "High card — " + rankNames[s.Tiebreak[0]]
	case game.OnePair:
		return "One pair — " + rankPlural[s.Tiebreak[0]] + ", " + rankNames[s.Tiebreak[1]] + " kicker"
	case game.TwoPair:
		return "Two pair — " + rankPlural[s.Tiebreak[0]] + " and " + rankPlural[s.Tiebreak[1]]
	case game.ThreeOfAKind:
		return "Three of a kind — " + rankPlural[s.Tiebreak[0]]
	case game.Straight:
		return "Straight — " + rankNames[s.Tiebreak[0]] + " high"
	case game.Flush:
		return "Flush — " + rankNames[s.Tiebreak[0]] + " high"
	case game.FullHouse:
		return "Full house — " + rankPlural[s.Tiebreak[0]] + " over " + rankPlural[s.Tiebreak[1]]
	case game.FourOfAKind:
		return "Four of a kind — " + rankPlural[s.Tiebreak[0]]
	case game.StraightFlush:
		return "Straight flush — " + rankNames[s.Tiebreak[0]] + " high"
	}
	return s.Category.String()
}

// sfx plays a sound effect through the optional audio engine, no-op'ing when
// audio is unavailable (tests, missing hardware, muted). sfxHook is a test seam.
// sfxListener (set via SetSFXListener) fires for every semantic effect BEFORE
// the desktop engine gate, so the hosted web transport can emit exactly-once
// SFX events even though hosted sessions force SoundEffects off and have no
// Audio engine.
func (m *Model) sfx(name string) {
	if m.sfxListener != nil {
		m.sfxListener(name)
	}
	if !m.SoundEffects {
		return
	}
	if m.sfxHook != nil {
		m.sfxHook(name)
	}
	if m.Audio != nil {
		m.Audio.Play(name)
	}
}

// SetSFXListener registers a callback invoked for every semantic sound effect
// the model triggers. It fires regardless of the desktop SoundEffects toggle,
// so the web transport receives authoritative SFX events without a desktop
// audio engine.
func (m *Model) SetSFXListener(fn func(name string)) {
	m.sfxListener = fn
}

// clearShowdown drops any active result-inspection state and stops the shuffle.
func (m *Model) clearShowdown() {
	m.Showdown = nil
	if m.Audio != nil {
		m.Audio.Stop(audio.CardShuffle)
	}
}

// beginOpponentReveal starts the deliberate anticipation pause before a genuine
// showdown. The opponent's cards stay hidden until the reveal tick fires; the
// reveal itself happens in the opponentRevealMsg handler.
func (m *Model) beginOpponentReveal() {
	m.opponentRevealPending = true
	m.opponentRevealScheduled = false
	m.opponentRevealToken = m.Game.Token()
}

// completeOpponentReveal reveals the opponent's hand immediately, skipping the
// remainder of the pause. Invoked by the reveal timer and by tests that deliver
// the reveal deterministically.
func (m *Model) completeOpponentReveal() {
	if !m.opponentRevealPending || m.Game == nil {
		return
	}
	m.opponentRevealPending = false
	m.opponentRevealScheduled = false
	m.enterShowdown(m.Game.Result())
}

// enterShowdown captures the settled hand for inspection. Fold results never
// reach here: the opponent's hidden cards stay hidden.
func (m *Model) enterShowdown(result game.HandResultState) {
	if result.Folded {
		return
	}
	m.Showdown = &showdownState{
		Token:      m.Game.Token(),
		Tie:        result.Tie,
		Winner:     result.Winner,
		HumanHand:  m.Game.Hand(game.Human),
		OppHand:    m.Game.Hand(game.Opponent),
		HumanScore: result.HumanRank,
		OppScore:   result.OpponentRank,
		MatchOver:  result.MatchOver,
	}
	m.DrawerOpen = true
	m.Reveal = m.geometry().DrawerH
	m.Focus = "actions"
	m.Action = 0
	m.sfx(audio.CardPlace)
}

// outcomeFor maps a settled hand result (tie flag + winner) to its banner
// wording and colour: human wins are green, opponent wins red, splits gold.
func (m *Model) outcomeFor(tie bool, winner game.Player, t Theme) (text, color string) {
	switch {
	case tie:
		return "Split pot", t.Accent
	case winner == game.Human:
		return "You win", t.Good
	default:
		return m.character().Name + " wins", t.Bad
	}
}

// settledOutcome reports the outcome wording and banner colour for a settled
// hand, or ok=false when no hand result is currently shown. It covers the
// showdown snapshot, real fold results, and the deterministic demo phases.
func (m *Model) settledOutcome(t Theme) (text, color string, ok bool) {
	if m.opponentRevealPending {
		return "", "", false
	}
	if m.Showdown != nil {
		text, color = m.outcomeFor(m.Showdown.Tie, m.Showdown.Winner, t)
		return text, color, true
	}
	if m.Game != nil {
		if m.Game.Phase() != game.HandResult {
			return "", "", false
		}
		r := m.Game.Result()
		text, color = m.outcomeFor(r.Tie, r.Winner, t)
		return text, color, true
	}
	switch m.Phase {
	case "win":
		return "You win", t.Good, true
	case "loss":
		return m.character().Name + " wins", t.Bad, true
	case "tie":
		return "Split pot", t.Accent, true
	}
	return "", "", false
}

// resultBanner renders the prominent, static hand-outcome banner centred within
// w cells: two solid block pairs flank the outcome text with one space between
// each block and the text, all in the outcome colour and bold. Width is
// measured ANSI-aware so the Unicode blocks and SGR codes size correctly.
func (m *Model) resultBanner(text, color string, w int, t Theme) string {
	block := m.sym("██", "##")
	line := paint(block+" "+text+" "+block, color, t.Panel, true)
	width := ansi.StringWidth(line)
	left := (w - width) / 2
	right := w - left - width
	return fill(left, t.Panel) + line + fill(right, t.Panel)
}

func (m *Model) showdownKey(k string) {
	switch k {
	case "enter":
		m.showdownActivate()
	case "tab", "shift+tab", "left", "right", "up", "down":
		m.Action = (m.Action + 1) % 2
		m.Focus = "actions"
	case "space":
		m.toggle()
	}
}

func (m *Model) showdownActivate() {
	if m.Showdown == nil {
		return
	}
	if m.Action == 0 {
		m.Showdown.ShowMine = !m.Showdown.ShowMine
		return
	}
	m.showdownContinue()
}

func (m *Model) showdownContinue() {
	if m.Showdown == nil || m.Game == nil {
		return
	}
	m.clearShowdown()
	if m.Game.Result().MatchOver {
		m.resolveLevelEnd()
		return
	}
	if err := m.nextRealHand(); err != nil {
		m.Notice = err.Error()
	}
}

func (m *Model) shuffleTick(token uint64) {
	if m.Showdown == nil || m.Showdown.Token != token || m.Showdown.MatchOver || m.Screen != "game" || m.Modal != "" {
		return
	}
	m.Showdown.ShufflePending = false
	if m.Showdown.ShufflePlayed {
		return
	}
	m.Showdown.ShufflePlayed = true
	m.sfx(audio.CardShuffle)
}

// beginDealReveal starts the initial left-to-right deal. Reduced motion reveals
// everything at once with a single slide cue.
func (m *Model) beginDealReveal() {
	if m.Game == nil {
		return
	}
	m.dealToken = m.Game.Token()
	if m.Config.ReducedMotion {
		m.dealReveal = false
		m.dealRevealed = 5
		m.sfx(audio.CardSlide)
		return
	}
	m.dealReveal = true
	m.dealRevealed = 0
}

// skipDealReveal reveals all remaining cards at once without a slide burst.
func (m *Model) skipDealReveal() {
	m.dealReveal = false
	m.dealRevealed = 5
}

func (m *Model) dealTick() {
	if !m.dealReveal || m.Screen != "game" || m.Modal != "" || m.Game == nil || m.Game.Token() != m.dealToken {
		return
	}
	m.dealRevealed++
	m.sfx(audio.CardSlide)
	if m.dealRevealed >= 5 {
		m.dealReveal = false
	}
}

// blitDrawer copies a composed drawer canvas onto the main canvas at the reveal
// offset, and remaps hit regions. Shared by the table and showdown drawers.
func (m *Model) blitDrawer(c *Canvas, d *Canvas, g Geometry) {
	reveal := min(m.Reveal, g.DrawerH)
	base := m.H - 1 - reveal
	for y := 0; y < reveal; y++ {
		c.put(1, base+y, d.Lines[y])
	}
	for _, h := range d.Hits {
		if h.Y < reveal {
			h.X++
			h.Y += base
			h.H = min(h.H, base+reveal-h.Y)
			c.Hits = append(c.Hits, h)
		}
	}
}

// showdownDrawer renders the result inspection view inside the existing card
// area: opponent hand by default, "Show my hand" toggle, and Continue.
func (m *Model) showdownDrawer(c *Canvas, t Theme) {
	g := m.geometry()
	d := newCanvas(m.W-2, g.DrawerH, t.Panel)
	d.HoverID = m.HoverID
	pt := t
	pt.BG = t.Panel
	d.put(0, 0, paint(strings.Repeat(m.sym("─", "-"), d.W), t.Accent, t.Panel, false))

	show := m.Showdown
	name := m.character().Name
	heading, headingColor := strings.ToUpper(name)+"'S HAND", t.Bad
	if show.ShowMine {
		heading, headingColor = "YOUR HAND", t.Good
	}
	d.put(3, 1, paint(heading, headingColor, t.Panel, true))

	if text, color, ok := m.settledOutcome(t); ok {
		d.put(0, 2, m.resultBanner(text, color, d.W, t))
	}

	cw, gap := 13, 3
	start := (d.W - (cw*5 + gap*4)) / 2
	hand := show.OppHand
	if show.ShowMine {
		hand = show.HumanHand
	}
	for i := 0; i < 5; i++ {
		d.put(start+i*(cw+gap), 3, m.showdownCard(hand[i], cw, t, headingColor))
	}
	d.put(3, 10, paint(name+"'s hand: "+describeScore(show.OppScore), t.Text, t.Panel, false))
	d.put(3, 11, paint("Your hand: "+describeScore(show.HumanScore), t.Text, t.Panel, false))

	actions := m.actions()
	x := d.W - 3
	for i := len(actions) - 1; i >= 0; i-- {
		label := actions[i]
		w := ansi.StringWidth(label) + 2
		x -= w
		d.button(x, 12, label, fmt.Sprintf("action:%d", i), m.Focus == "actions" && m.Action == i, pt)
		x -= 2
	}
	m.blitDrawer(c, d, g)
}

// showdownCard renders one face-up card with a distinct ownership accent frame.
func (m *Model) showdownCard(card game.Card, w int, t Theme, accent string) string {
	rank := "--23456789TJQKA"[card.Rank : card.Rank+1]
	suit := []string{m.sym("♣", "C"), m.sym("♦", "D"), m.sym("♥", "H"), m.sym("♠", "S")}[card.Suit]
	inner := []string{
		fmt.Sprintf("%-*s", w-2, rank+" "+suit),
		strings.Repeat(" ", w-2),
		strings.Repeat(" ", (w-3)/2) + suit + strings.Repeat(" ", (w-2)-(w-3)/2-1),
		strings.Repeat(" ", w-2),
		fmt.Sprintf("%*s", w-2, suit+" "+rank),
	}
	b := lipgloss.RoundedBorder()
	if m.Config.ASCII {
		b = lipgloss.ASCIIBorder()
	}
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(suitInk(card.Suit, t))).Background(lipgloss.Color(t.Card)).Border(b).BorderForeground(lipgloss.Color(accent)).BorderBackground(lipgloss.Color(t.Card))
	return s.Render(strings.Join(inner, "\n"))
}
