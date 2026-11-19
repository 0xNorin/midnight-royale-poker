package ui

import (
	tea "charm.land/bubbletea/v2"
	"midnight-royale/internal/audio"
	"strings"
	"time"
)

const doubleClickInterval = 350 * time.Millisecond

func (m *Model) clearPointer() {
	m.hoverHits = m.hoverHits[:0]
	m.HoverID = ""
	m.lastCardClick = ""
	m.lastCardClickAt = time.Time{}
}

// cacheHoverHits runs only when a frame is rendered, never on mouse movement.
// Keep only hoverable controls, so a lookup cannot process artwork or trigger IO.
func (m *Model) cacheHoverHits(c *Canvas) {
	m.hoverHits = m.hoverHits[:0]
	if !m.hoverEnabled() {
		return
	}
	for _, h := range c.Hits {
		if m.hoverableHit(h.ID) {
			m.hoverHits = append(m.hoverHits, h)
		}
	}
}

// hoverEnabled reports whether pointer hover applies on the current screen or
// overlay: character selectors, the shop, gameplay actions, the commerce
// details/checkout/purchases forms, and the pause menu.
func (m *Model) hoverEnabled() bool {
	if !m.fitsMinimum() {
		return false
	}
	if m.Modal == "pause" || m.Modal == "quit" || m.Modal == "quittomenu" {
		return true
	}
	if m.Modal != "" {
		return false
	}
	switch m.Screen {
	case "select", "shop", "game", "detail", "checkout", "purchases":
		return true
	}
	return false
}

// hoverableHit reports whether a rendered hit is a hoverable control on the
// current screen. Character tiles, the portrait scroll shield, and gameplay
// cards are click targets without a hover style, so they are excluded. Every
// commerce control is drawn with the shared button helper and is hoverable.
func (m *Model) hoverableHit(id string) bool {
	if id == "" {
		return false
	}
	switch {
	case m.Modal == "pause":
		return isPauseItemID(id)
	case m.Modal == "quit":
		return id == "quit-yes" || id == "quit-no"
	case m.Modal == "quittomenu":
		return id == "quittomenu-yes" || id == "quittomenu-no"
	case m.Screen == "game":
		return isGameplayHoverID(id)
	case m.Screen == "select", m.Screen == "shop":
		return !strings.HasPrefix(id, "character:")
	case m.Screen == "detail", m.Screen == "checkout", m.Screen == "purchases":
		return id != "shield"
	}
	return false
}

func isPauseItemID(id string) bool {
	for _, item := range pauseItems() {
		if item.id == id {
			return true
		}
	}
	return false
}

func isGameplayHoverID(id string) bool {
	if id == "bet-plus" || id == "bet-minus" || id == "menu" {
		return true
	}
	return strings.HasPrefix(id, "action:")
}

func (m *Model) hoverTarget(p tea.Mouse) string {
	if !m.hoverEnabled() {
		return ""
	}
	for _, h := range m.hoverHits {
		if h.contains(p.X, p.Y) {
			return h.ID
		}
	}
	return ""
}
func (m *Model) mouseMotion(p tea.Mouse) {
	if target := m.hoverTarget(p); target != m.HoverID {
		m.HoverID = target
	}
}

// FilterUnchangedHover discards redundant motion before Bubble Tea's Update,
// commands and View phases. Clicks, releases, wheel and keyboard pass unchanged.
func FilterUnchangedHover(model tea.Model, msg tea.Msg) tea.Msg {
	if motion, ok := msg.(tea.MouseMotionMsg); ok {
		if m, ok := model.(*Model); ok && m.hoverTarget(motion.Mouse()) == m.HoverID {
			return nil
		}
	}
	return msg
}

func (m *Model) clickHit(id string, now time.Time) {
	if now.Before(m.mouseBlockedUntil) {
		return
	}
	defer m.inputFeedback(id, true)()
	if (m.Screen == "select" || m.Screen == "shop") && m.Modal == "" && strings.HasPrefix(id, "character:") {
		double := id == m.lastCardClick && !m.lastCardClickAt.IsZero() && now.Sub(m.lastCardClickAt) >= 0 && now.Sub(m.lastCardClickAt) <= doubleClickInterval
		m.hit(id) // existing selection path; the full visible card is already a hit
		if double {
			m.clearPointer()
			m.key("enter") // same focused-card action and product/access handling as Enter
			// Consume any queued third press as well as releases (which never activate).
			m.mouseBlockedUntil = now.Add(doubleClickInterval)
		} else {
			m.lastCardClick, m.lastCardClickAt = id, now
		}
		return
	}
	if m.Screen == "collection" && m.Modal == "" && id == "collection-art" {
		double := id == m.lastCardClick && !m.lastCardClickAt.IsZero() && now.Sub(m.lastCardClickAt) >= 0 && now.Sub(m.lastCardClickAt) <= doubleClickInterval
		if double {
			m.clearPointer()
			m.key("enter") // same open action as Enter (collectionKey)
			// Consume any queued third press as well as releases (which never activate).
			m.mouseBlockedUntil = now.Add(doubleClickInterval)
		} else {
			m.lastCardClick, m.lastCardClickAt = id, now
		}
		return
	}
	m.clearPointer()
	m.hit(id)
}

func (m *Model) scrollPortrait(delta int) {
	m.Scroll += delta
	m.clamp()
}

// Only item/focus/action changes count as UI navigation. Image scrolling,
// hover, background progress messages and redraws never enter this comparison.
type feedbackState struct {
	showMine, dealReveal                bool
	screen, modal, phase, focus, notice string
	selected                            [11]int
	flags                               [8]bool
	held                                [5]bool
	token                               uint64
}

func (m *Model) feedbackState() feedbackState {
	s := feedbackState{
		screen: m.Screen, modal: m.Modal, phase: m.Phase, focus: m.Focus,
		notice:   m.Notice,
		selected: [11]int{m.Selected, m.SelectFocus, m.OpeningFocus, m.GalleryCursor, m.GalleryFocus, m.Setting, m.ModalFocus, m.AdultContentFocus, m.SelectedCard, m.Action, m.ScrollStep},
		flags:    [8]bool{m.GalleryDetail, m.Config.ReducedMotion, m.Config.ASCII, m.SoundEffects, m.DrawerOpen, m.QuitRequested, m.clipboardCopy != "", m.DemoOptionsOpen},
		held:     m.Held,
	}
	s.dealReveal = m.dealReveal
	if m.Showdown != nil {
		s.showMine = m.Showdown.ShowMine
	}
	if m.Game != nil {
		s.token = m.Game.Token()
	}
	return s
}

// Nested dispatch (mouse -> hit -> key) produces one blip, never one per helper.
func (m *Model) inputFeedback(input string, mouse bool) func() {
	before := m.feedbackState()
	m.feedbackDepth++
	return func() {
		m.feedbackDepth--
		if m.feedbackDepth != 0 || !m.fitsMinimum() {
			return
		}
		changed := before != m.feedbackState()
		if mouse {
			// Selecting an already selected card or clicking the modal shield is silent.
			button := input != "shield" && !strings.HasPrefix(input, "character:") && !strings.HasPrefix(input, "card:") && !strings.HasPrefix(input, "gallery-item:")
			if input != "shield" && (changed || button) {
				m.sfx(audio.Blip)
			}
			return
		}
		switch input {
		case "up", "down", "left", "right", "tab", "shift+tab", "pgup", "pgdown", "[", "]", "enter", "space", "esc", "s", "o", "f1", "?", "p", "c", "r":
			if changed {
				m.sfx(audio.Blip)
			}
		}
	}
}

func (m *Model) startVictorySound() {
	if m.victorySoundOn {
		return
	}
	m.victorySoundOn = true
	// Only the title has background music today. Stop any still-fading title
	// theme; gameplay's appropriate background is silence. enterTitle restores it.
	m.stopIntroMusic()
	m.sfx(audio.Win)
}
func (m *Model) stopVictorySound() {
	if !m.victorySoundOn {
		return
	}
	m.victorySoundOn = false
	if m.Audio != nil {
		m.Audio.Stop(audio.Win)
	}
	if m.musicHook != nil {
		m.musicHook("stop-victory")
	}
}
