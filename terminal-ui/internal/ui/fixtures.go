package ui

import (
	"fmt"

	"midnight-royale/internal/game"
	"midnight-royale/internal/opponent"
)

type Character struct {
	ID, Name, Intro, Tag string
	Completed            bool
	Collection           CollectionID
}

func Characters() []Character {
	chars := []Character{
		{"amy", "Amy", "A new arrival. A familiar game. Take a seat and make your first hand count.", "THE ARRIVAL", false, CollectionGrandRivieraHotel},
		{"chloe", "Chloe", "The city can wait. Chloe has a table by the window and a very good poker face.", "CITY LIGHTS", false, CollectionGrandRivieraHotel},
		{"ruby", "Ruby", "Ruby plays with patience. Keep your cards close and watch for your opening.", "AFTER HOURS", false, CollectionGrandRivieraHotel},
		{"jenny", "Jenny", "One more hand before midnight. Jenny never likes to leave a story unfinished.", "LAST CALL", false, CollectionGrandRivieraHotel},
		{"sophie", "Sophie", "An old favourite returns to the table. There is always another hand to play.", "ENCORE", true, CollectionGrandRivieraHotel},
		{"vanessa", "Vanessa", "A quiet room, a fresh deck, and a challenge worth staying up for.", "VELVET ROOM", false, CollectionGrandRivieraHotel},
		{"candy", "Candy", "Bright lights, quick smiles. Candy is ready when you are.", "SUGAR RUSH", false, CollectionGrandRivieraHotel},
		{"roxanne", "Roxanne", "Roxanne calls the next hand. Make it a memorable one.", "NIGHT DRIVE", false, CollectionGrandRivieraHotel},
		{"tiffany", "Tiffany", "A little elegance. A little luck. A seat reserved just for you.", "BLUE HOUR", false, CollectionGrandRivieraHotel},
		{"scarlett", "Scarlett", "Scarlett raises an eyebrow. The rest is up to your cards.", "RED LETTER", false, CollectionGrandRivieraHotel},
		{"victoria", "Victoria", "Every card has its moment. Victoria is happy to wait for hers.", "GOLD STANDARD", false, CollectionGrandRivieraHotel},
		{"electra", "Electra", "The night is still young. Electra has one last challenge in mind.", "NEON HEART", false, CollectionGrandRivieraHotel},
		{"amber", "Amber", "A suitcase in one hand and a seat at the table. Amber plays every hand like a first hello.", "GOLDEN HOUR", false, CollectionAzurePalmsResort},
		{"bianca", "Bianca", "Bianca arrives with a smile and a bag full of sunshine. The cards are just getting comfortable.", "ISLAND HOP", false, CollectionAzurePalmsResort},
		{"celeste", "Celeste", "The tide is high and so are the stakes. Celeste keeps her cards close to the surf.", "TIDAL PULL", false, CollectionAzurePalmsResort},
		{"elena", "Elena", "Sunglasses on, guard down. Elena knows exactly when to raise.", "SUN KISSED", false, CollectionAzurePalmsResort},
		{"leila", "Leila", "A quiet dock, a slow evening, and a hand worth staying for. Leila is already at the table.", "LAGOON LIGHT", false, CollectionAzurePalmsResort},
		{"maren", "Maren", "Maren looks back just long enough to make you look twice. Then she bets.", "PALM SHADOW", false, CollectionAzurePalmsResort},
		{"marina", "Marina", "A flower in her hair and a plan up her sleeve. Marina deals in quiet confidence.", "TROPIC BLOOM", false, CollectionAzurePalmsResort},
		{"nadia", "Nadia", "Nadia came for the sun and stayed for the game. The pot is only getting bigger.", "CORAL CROSSING", false, CollectionAzurePalmsResort},
		{"priya", "Priya", "Priya arrives with purpose and a pair of cards she likes. The night is young and warm.", "MONSOON BLUE", false, CollectionAzurePalmsResort},
		{"simone", "Simone", "Simone keeps her sunglasses on and her tells hidden. Good luck reading this hand.", "HIGH TIDE", false, CollectionAzurePalmsResort},
		{"talia", "Talia", "Talia finds a table by the palms and settles in. Every sunset needs a final hand.", "SUNSET SLIDE", false, CollectionAzurePalmsResort},
		{"vivienne", "Vivienne", "Vivienne watches the water and the cards with equal patience. Then she strikes.", "HARBOR GLOW", false, CollectionAzurePalmsResort},
		{"sora", "Sora", "Sora catches every angle. Give the photographer something worth remembering.", "NIGHT FOCUS", false, CollectionStarlightLodge},
		{"yasmin", "Yasmin", "Yasmin recognizes every note in the air. Try leaving an impression she can’t bottle.", "BOTTLED LIGHT", false, CollectionStarlightLodge},
		{"amara", "Amara", "Amara knows exactly when to hold a note—and when to keep you waiting. Don’t miss your cue.", "LAST CHORUS", false, CollectionStarlightLodge},
		{"elise", "Elise", "Elise never waits for permission to take off. Give her a reason to stay grounded.", "NORTH WIND", false, CollectionStarlightLodge},
		{"noura", "Noura", "Noura believes anticipation is part of the pleasure. Let her decide how close you get.", "SLOW BURN", false, CollectionStarlightLodge},
		{"solveig", "Solveig", "Solveig plays by instinct, not by the score. See if you can keep up when she changes tempo.", "OFF SCORE", false, CollectionStarlightLodge},
		{"leona", "Leona", "Leona has flown through worse than this. Keep your nerve if you want her attention.", "SMOOTH GLIDE", false, CollectionStarlightLodge},
		{"valentina", "Valentina", "Valentina knows a rare vintage when she finds one. Convince her you’re worth a second taste.", "SECOND POUR", false, CollectionStarlightLodge},
		{"nadja", "Nadja", "Nadja has already studied every angle. Try giving the architect something she didn’t plan for.", "LONG SHADOW", false, CollectionStarlightLodge},
		{"imani", "Imani", "Imani studies storms for a living. Tonight, she may be the one making the weather.", "STORM WATCH", false, CollectionStarlightLodge},
		{"samira", "Samira", "Samira stays calm when everything else is falling. Make her lose focus—just once.", "DEAD CALM", false, CollectionStarlightLodge},
		{"lucia", "Lucía", "Lucía commands the room before a single note is played. Follow her lead—or miss the grand finale.", "GRAND FINALE", false, CollectionStarlightLodge},
	}
	return chars
}

// ShippedCharacters returns the character roster the build can present.
func ShippedCharacters() []Character {
	// 2.1.0 ships the whole catalogue in every edition; the Open edition makes
	// all of it free, so the full roster is visible and playable.
	return Characters()
}

var PhaseOrder = []string{"new", "hold", "bet", "opponent", "win", "loss", "tie", "match", "victory"}
var ScreenOrder = []string{"opening", "collection", "select", "gallery", "intro", "game", "settings"}

func cycle(list []string, current string) string {
	for i, s := range list {
		if s == current {
			return list[(i+1)%len(list)]
		}
	}
	return list[0]
}
func (m *Model) character() Character {
	roster := m.roster()
	if len(roster) == 0 {
		return Character{}
	}
	return roster[m.selectedIndex()]
}

// accessible reports whether the selected opponent can be played. Every
// character ships with the game, so the only thing that could stand in the way
// is a missing one.
func (m *Model) accessible() bool { return len(m.roster()) > 0 }

// roster returns the opponents belonging to the selected collection, in
// catalog order. It is the single filtered view used by the opponent selector.
func (m *Model) roster() []Character {
	var out []Character
	for _, ch := range m.Characters {
		if ch.Collection == m.SelectedCollection {
			out = append(out, ch)
		}
	}
	return out
}

// rosterPos returns the position of a character id within the active
// collection's roster, or 0 when absent.
func (m *Model) rosterPos(id string) int {
	for i, ch := range m.roster() {
		if ch.ID == id {
			return i
		}
	}
	return 0
}

// characterByID resolves a character from the full catalog by stable id.
func (m *Model) characterByID(id string) (Character, bool) {
	for _, ch := range m.Characters {
		if ch.ID == id {
			return ch, true
		}
	}
	return Character{}, false
}

// selectedIndex clamps m.Selected to the active roster so a stale cursor can
// never render an invalid page or resolve a character outside the collection.
func (m *Model) selectedIndex() int {
	n := len(m.roster())
	if n == 0 {
		return 0
	}
	if m.Selected < 0 {
		return 0
	}
	if m.Selected >= n {
		return n - 1
	}
	return m.Selected
}

// characterBandFor returns the fixed difficulty label for a character id.
func characterBandFor(id string) string {
	if p, ok := opponent.ProfileFor(id); ok {
		return p.Band.String()
	}
	return "Medium"
}

func (m *Model) characterBand() string { return characterBandFor(m.character().ID) }

// characterIndexByID returns the absolute index of a character within the
// fixed catalog order, or 0 when the id is unknown.
func (m *Model) characterIndexByID(id string) int {
	for i, ch := range m.Characters {
		if ch.ID == id {
			return i
		}
	}
	return 0
}

func (m *Model) reaction() string {
	switch m.Phase {
	case "win":
		return "That was a lucky hand."
	case "loss":
		return "I should have kept that ace."
	case "tie":
		return "We split the pot."
	case "opponent":
		return "Let me think about this one."
	case "match":
		return "The table resets."
	default:
		return "Your move."
	}
}
func (m *Model) actions() []string {
	if m.opponentRevealPending {
		// The hand has settled but the reveal pause is still running: expose no
		// action so Continue cannot be pressed or clicked before the player has
		// seen whether they won or lost.
		return nil
	}
	if m.Showdown != nil {
		if m.Showdown.ShowMine {
			return []string{"Show " + m.character().Name + "'s hand", "Continue"}
		}
		return []string{"Show my hand", "Continue"}
	}
	if m.Game != nil {
		switch m.Game.Phase() {
		case game.Drawing:
			if m.Game.Actor() == game.Human {
				return drawConfirmActions(len(m.selectedDiscards()))
			}
		case game.BettingBeforeDraw, game.BettingAfterDraw:
			if m.Game.Actor() == game.Human {
				return m.betActionLabels()
			}
		case game.HandResult:
			return []string{m.handResultAction()}
		}
		return nil
	}
	switch m.Phase {
	case "new", "hold":
		return drawConfirmActions(len(m.selectedDiscards()))
	case "bet":
		return []string{"Bet", "Fold"}
	case "win", "loss", "tie":
		return []string{"Next hand"}
	default:
		return nil
	}
}

// drawConfirmActions renders the single draw-phase confirmation label. A zero
// discard selection offers "Keep all"; otherwise it names the exact exchange.
func drawConfirmActions(n int) []string {
	if n == 0 {
		return []string{"Keep all"}
	}
	return []string{fmt.Sprintf("Exchange %d", n)}
}

// Pause overlay actions. IDs are reused by both the keyboard focus model and
// the mouse hit targets so both input paths drive the same state.
type pauseItem struct {
	id    string
	label string
}

func pauseItems() []pauseItem {
	return []pauseItem{
		{"resume", "Resume"},
		{"settings", "Settings"},
		{"characters", "Quit to Main Menu"},
		{"quit", "Quit"},
	}
}

func (m *Model) pauseActivateIndex(i int) {
	items := pauseItems()
	if i < 0 || i >= len(items) {
		return
	}
	m.pauseActivateID(items[i].id)
}

func (m *Model) pauseActivateID(id string) {
	switch id {
	case "resume":
		m.Modal = ""
	case "settings":
		m.goScreen("settings")
	case "characters":
		m.openQuitToMenuConfirm()
	case "quit":
		m.QuitRequested = true
	}
}

// Victory overlay actions: a small, dismissible set shown on image 5.
func victoryItems() []pauseItem {
	return []pauseItem{
		{"victory-close", "Close window"},
		{"victory-playagain", "Play again"},
		{"victory-characters", "Character selection"},
	}
}

func (m *Model) victoryActivateIndex(i int) {
	items := victoryItems()
	if i < 0 || i >= len(items) {
		return
	}
	m.victoryActivateID(items[i].id)
}

func (m *Model) victoryActivateID(id string) {
	switch id {
	case "victory-close":
		m.Modal = "" // close the dialog and leave image 5 visible for inspection
	case "victory-playagain":
		m.playAgain()
	case "victory-characters":
		m.goScreen("select")
	}
}

// selectButtons lists the focusable action buttons on the character-selection
// screen, in Tab order. The character grid is focus index 0 (not listed here).
func (m *Model) selectButtons() []pauseItem {
	items := []pauseItem{
		{"activate", "Continue"},
		{"gallery", "Gallery"},
		{"title", "Title screen"},
	}
	return append(items, pauseItem{"settings", "Settings"})
}

func (m *Model) selectActivateIndex(i int) {
	btns := m.selectButtons()
	if i < 0 || i >= len(btns) {
		return
	}
	m.selectActivateID(btns[i].id)
}

func (m *Model) selectActivateID(id string) {
	switch id {
	case "activate":
		m.activate()
	case "gallery":
		m.goScreen("gallery")
	case "title":
		m.enterTitle()
	case "settings":
		m.goScreen("settings")
	}
}
