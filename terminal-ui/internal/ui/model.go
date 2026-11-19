// Package ui owns input, layout and deterministic presentation state.
package ui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"midnight-royale/internal/art"
	"midnight-royale/internal/audio"
	"midnight-royale/internal/game"
	"midnight-royale/internal/opponent"
	"midnight-royale/internal/progress"
	"midnight-royale/internal/run"
	"time"
)

const introMusicFade = 3500 * time.Millisecond

// Logf is an optional diagnostic sink wired by the launcher when debug
// logging is enabled. It must never receive secret values (codes, credentials,
// keys, clipboard or decrypted artwork bytes).
var Logf func(format string, args ...any)

type Assets struct {
	Art         map[string]*art.Art
	Faces       map[string]*art.Art
	Intro       *art.Art
	Collections map[string]*art.Art
}

// Clone returns a per-session shallow copy. The artwork maps are copied so a
// hosted session can materialize its own paid content into a private map
// without leaking it into other sessions that share the source bundle. The
// *art.Art pointers themselves (the free bundle) are shared and read-only.
func (a *Assets) Clone() *Assets {
	if a == nil {
		return &Assets{}
	}
	c := &Assets{Intro: a.Intro}
	if a.Art != nil {
		c.Art = make(map[string]*art.Art, len(a.Art))
		for k, v := range a.Art {
			c.Art[k] = v
		}
	}
	if a.Faces != nil {
		c.Faces = make(map[string]*art.Art, len(a.Faces))
		for k, v := range a.Faces {
			c.Faces[k] = v
		}
	}
	if a.Collections != nil {
		c.Collections = make(map[string]*art.Art, len(a.Collections))
		for k, v := range a.Collections {
			c.Collections[k] = v
		}
	}
	return c
}

type Config struct {
	Screen, Character, Phase                     string
	Collection                                   string
	Stage                                        int
	ReducedMotion, ASCII, Dev, Demo, DisableSave bool
	AdultContentAccepted                         bool
	DeterministicSeed                            int64
	SavePath                                     string
	Installed                                    map[string]bool
	AdultContent                                 AdultContentAccess
	// Licence is the verified product-licence state for this launch. The zero
	// value means "no licence"; the 1.2.x build ignores it.
	// LicencePath is where this installation stores its .mrlic file.
	// HostedAccount is the transport-agnostic account/recovery capability for a
	// hosted (SSH) session. It is nil for the desktop build. The UI depends on
	// this narrow interface, never on Wish or backend types.
	// HostedCommerce is the transport-agnostic linked-buyer purchase capability
	// for a hosted (SSH) session. It is nil for the desktop build. The UI
	// depends on this narrow interface, never on Wish or backend types.
}

type Model struct {
	Config               Config
	Assets               *Assets
	W, H, Scroll         int
	DrawerOpen           bool
	Reveal               int
	Focus                string
	SelectedCard         int
	Held                 [5]bool
	Replaced             [5]bool
	Modal                string
	ModalFocus           int
	SettingsReturnScreen string
	// LicenceError is the reason the last activation failed. It is never a
	// place for the typed serial: that value is cleared the moment the answer
	// arrives.
	// PurchasedSerial is the serial this session's purchase issued, shown once
	// so the player can save it. It is never persisted or logged.
	// ShownLicenceSerial holds the serial the player explicitly asked to see on
	// the licence screen. It is cleared on hide, on leaving the screen and on
	// open; it is never persisted or logged.
	// InstallContinue is the screen to open when an automatic download
	// succeeds ("" stays on the progress screen).
	SettingsReturnModal                    string
	ScrollStep                             int
	Characters                             []Character
	Selected, Stage                        int
	SelectedCollection                     CollectionID
	Screen, Phase                          string
	Action, Bet, Hand, Difficulty, Setting int
	BetAmount                              int
	LevelTransition                        run.Transition
	HeaderMenuFocus                        bool
	Notice                                 string
	animationPending, opponentPending      bool
	clipboardCopy                          string
	copyToken                              uint64
	copyFeedbackErr                        error
	copyFeedbackUntil                      time.Time
	copyFeedbackPending                    bool
	copyFeedbackID                         uint64
	countdownPending                       bool
	clipboardPasteRequested                bool
	clipboardPastePending                  bool
	QuitRequested                          bool
	ExitScroll                             int
	opponentEpoch                          int
	revealPending                          bool
	revealOrder                            []int
	revealStep                             int
	opponentRevealPending                  bool
	opponentRevealScheduled                bool
	opponentRevealToken                    uint64
	Game                                   *game.Game
	Bot                                    *opponent.Bot
	Save                                   progress.Data
	Store                                  *progress.Store
	opponentCancel                         context.CancelFunc
	progressEligible                       bool

	// Royal Score accounting for the current character run. Deductions
	// accumulate across level transitions and reset only when a new run
	// begins. thisRunScore and the two flags are captured at victory for the
	// dialog and plaque.
	runLostHands           int
	runLostStages          int
	thisRunScore           int
	runNewBest             bool
	runCompletedCollection bool

	sparkleActive, sparklePending bool
	sparkleEpoch                  uint64
	sparkleStep                   int

	hoverHits          []Hit // small control-only cache from the displayed frame
	HoverID            string
	lastCardClick      string
	lastCardClickAt    time.Time
	mouseBlockedUntil  time.Time
	feedbackDepth      int
	victorySoundOn     bool
	freshSoundPending  bool
	betFeedbackUntil   [2]time.Time
	betFeedbackPending bool
	betFeedbackID      uint64

	Audio        *audio.Engine
	SoundEffects bool
	titleMusicOn bool
	sized        bool // set once the real terminal size has arrived
	Showdown     *showdownState
	dealReveal   bool
	dealRevealed int
	dealToken    uint64
	sfxHook      func(string)
	sfxListener  func(string)
	musicHook    func(string)

	// HelpScroll is the scroll offset of the shared Help overlay; OpeningFocus
	// is the title-screen focus index (0 = primary action, 1 = F1 Help).
	helpBackdrop        *Canvas
	helpLeft, helpRight []string
	helpCloseHit        Hit
	helpCloseUntil      time.Time
	HelpScroll          int
	OpeningFocus        int
	SelectFocus         int
	SelectActionFocus   int
	DemoOptionsOpen     bool
	AdultContentFocus   int
	GalleryCursor       int
	GalleryScroll       int
	GalleryFocus        int
	GalleryDetail       bool

	adultContentTarget       string
	adultContentPending      bool
	adultContentFailure      string
	adultContentInputBlocked bool

	// FullGameFocus is the focused action on the Full Game screen.
	// ShopOrigin is the screen the Full Game screen was opened from, so Back
	// returns to the roster or to the character detail that led there.
	// LicenceOrigin is the screen the licence screen was opened from.
	// CommerceIssueKind classifies the last commerce failure (offline, catalogue
	// mismatch or checkout failure) so screens never describe one as another.
	// CommerceIssueText is the classified, player-facing cause of that
	// failure. It survives screen changes and banner updates, so going Back
	// from a failed checkout still shows why the purchase failed.
	IntroFocus int

	// HostedAccountMode is the hosted account/recovery screen stage
	// ("", overview, entry, delivery, done, failed). The remaining fields hold
	// in-memory-only recovery state: A is masked on render, B is held only
	// during delivery and never persisted.

	// Hosted account-creation state (S5), in-memory only and per session.
	HostedAccountCreateRequested bool
	HostedAccountCreatePending   bool

	// Browser-link code state (S19C), in-memory only and per session. The code
	// is shown once and never persisted or logged.
	HostedLinkCode      string
	HostedLinkExpires   string
	HostedLinkError     string
	HostedLinkRequested bool
	HostedLinkPending   bool

	// Hosted purchase state (S4), in-memory only and per session.
	HostedBuyProductID    string
	HostedCheckoutLink    string
	HostedPayReference    string
	HostedCheckoutState   string // "" | creating | waiting | refreshing | paid | expired | failed
	HostedCommerceError   string
	HostedCheckoutFocus   int
	hostedCommerceAction  string
	hostedCommercePending bool
	hostedCommerceEpoch   int
	hostedCommerceCancel  context.CancelFunc
	hostedPollPending     bool
	hostedPollTimerNeeded bool
	hostedPollFailures    int
}
type Geometry struct{ ArtY, ArtH, ArtX, DrawerY, DrawerH int }
type frameMsg struct{}
type revealMsg struct{}
type checkoutCountdownMsg struct{}
type downloadTickMsg struct{}
type copyFeedbackMsg struct{ ID uint64 }

const copyFeedbackDuration = 1200 * time.Millisecond

// opponentRevealMsg completes a pending opponent-card reveal. The token pins it
// to the exact showdown that scheduled it.
type opponentRevealMsg struct{ Token uint64 }

// opponentRevealDelay is the deliberate anticipation pause before the
// opponent's hand is revealed at a genuine showdown.
const opponentRevealDelay = 1500 * time.Millisecond
const downloadTickInterval = 50 * time.Millisecond

type opponentMsg struct {
	Epoch  int
	Token  uint64
	Action game.Action
	Amount int
	Draw   []int
	IsDraw bool
	Demo   bool
	Err    error
}

func artKey(name string, stage int) string { return fmt.Sprintf("%s%02d", name, stage) }
func New(c Config, a *Assets) *Model {
	if c.Screen == "" {
		c.Screen = "opening"
	}
	if c.Phase == "" {
		c.Phase = "new"
	}
	if c.Stage == 0 {
		c.Stage = 1
	}
	m := &Model{Config: c, Assets: a, W: 104, H: 40, DrawerOpen: true, Focus: "cards", Held: [5]bool{true, true, true, true, true}, ScrollStep: 3, Characters: ShippedCharacters(), SelectedCollection: DefaultCollectionID, Screen: c.Screen, Phase: c.Phase, Stage: c.Stage, Bet: 50, SoundEffects: true, SelectActionFocus: 1, progressEligible: !c.Demo && c.Stage == run.MinLevel}
	if c.Collection != "" {
		for _, col := range Collections() {
			if col.ID == CollectionID(c.Collection) {
				m.SelectedCollection = col.ID
				break
			}
		}
	}
	if !c.Demo {
		for i := range m.Characters {
			m.Characters[i].Completed = false // real completion is restored by initProgress, never visual fixtures
		}
	}
	for _, c := range m.Characters {
		if c.ID == m.Config.Character {
			m.Selected = m.rosterPos(c.ID)
		}
	}
	if m.Stage < 1 || m.Stage > 5 {
		m.Stage = 1
	}
	m.initProgress()
	m.Reveal = m.geometry().DrawerH
	m.setPhase(m.Phase)
	m.initAdultContent(c.Screen)
	if m.Screen != adultContentScreen && (m.Screen == "help" || m.Screen == "pause") {
		m.openModal(m.Screen)
		m.Screen = "game"
	}
	if m.Screen != adultContentScreen && m.Screen == "game" && !c.Demo {
		if err := m.startRealMatch(); err != nil {
			m.Notice = err.Error()
		} else if m.Stage == run.MinLevel {
			m.freshSoundPending = true
		}
	}
	return m
}

// fitsMinimum reports whether the terminal meets the minimum playable size.
func (m *Model) fitsMinimum() bool { return m.W >= 100 && m.H >= 35 }

func (m *Model) geometry() Geometry {
	const drawerHeight = 14
	return Geometry{ArtY: 2, ArtH: max(0, m.H-3), ArtX: (m.W - 96) / 2, DrawerY: m.H - 1 - drawerHeight, DrawerH: drawerHeight}
}
func (m *Model) resize(w, h int) {
	m.helpBackdrop = nil
	m.helpCloseUntil = time.Time{}
	m.clearPointer()
	m.W = max(1, w)
	m.H = max(1, h)
	m.clamp()
	if m.Screen == "gallery" && m.GalleryDetail {
		m.clampGalleryScroll()
	}
	if m.DrawerOpen && !m.animationPending {
		m.Reveal = m.geometry().DrawerH
	} else {
		m.Reveal = min(m.Reveal, m.geometry().DrawerH)
	}
	m.sized = true
	m.syncTitleMusic()
}
func (m *Model) clamp() { m.Scroll = max(0, min(m.Scroll, max(0, 72-m.geometry().ArtH))) }
func (m *Model) toggle() {
	m.DrawerOpen = !m.DrawerOpen
	if m.DrawerOpen {
		m.Focus = m.phaseFocus()
	} else {
		m.Focus = "art"
	}
	if m.Config.ReducedMotion {
		if m.DrawerOpen {
			m.Reveal = m.geometry().DrawerH
		} else {
			m.Reveal = 0
		}
	}
}
func (m *Model) openModal(name string) {
	if name == "pause" && m.Modal != "pause" {
		m.sfx(audio.Pause)
	}
	m.stopSparkles()
	m.clearPointer()
	m.HeaderMenuFocus = false
	m.Modal = name
	m.ModalFocus = 0
	if name == "help" {
		m.helpBackdrop = nil
		m.helpCloseUntil = time.Time{}
		m.HelpScroll = 0
	}
}
func (m *Model) animate() {
	if m.DrawerOpen {
		m.Reveal = min(m.geometry().DrawerH, m.Reveal+4)
	} else {
		m.Reveal = max(0, m.Reveal-4)
	}
}
func (m *Model) phaseFocus() string {
	switch m.Phase {
	case "new", "hold":
		return "cards"
	case "bet":
		if m.Game != nil {
			return "actions"
		}
		return "bet"
	default:
		return "actions"
	}
}
func (m *Model) setPhase(p string) {
	if Logf != nil {
		Logf("phase: %s -> %s", m.Phase, p)
	}
	if p != "victory" {
		m.stopVictorySound()
	}
	m.cancelOpponent()
	m.Phase = p
	m.Action = 0
	if m.DrawerOpen {
		m.Focus = m.phaseFocus()
	}
	m.opponentEpoch++
	m.opponentPending = false
}
func (m *Model) goScreen(s string) {
	if Logf != nil {
		Logf("screen: %s -> %s", m.Screen, s)
	}
	if s != m.Screen {
		m.stopSparkles()
	}
	m.clearPointer()
	if s != "game" {
		m.stopVictorySound()
	}
	if m.adultContentPending && s != "opening" && s != adultContentScreen && s != exitScreen {
		s = adultContentScreen
	}
	wasOpening := m.Screen == "opening"
	if s != m.Screen {
		m.clipboardPastePending = false
	}
	m.cancelOpponent()
	// Settings records the screen that opened it, so Esc returns there.
	if s == "settings" && m.Screen != "settings" {
		m.SettingsReturnScreen = m.Screen
		m.SettingsReturnModal = m.Modal
	}
	m.Screen = s
	m.Modal = ""
	m.ModalFocus = 0
	m.HeaderMenuFocus = false
	m.Notice = ""
	m.opponentEpoch++
	m.opponentPending = false
	if wasOpening && s != "opening" {
		m.fadeIntroMusic()
	}
	if s == "game" {
		m.DrawerOpen = true
		m.Reveal = m.geometry().DrawerH
		m.Focus = m.phaseFocus()
	}
	if s == "select" {
		m.SelectFocus = 0
		if m.SelectActionFocus < 1 || m.SelectActionFocus > len(m.selectButtons()) {
			m.SelectActionFocus = 1
		}
	}
	if s == "intro" {
		m.IntroFocus = 0
	}
	if s == "gallery" {
		m.enterGallery()
	}
	if s == "settings" {
		m.Setting = 0
	}
}
func (m *Model) startIntroMusic() {
	if m.musicHook != nil {
		m.musicHook("play")
	}
	if m.Audio != nil {
		m.Audio.PlayMusic()
	}
}

func (m *Model) fadeIntroMusic() {
	m.titleMusicOn = false
	if m.musicHook != nil {
		m.musicHook("fade")
	}
	if m.Audio != nil {
		m.Audio.FadeMusic(introMusicFade)
	}
}

func (m *Model) stopIntroMusic() {
	if m.musicHook != nil {
		m.musicHook("stop")
	}
	if m.Audio != nil {
		m.Audio.StopMusic()
	}
}

// syncTitleMusic starts the intro theme exactly once per title-screen
// lifecycle: the first time the opening screen is visible at or above the
// minimum terminal size, once the real terminal size is known. Before the
// first size message the model's W/H are still the launch defaults, so the
// theme must not start then. A temporary transition below the minimum only
// changes what is rendered - it must not reset the audio lifecycle: the theme
// keeps playing on the same player and is never restarted by a resize. The
// theme is stopped only by fadeIntroMusic (leaving the intro), disabling
// audio, or StopAll on quit.
func (m *Model) syncTitleMusic() {
	if m.Screen == "opening" && m.sized && m.fitsMinimum() && !m.titleMusicOn {
		m.titleMusicOn = true
		m.startIntroMusic()
	}
}

// enterTitle returns to the neon opening/title screen, restores the default
// title focus, and (re)starts the intro theme according to the current sound
// setting. goScreen already fades the music on the way out, so repeated visits
// neither stack timers nor duplicate audio.
func (m *Model) enterTitle() {
	m.goScreen("opening")
	m.OpeningFocus = 0
	m.syncTitleMusic()
}

// introBack returns from the character-introduction screen to the
// character-selection screen for the current collection. goScreen("select")
// preserves the selected collection, character and page, so this is a pure
// back-navigation. It is the single shared action behind both the visible
// Back button and Esc on the character-intro screen, and must never open
// the quit confirmation.
func (m *Model) introBack() {
	m.goScreen("select")
}

func (m *Model) selectCharacter(i int) {
	m.clearPointer()
	n := len(m.roster())
	if n == 0 {
		return
	}
	m.Selected = ((i % n) + n) % n
	m.Stage = run.MinLevel // a new run always starts at image 1
	m.Scroll = 0
	m.progressEligible = !m.Config.Demo
}

// selectCharacterByID selects a character by its stable id, resolving the
// roster position from the active collection.
func (m *Model) selectCharacterByID(id string) {
	m.selectCharacter(m.rosterPos(id))
}

// pageDown / pageUp page the opponent grid, clamping so navigation never
// exceeds the roster's last page nor goes below the first.
func (m *Model) pageDown() {
	per := m.perPage()
	n := len(m.roster())
	if n == 0 {
		return
	}
	last := (n - 1) / per
	page := m.selectedIndex() / per
	if page >= last {
		return
	}
	m.selectCharacter((page + 1) * per)
}

func (m *Model) pageUp() {
	per := m.perPage()
	if m.selectedIndex()/per <= 0 {
		return
	}
	m.selectCharacter((m.selectedIndex()/per - 1) * per)
}

// selectKey handles spatial arrow-key navigation on the character-selection
// screen. The screen has two logical focus areas: the character card grid
// (SelectFocus == 0) and the bottom action row (SelectFocus >= 1). Arrow keys
// move within and between the areas; Tab/Shift+Tab drive the same focus state.
func (m *Model) selectKey(k string) {
	// Page Up / Page Down always page the character grid, regardless of focus.
	switch k {
	case "pgdown", "]":
		m.pageDown()
		return
	case "pgup", "[":
		m.pageUp()
		return
	}
	if m.SelectFocus > 0 {
		// Bottom action row focused.
		switch k {
		case "left":
			m.setSelectActionFocus(m.SelectFocus - 1)
		case "right":
			m.setSelectActionFocus(m.SelectFocus + 1)
		case "up":
			m.SelectFocus = 0
		case "down":
			// remain in the action row
		}
		return
	}
	// Character grid focused.
	switch k {
	case "left":
		m.selectCharacter(m.Selected - 1)
	case "right":
		m.selectCharacter(m.Selected + 1)
	case "down":
		m.setSelectActionFocus(m.SelectActionFocus)
	case "up":
		// remain in the character grid (no three-character jump)
	}
}

// setSelectActionFocus moves focus to a bottom action (1-based) and remembers
// it, clamping to the available actions so a remembered index can never exceed
// the current button set when action availability changes.
func (m *Model) setSelectActionFocus(i int) {
	buttons := m.selectButtons()
	if i < 1 {
		i = 1
	}
	if i > len(buttons) {
		i = len(buttons)
	}
	m.SelectFocus = i
	m.SelectActionFocus = i
}
func (m *Model) activate() {
	switch m.Screen {
	case "opening":
		if m.adultContentPending {
			m.goScreen(adultContentScreen)
			m.adultContentInputBlocked = false
		} else {
			m.goScreen("collection")
		}
	case "select":
		m.goScreen("intro")
	case "intro":
		if m.accessible() || m.Config.Dev {
			if !m.Config.Demo {
				m.beginRun()
				if err := m.startRealMatch(); err != nil {
					m.Notice = err.Error()
					return
				}
				if m.Stage == run.MinLevel {
					m.sfx(audio.NewGameStart)
				}
				m.goScreen("game")
				return
			}
			m.Held = [5]bool{true, true, true, true, true}
			m.Replaced = [5]bool{}
			m.SelectedCard = 0
			m.Hand = 0
			m.Bet = 40
			m.setPhase("new")
			m.goScreen("game")
		}
	case "game":
		if m.Game != nil {
			m.activateRealGame()
			return
		}
		if !m.DrawerOpen {
			return
		}
		if m.Focus == "cards" && (m.Phase == "new" || m.Phase == "hold") {
			m.toggleDiscard(m.SelectedCard)
			m.Phase = "hold"
			return
		}
		if m.Focus == "bet" {
			m.Focus = "actions"
			return
		}
		if m.Focus != "actions" {
			return
		}
		switch m.Phase {
		case "new", "hold":
			m.setPhase("bet")
		case "bet":
			if m.Action == 1 {
				m.setPhase("loss")
			} else {
				m.setPhase("opponent")
			}
		case "win":
			m.Hand++
			m.Held = [5]bool{true, true, true, true, true}
			m.Replaced = [5]bool{}
			m.setPhase("new")
		case "loss", "progress":
			m.Hand++
			m.Held = [5]bool{true, true, true, true, true}
			m.Replaced = [5]bool{}
			m.setPhase("new")
		}
	}
}
func (m *Model) finishOpponent() {
	if m.Phase != "opponent" {
		return
	}
	if m.Hand%3 == 1 {
		m.setPhase("loss")
	} else {
		m.setPhase("win")
	}
}
func (m *Model) paymentNext() {
	// Payment state is server-authoritative. Visual fixtures cannot grant
	// ownership or manufacture elapsed-time success.
}
func (m *Model) key(k string) {
	m.keyEvent(k, false)
}

func (m *Model) keyEvent(k string, repeat bool) {
	m.helpCloseUntil = time.Time{}
	defer m.inputFeedback(k, false)()
	m.clearPointer()
	if m.Screen == adultContentScreen {
		m.adultContentKey(k, repeat)
		return
	}
	if m.Screen == exitScreen {
		m.exitScreenKey(k, repeat)
		return
	}
	if m.Modal != "" {
		m.modalKey(k)
		return
	}
	if k == "f1" || k == "?" {
		m.openModal("help")
		return
	}
	if k == "f12" {
		m.openModal("developer")
		return
	}
	if m.Config.Dev {
		if m.devKey(k) {
			return
		}
	}
	if m.W < 100 || m.H < 35 {
		return
	}
	if m.Screen == "opening" {
		switch k {
		case "enter", "space":
			if m.OpeningFocus == 0 {
				m.showAdultContentAfterOpening()
			} else {
				m.openModal("help")
			}
		case "tab", "shift+tab", "left", "right", "up", "down":
			m.OpeningFocus = 1 - m.OpeningFocus
		case "esc":
			m.openQuitConfirm()
		}
		return
	}
	if m.Screen == "collection" {
		m.collectionKey(k)
		return
	}
	switch k {
	case "o":
		if m.Screen != "game" {
			m.goScreen("settings")
			return
		}
	}
	if m.Screen == "settings" {
		m.settingsKey(k)
		return
	}
	if m.Screen == "gallery" {
		m.galleryKey(k)
		return
	}
	if k == "esc" {
		switch m.Screen {
		case "game":
			if m.DrawerOpen {
				m.toggle()
			} else {
				m.openModal("pause")
			}
		case "select":
			m.goScreen("collection")
		case "intro":
			m.introBack()
		default:
			m.goScreen("select")
		}
		return
	}
	if m.Screen == "select" {
		if k == "tab" || k == "shift+tab" {
			n := len(m.selectButtons()) + 1 // character grid + buttons
			if k == "tab" {
				m.SelectFocus = (m.SelectFocus + 1) % n
			} else {
				m.SelectFocus = (m.SelectFocus + n - 1) % n
			}
			if m.SelectFocus > 0 {
				m.SelectActionFocus = m.SelectFocus
			}
			return
		}
		if k == "enter" || k == "space" {
			if m.SelectFocus == 0 {
				m.activate()
			} else {
				m.selectActivateIndex(m.SelectFocus - 1)
			}
			return
		}
	}
	if m.Screen == "select" {
		m.selectKey(k)
		return
	}
	if m.Screen == "intro" {
		switch k {
		case "left", "shift+tab", "right", "tab":
			m.IntroFocus = 1 - m.IntroFocus
		case "enter":
			if m.IntroFocus == 0 {
				m.activate()
			} else {
				m.introBack()
			}
		}
		return
	}
	if m.Screen != "game" && m.Screen != "intro" {
		return
	}
	if m.Screen == "game" && m.Phase == "victory" {
		switch k {
		case "space", "enter":
			if m.Modal == "" {
				m.openModal("victory")
			}
		case "p":
			m.openModal("pause")
		case "up":
			m.Scroll--
		case "down":
			m.Scroll++
		case "pgup":
			m.Scroll -= max(1, m.geometry().ArtH-2)
		case "pgdown":
			m.Scroll += max(1, m.geometry().ArtH-2)
		case "home":
			m.Scroll = 0
		case "end":
			m.Scroll = 72
		}
		m.clamp()
		return
	}
	if m.Screen == "game" && m.Showdown != nil {
		m.showdownKey(k)
		return
	}
	if m.Screen == "game" && m.opponentRevealPending {
		// During the reveal pause, discard advance inputs entirely: they must
		// not be buffered and replayed once the timer fires. F1/quit/pause/esc
		// are handled above and remain available.
		switch k {
		case "enter", "space":
			return
		}
	}
	if m.Screen == "game" && m.dealReveal {
		switch k {
		case "enter":
			m.skipDealReveal()
		case "space":
			m.toggle()
		}
		return
	}
	if m.Screen == "game" {
		if m.HeaderMenuFocus {
			switch k {
			case "enter", "space":
				m.HeaderMenuFocus = false
				m.openModal("pause")
				return
			case "tab", "shift+tab", "esc":
				m.HeaderMenuFocus = false
				return
			}
		}
		switch k {
		case "space":
			m.toggle()
			return
		case "p":
			m.openModal("pause")
			return
		case "enter":
			m.activate()
			return
		case "tab", "shift+tab":
			if !m.DrawerOpen {
				m.HeaderMenuFocus = !m.HeaderMenuFocus
				return
			}
			delta := 1
			if k == "shift+tab" {
				delta = -1
			}
			if m.moveBetControl(delta) {
				return
			}
			switch m.Phase {
			case "new", "hold":
				if m.Focus == "cards" {
					m.Focus = "actions"
				} else {
					m.Focus = "cards"
				}
			case "bet":
				if m.Game != nil {
					m.Focus = "actions"
				} else if m.Focus == "bet" {
					m.Focus = "actions"
				} else {
					m.Focus = "bet"
				}
			}
			return
		case "left", "right":
			if m.DrawerOpen {
				d := 1
				if k == "left" {
					d = -1
				}
				if m.moveBetControl(d) {
					return
				}
				switch m.Focus {
				case "cards":
					m.SelectedCard = (m.SelectedCard + d + 5) % 5
				case "bet":
					m.Bet = max(50, min(500, m.Bet+d*50))
				case "actions":
					if n := len(m.actions()); n > 0 {
						m.Action = (m.Action + d + n) % n
					}
				}
			}
			return
		case "up", "down", "+", "=", "-", "a":
			if m.DrawerOpen && m.Phase == "bet" && m.Game != nil && m.Game.Actor() == game.Human {
				if _, _, ok := m.betAmountRange(); ok {
					switch k {
					case "up", "+", "=":
						m.adjustBetAmount(1)
					case "down", "-":
						m.adjustBetAmount(-1)
					case "a":
						m.betAllIn()
					}
					return
				}
			}
			if k == "+" || k == "=" || k == "-" || k == "a" {
				return
			}
		case "1", "2", "3", "4", "5":
			if m.DrawerOpen && (m.Phase == "new" || m.Phase == "hold") {
				i := int(k[0] - '1')
				m.toggleDiscard(i)
				m.SelectedCard = i
				m.Focus = "cards"
				m.Phase = "hold"
			}
			return
		}
	}
	if m.Screen != "game" || !m.DrawerOpen {
		switch k {
		case "up":
			m.Scroll--
		case "down":
			m.Scroll++
		case "pgup":
			m.Scroll -= max(1, m.geometry().ArtH-2)
		case "pgdown":
			m.Scroll += max(1, m.geometry().ArtH-2)
		case "home":
			m.Scroll = 0
		case "end":
			m.Scroll = 72
		}
		m.clamp()
	}
}
func (m *Model) perPage() int {
	// Three enlarged portraits per page fit every supported size (min 100x35).
	return 3
}
func (m *Model) modalKey(k string) {
	if (k == "f1" || k == "?") && m.Modal == "help" {
		m.Modal = ""
		return
	}
	if k == "esc" && m.Modal == "levelresult" {
		m.applyLevelTransition()
		return
	}
	if k == "esc" && m.Modal == "victory" {
		m.Modal = "" // inspect image 5
		return
	}
	if k == "esc" || k == "f12" && m.Modal == "developer" {
		m.Modal = ""
		return
	}
	switch m.Modal {
	case "pause":
		switch k {
		case "enter":
			m.pauseActivateIndex(m.ModalFocus)
		case "tab", "down", "right":
			m.ModalFocus = (m.ModalFocus + 1) % len(pauseItems())
		case "shift+tab", "up", "left":
			m.ModalFocus = (m.ModalFocus - 1 + len(pauseItems())) % len(pauseItems())
		case "s":
			m.goScreen("settings")
		case "h":
			m.openQuitToMenuConfirm()
		}
	case "quit":
		switch k {
		case "esc":
			m.Modal = "" // return to the title screen
		case "enter", "space":
			m.quitConfirmChoice(m.ModalFocus == 0)
		case "tab", "shift+tab", "left", "right", "up", "down":
			m.ModalFocus = 1 - m.ModalFocus
		}
	case "quittomenu":
		switch k {
		case "esc":
			m.quitToMenuChoice(false)
		case "enter", "space":
			m.quitToMenuChoice(m.ModalFocus == 0)
		case "tab", "shift+tab", "left", "right", "up", "down":
			m.ModalFocus = 1 - m.ModalFocus
		}
	case "levelresult":
		if k == "enter" {
			m.applyLevelTransition()
		}
	case "victory":
		switch k {
		case "enter":
			m.victoryActivateIndex(m.ModalFocus)
		case "tab", "down", "right":
			m.ModalFocus = (m.ModalFocus + 1) % len(victoryItems())
		case "shift+tab", "up", "left":
			m.ModalFocus = (m.ModalFocus - 1 + len(victoryItems())) % len(victoryItems())
		case "r":
			m.playAgain()
		case "c":
			m.goScreen("select")
		}
	case "help":
		switch k {
		case "up":
			m.scrollHelp(-1)
		case "down":
			m.scrollHelp(1)
		case "pgup":
			m.scrollHelp(-10)
		case "pgdown":
			m.scrollHelp(10)
		case "home":
			m.HelpScroll = 0
		case "end":
			m.scrollHelp(len(m.helpLines()))
		}
	case "developer":
		m.devKey(k)
	}
}
func (m *Model) devKey(k string) bool {
	switch k {
	case "f3":
		m.goScreen(cycle(ScreenOrder, m.Screen))
	case "f4":
		m.setPhase(cycle(PhaseOrder, m.Phase))
	case "f5":
		m.selectCharacter(m.Selected + 1)
	case "f6":
		m.Stage = m.Stage%5 + 1
		m.Scroll = 0
		m.progressEligible = false
	case "f7", "7":
		if m.Game != nil {
			m.progressEligible = false
			m.Game.ForceBustForFixture(game.Opponent)
			m.resolveLevelEnd()
		}
	case "f8", "8":
		if m.Game != nil {
			m.progressEligible = false
			m.Game.ForceBustForFixture(game.Human)
			m.resolveLevelEnd()
		}
	case "f9", "9":
		m.Stage = run.MaxLevel
		m.Scroll = 0
		m.progressEligible = false
		if err := m.startRealMatch(); err != nil {
			m.Notice = err.Error()
		}
	default:
		return false
	}
	return true
}

// settingsFocusCount is the size of the Settings focus ring: every setting row
// plus the Back action the screen renders, so focus never skips an action the
// mouse can click.
func (m *Model) settingsFocusCount() int { return len(m.settingsItems()) + 1 }

// settingsBackFocused reports whether focus sits on the Back action.
func (m *Model) settingsBackFocused() bool { return m.Setting >= len(m.settingsItems()) }

// leaveSettings returns to the screen that opened Settings. The licence and
// purchases children deliberately never replace that origin, so this always
// unwinds the way the player came in.
func (m *Model) leaveSettings() {
	s := m.SettingsReturnScreen
	if s == "" || s == "settings" {
		s = "select"
	}
	m.Screen = s
	m.Modal = m.SettingsReturnModal
	m.SettingsReturnModal = ""
	if m.Modal == "pause" {
		m.ModalFocus = 0
	}
}

func (m *Model) settingsKey(k string) {
	items := m.settingsItems()
	count := m.settingsFocusCount()
	if m.Setting < 0 || m.Setting >= count {
		m.Setting = 0
	}
	switch k {
	case "esc":
		m.leaveSettings()
	case "up", "shift+tab":
		m.Setting = (m.Setting + count - 1) % count
	case "down", "tab":
		m.Setting = (m.Setting + 1) % count
	case "enter", "space", "left", "right":
		if m.settingsBackFocused() {
			m.leaveSettings()
			return
		}
		m.activateSetting(items[m.Setting].id)
		m.persistProgress()
	}
}
func (m *Model) mouse(p tea.Mouse, wheel bool) {
	// Only repeated presses on the just-dismissed Close area are consumed.
	// Other controls and all keyboard/wheel input remain immediately available.
	if !wheel && p.Button == tea.MouseLeft && time.Now().Before(m.helpCloseUntil) && m.helpCloseHit.contains(p.X, p.Y) {
		return
	}
	if m.Modal == "help" {
		m.helpMouse(p, wheel)
		return
	}
	if !wheel && time.Now().Before(m.mouseBlockedUntil) {
		return
	}
	if m.Modal != "" {
		if !wheel && p.Button == tea.MouseLeft {
			for _, h := range m.render().Hits {
				if h.contains(p.X, p.Y) {
					m.clickHit(h.ID, time.Now())
					return
				}
			}
		}
		return
	}
	if m.W < 100 || m.H < 35 {
		return
	}
	g := m.geometry()
	covered := m.Screen == "game" && m.Reveal > 0 && p.Y >= m.H-1-m.Reveal && p.Y < m.H-1
	if wheel {
		if m.Screen == "gallery" && m.GalleryDetail && p.Y >= m.galleryGeometry().ArtY && p.Y < m.H-1 {
			if p.Button == tea.MouseWheelUp {
				m.GalleryScroll -= m.ScrollStep
			} else if p.Button == tea.MouseWheelDown {
				m.GalleryScroll += m.ScrollStep
			}
			m.clampGalleryScroll()
			return
		}
		if (m.Screen == "game" || m.Screen == "intro" || m.Screen == "detail") && !covered && p.Y >= g.ArtY && p.Y < m.H-2 {
			for _, h := range m.render().Hits {
				if h.ID == "shield" && h.contains(p.X, p.Y) {
					return
				}
			}
			if p.Button == tea.MouseWheelUp {
				m.scrollPortrait(-m.ScrollStep)
			} else if p.Button == tea.MouseWheelDown {
				m.scrollPortrait(m.ScrollStep)
			}
		}
		return
	}
	if p.Button != tea.MouseLeft {
		return
	}
	for _, h := range m.render().Hits {
		if h.contains(p.X, p.Y) {
			m.clickHit(h.ID, time.Now())
			return
		}
	}
}
func (m *Model) hit(id string) {
	defer m.inputFeedback(id, true)()
	if m.galleryHit(id) {
		return
	}
	var i int
	switch {
	case id == "adult-content-primary":
		m.activateAdultContentPrimary()
	case id == "adult-content-exit":
		m.QuitRequested = true
	case id == "drawer":
		m.toggle()
	case id == "activate":
		m.activate()
	case id == "collection-prev":
		m.selectCollection(-1)
	case id == "collection-next":
		m.selectCollection(+1)
	case id == "collection-open":
		if m.collection().isAvailable() {
			m.goScreen("select")
		}
	case id == "gallery":
		m.goScreen("gallery")
	case id == "title":
		m.enterTitle()
	case id == "settings":
		m.goScreen("settings")
	case id == "home":
		m.goScreen("select")
	case id == "back":
		m.key("esc")
	case id == "intro-back":
		m.introBack()
	case id == "help":
		m.openModal("help")
	case id == "close":
		m.Modal = ""
	case id == "resume":
		m.Modal = ""
	case id == "menu":
		m.openModal("pause")
	case id == "characters":
		m.openQuitToMenuConfirm()
	case id == "quit":
		m.QuitRequested = true
	case id == "quit-yes":
		m.quitConfirmChoice(true)
	case id == "quit-no":
		m.quitConfirmChoice(false)
	case id == "quittomenu-yes":
		m.quitToMenuChoice(true)
	case id == "quittomenu-no":
		m.quitToMenuChoice(false)
	case id == "page-next":
		m.pageDown()
	case id == "page-prev":
		m.pageUp()
	case id == "demo-options":
		m.DemoOptionsOpen = !m.DemoOptionsOpen
	case id == "bet-minus":
		if m.Game != nil {
			m.Focus = "bet-minus"
			m.adjustBetAmount(-1)
		} else {
			m.Bet = max(50, m.Bet-50)
			m.Focus = "bet"
		}
	case id == "bet-plus":
		if m.Game != nil {
			m.Focus = "bet-plus"
			m.adjustBetAmount(1)
		} else {
			m.Bet = min(500, m.Bet+50)
			m.Focus = "bet"
		}
	case id == "level-continue":
		m.applyLevelTransition()
	case id == "victory-close":
		m.Modal = ""
	case id == "victory-playagain":
		m.playAgain()
	case id == "victory-characters":
		m.goScreen("select")
	default:
		var cid string
		if n, _ := fmt.Sscanf(id, "character:%s", &cid); n == 1 {
			if m.Screen == "select" {
				m.SelectFocus = 0
			}
			m.selectCharacterByID(cid)
			return
		}
		if n, _ := fmt.Sscanf(id, "card:%d", &i); n == 1 && m.DrawerOpen && (m.Phase == "new" || m.Phase == "hold") {
			m.SelectedCard = i
			m.Focus = "cards"
			m.toggleDiscard(i)
			return
		}
		if n, _ := fmt.Sscanf(id, "action:%d", &i); n == 1 && m.DrawerOpen {
			m.Action = i
			m.Focus = "actions"
			m.activate()
			return
		}
		if n, _ := fmt.Sscanf(id, "setting:%d", &i); n == 1 {
			m.Setting = i
			m.settingsKey("enter")
		}
	}
}

func (m *Model) Init() tea.Cmd {
	if m.freshSoundPending {
		m.freshSoundPending = false
		m.sfx(audio.NewGameStart)
	}
	m.syncTitleMusic()
	return m.commands()
}
func (m *Model) commands() tea.Cmd {
	var cmds []tea.Cmd
	if cmd := m.betFeedbackCommand(time.Now()); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.sparkleCommand(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if m.clipboardCopy != "" {
		content := m.clipboardCopy
		m.clipboardCopy = ""
		m.copyToken++
		cmds = append(cmds, writeClipboardCommand(content, m.copyToken))
	}
	if !m.copyFeedbackUntil.IsZero() && !m.copyFeedbackPending {
		m.copyFeedbackPending = true
		m.copyFeedbackID++
		id := m.copyFeedbackID
		cmds = append(cmds, tea.Tick(time.Until(m.copyFeedbackUntil), func(time.Time) tea.Msg { return copyFeedbackMsg{ID: id} }))
	}
	target := 0
	if m.DrawerOpen {
		target = m.geometry().DrawerH
	}
	if m.Screen == "game" && m.Reveal != target && !m.Config.ReducedMotion && !m.animationPending && m.Modal == "" {
		m.animationPending = true
		cmds = append(cmds, tea.Tick(30*time.Millisecond, func(time.Time) tea.Msg { return frameMsg{} }))
	}
	if m.revealPending && m.Screen == "game" && m.Modal == "" {
		cmds = append(cmds, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return revealMsg{} }))
	}
	if m.dealReveal && m.Screen == "game" && m.Modal == "" {
		cmds = append(cmds, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg { return dealMsg{} }))
	}
	if m.opponentRevealPending && !m.opponentRevealScheduled && m.Screen == "game" && m.Modal == "" {
		m.opponentRevealScheduled = true
		token := m.opponentRevealToken
		cmds = append(cmds, tea.Tick(opponentRevealDelay, func(time.Time) tea.Msg { return opponentRevealMsg{Token: token} }))
	}
	if m.Showdown != nil && !m.Showdown.ShufflePending && !m.Showdown.ShufflePlayed && !m.Showdown.MatchOver && m.Screen == "game" && m.Modal == "" {
		m.Showdown.ShufflePending = true
		// Capture the token now; the tick fires up to 900 ms later, by which
		// point Continue may have cleared m.Showdown (nil deref otherwise).
		token := m.Showdown.Token
		cmds = append(cmds, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return shuffleMsg{Token: token} }))
	}
	if m.Screen == "game" && m.Game != nil && m.Modal == "" && m.Game.Actor() == game.Opponent && (m.Game.Phase() == game.Drawing || m.Game.Phase() == game.BettingBeforeDraw || m.Game.Phase() == game.BettingAfterDraw) && !m.opponentPending {
		m.opponentPending = true
		cmds = append(cmds, m.makeOpponentCommand(m.opponentEpoch, m.Game.Token()))
	} else if m.Screen == "game" && m.Game == nil && m.Phase == "opponent" && m.Modal == "" && !m.opponentPending {
		m.opponentPending = true
		epoch := m.opponentEpoch
		cmds = append(cmds, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return opponentMsg{Epoch: epoch, Demo: true} }))
	}
	return tea.Batch(cmds...)
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	screen, modal, detail := m.Screen, m.Modal, m.GalleryDetail
	defer func() {
		if screen != m.Screen || modal != m.Modal || detail != m.GalleryDetail {
			m.clearPointer()
		}
		if m.Screen != "game" || m.Phase != "victory" {
			m.stopVictorySound()
		}
	}()
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(v.Width, v.Height)
	case tea.KeyPressMsg:
		key := v.String()
		// Ctrl+C is the deliberate quit shortcut and always wins, even while a
		// text field owns the keyboard.
		if key == "ctrl+c" {
			return m, m.requestExit()
		}
		if key == "q" {
			return m, m.requestExit()
		}
		m.keyEvent(key, v.Key().IsRepeat)
		if m.Modal == "help" {
			return m, nil
		}
	case tea.KeyReleaseMsg:
		if m.Screen == adultContentScreen {
			m.adultContentInputBlocked = false
		}
	case tea.MouseMotionMsg:
		m.mouseMotion(v.Mouse())
		return m, nil // hover changes only paint; never schedule game/network work
	case tea.BlurMsg:
		m.clearPointer()
	case tea.MouseClickMsg:
		m.mouse(v.Mouse(), false)
		if m.Modal == "help" {
			return m, nil
		}
	case tea.MouseWheelMsg:
		m.mouse(v.Mouse(), true)
		if m.Modal == "help" {
			return m, nil
		}
	case betFeedbackMsg:
		return m, m.betFeedbackTick(v.ID, time.Now())
	case sparkleMsg:
		if !m.advanceSparkles(v.Epoch) {
			return m, nil
		}
	case frameMsg:
		m.animationPending = false
		m.animate()
	case revealMsg:
		if m.revealPending && m.Screen == "game" && m.Modal == "" {
			m.revealStep++
			m.sfx(audio.CardSlide)
			if m.revealStep >= len(m.revealOrder) {
				m.revealPending = false
			}
		}
	case dealMsg:
		m.dealTick()
	case shuffleMsg:
		m.shuffleTick(v.Token)
	case opponentRevealMsg:
		if m.opponentRevealPending && v.Token == m.opponentRevealToken && m.Game != nil && m.Game.Token() == v.Token {
			// Release the scheduled flag so a blocked reveal (e.g. help open)
			// re-schedules on the next commands() instead of getting stuck.
			m.opponentRevealScheduled = false
			if m.Screen == "game" && m.Modal == "" {
				m.completeOpponentReveal()
			}
		}
	case copyResultMsg:
		if v.Token == m.copyToken {
			m.copyFeedbackErr = v.Err
			m.copyFeedbackUntil = time.Now().Add(copyFeedbackDuration)
		}
	case copyFeedbackMsg:
		if v.ID == m.copyFeedbackID {
			m.copyFeedbackPending = false
			m.copyFeedbackUntil = time.Time{}
			m.copyFeedbackErr = nil
		}
	case opponentMsg:
		if v.Demo {
			if v.Epoch == m.opponentEpoch {
				m.opponentPending = false
				if m.Modal == "" && m.Screen == "game" {
					m.finishOpponent()
				}
			}
		} else {
			m.applyOpponentMessage(v)
		}
	}
	if m.QuitRequested {
		m.QuitRequested = false
		return m, m.requestExit()
	}
	return m, m.commands()
}
