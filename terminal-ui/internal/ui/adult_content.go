package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	adultContentScreen   = "adult-content"
	adultContentTitle    = "ADULT CONTENT — 18+"
	adultContentCopy     = "Midnight Royale contains stylized pixel-art nudity and adult themes. It does not depict explicit sexual acts. This game is intended for adults aged 18 and over."
	adultContentContinue = "I am 18 or older — Continue"
)

// AdultContentAccess is the narrow seam between the launch UI and the local,
// independent acceptance record. It deliberately has no buyer or network
// methods.
type AdultContentAccess interface {
	Status() (bool, error)
	Accept() error
}

func (m *Model) initAdultContent(requestedScreen string) {
	m.adultContentTarget = requestedScreen
	if requestedScreen == "opening" {
		m.adultContentTarget = "collection"
	}
	if m.Config.AdultContentAccepted {
		return
	}
	m.adultContentPending = true
	if m.Config.AdultContent != nil {
		accepted, err := m.Config.AdultContent.Status()
		if err != nil {
			m.adultContentFailure = "load"
		} else if accepted {
			m.markAdultContentAccepted()
		}
	}
	if m.adultContentPending && requestedScreen != "opening" {
		m.Screen = adultContentScreen
		m.adultContentInputBlocked = false
	}
}

func (m *Model) markAdultContentAccepted() {
	m.Config.AdultContentAccepted = true
	m.adultContentPending = false
	m.adultContentFailure = ""
	m.adultContentInputBlocked = false
}

func (m *Model) showAdultContentAfterOpening() {
	if !m.adultContentPending {
		m.goScreen("collection")
		return
	}
	m.goScreen(adultContentScreen)
	// Legacy terminals cannot identify repeats or key releases. Requiring a
	// release event or navigation before activation ensures buffered/held
	// Enter and Space events from the title cannot accept this screen.
	m.adultContentInputBlocked = true
}

func (m *Model) acceptAdultContent() {
	if m.Config.AdultContent == nil {
		m.adultContentFailure = "save"
		return
	}
	if err := m.Config.AdultContent.Accept(); err != nil {
		m.adultContentFailure = "save"
		return
	}
	m.markAdultContentAccepted()
	m.enterAdultContentTarget()
}

func (m *Model) activateAdultContentPrimary() {
	if !m.sized || !m.fitsMinimum() {
		return
	}
	m.adultContentInputBlocked = false
	if m.adultContentFailure != "" {
		m.retryAdultContent()
	} else {
		m.acceptAdultContent()
	}
}

func (m *Model) retryAdultContent() {
	if m.Config.AdultContent == nil {
		m.adultContentFailure = "save"
		return
	}
	if m.adultContentFailure == "save" {
		m.acceptAdultContent()
		return
	}
	accepted, err := m.Config.AdultContent.Status()
	if err != nil {
		m.adultContentFailure = "load"
		return
	}
	if accepted {
		m.markAdultContentAccepted()
		m.enterAdultContentTarget()
		return
	}
	// The store is readable and no record exists. Return to the ordinary
	// confirmation; Retry itself never counts as acceptance.
	m.adultContentFailure = ""
	m.AdultContentFocus = 0
	m.adultContentInputBlocked = true
}

func (m *Model) enterAdultContentTarget() {
	target := m.adultContentTarget
	if target == "" || target == adultContentScreen || target == "opening" {
		target = "collection"
	}
	if target == "help" || target == "pause" {
		modal := target
		m.goScreen("game")
		if !m.Config.Demo {
			if err := m.startRealMatch(); err != nil {
				m.Notice = err.Error()
				return
			}
		}
		m.openModal(modal)
	} else {
		m.goScreen(target)
		if target == "settings" {
			m.SettingsReturnScreen = "select"
		}
		if target == "game" && !m.Config.Demo {
			if err := m.startRealMatch(); err != nil {
				m.Notice = err.Error()
			}
		}
	}
}

func (m *Model) adultContentKey(key string, repeat bool) {
	if key == "esc" {
		m.QuitRequested = true
		return
	}
	if repeat {
		return
	}
	switch key {
	case "tab", "right", "down":
		m.adultContentInputBlocked = false
		m.AdultContentFocus = (m.AdultContentFocus + 1) % 2
	case "shift+tab", "left", "up":
		m.adultContentInputBlocked = false
		m.AdultContentFocus = (m.AdultContentFocus + 1) % 2
	case "enter", "space":
		if m.adultContentInputBlocked {
			return
		}
		if m.AdultContentFocus == 1 {
			m.QuitRequested = true
		} else {
			m.activateAdultContentPrimary()
		}
	}
}

func (m *Model) adultContent(c *Canvas, t Theme) {
	panelW := min(86, m.W-12)
	panelH := 17
	x := (m.W - panelW) / 2
	y := max(3, (m.H-panelH)/2)
	c.panel(x, y, panelW, panelH, t)
	c.center(y+2, paint(adultContentTitle, t.Accent, t.Panel, true))

	body := adultContentCopy
	if m.adultContentFailure != "" {
		operation := "loaded"
		if m.adultContentFailure == "save" {
			operation = "saved"
		}
		body = fmt.Sprintf("Acceptance could not be %s safely. Adult content remains locked. Check access to the application user-data folder, then retry or exit.", operation)
	}
	lines := strings.Split(ansi.Wrap(body, panelW-8, ""), "\n")
	for i, line := range lines {
		c.put(x+4, y+5+i, paint(line, t.Text, t.Panel, false))
	}

	primary := adultContentContinue
	if m.adultContentFailure != "" {
		primary = "Retry"
	}
	primaryW := ansi.StringWidth(primary) + 2
	exitW := ansi.StringWidth("Exit") + 2
	gap := 4
	actionsX := x + (panelW-primaryW-gap-exitW)/2
	actionsY := y + panelH - 5
	c.button(actionsX, actionsY, primary, "adult-content-primary", m.AdultContentFocus == 0, t)
	c.button(actionsX+primaryW+gap, actionsY, "Exit", "adult-content-exit", m.AdultContentFocus == 1, t)
	c.center(y+panelH-2, paint("Tab / arrows: Choose · Enter: Confirm · Esc: Exit", t.Muted, t.Panel, false))
}
