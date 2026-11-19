package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"midnight-royale/internal/audio"
	"midnight-royale/internal/game"
	"midnight-royale/internal/opponent"
	"midnight-royale/internal/progress"
	"midnight-royale/internal/run"
)

func (m *Model) initProgress() {
	m.Save = progress.NewData()
	if m.Config.DisableSave {
		return
	}
	path := m.Config.SavePath
	if path == "" {
		var err error
		if m.Config.Dev || m.Config.Demo {
			path, err = progress.DevelopmentPath()
		} else {
			path, err = progress.DefaultPath()
		}
		if err != nil {
			m.Notice = "Local save unavailable: " + err.Error()
			return
		}
	}
	m.Store = progress.NewStore(path)
	data, err := m.Store.Load()
	if err != nil {
		m.Notice = "Save not loaded; original file preserved: " + err.Error()
		return
	}
	m.Save = data
	m.Config.ReducedMotion = data.Preferences.ReducedMotion
	m.Config.ASCII = data.Preferences.ASCII
	m.SoundEffects = data.Preferences.SoundOn()
	if data.Preferences.ScrollStep >= 1 && data.Preferences.ScrollStep <= 5 {
		m.ScrollStep = data.Preferences.ScrollStep
	}
	for i := range m.Characters {
		saved := data.Character(progressNamespace(m.Characters[i].Collection), m.Characters[i].ID)
		m.Characters[i].Completed = saved.Completed
	}
}

func (m *Model) persistProgress() {
	if m.Store == nil {
		return
	}
	m.Save.Preferences = progress.Preferences{ReducedMotion: m.Config.ReducedMotion, ASCII: m.Config.ASCII, ScrollStep: m.ScrollStep, SoundEffects: &m.SoundEffects}
	if err := m.Store.Save(m.Save); err != nil {
		m.Notice = "Progress not saved: " + err.Error()
	}
}

// startRealMatch begins a fresh level at m.Stage with 500 chips each and a
// clean deck/hand state.
func (m *Model) startRealMatch() error {
	if m.Config.Demo {
		return errors.New("ui: deterministic visual demo is separate from real play")
	}
	cfg := game.DefaultConfig()
	var dealRNG game.RNG = game.CryptoRNG{}
	var botRNG game.RNG = game.CryptoRNG{}
	if m.Config.DeterministicSeed != 0 {
		dealRNG = game.NewSeededRNG(m.Config.DeterministicSeed)
		botRNG = game.NewSeededRNG(m.Config.DeterministicSeed + 1)
	}
	session, err := game.New(cfg, dealRNG)
	if err != nil {
		return err
	}
	if err := session.StartHand(); err != nil {
		return err
	}
	profile, ok := opponent.ProfileFor(m.character().ID)
	if !ok {
		profile = opponent.DefaultProfiles()[0]
	}
	m.Game = session
	m.Bot = opponent.New(profile, botRNG)
	m.Held = [5]bool{true, true, true, true, true}
	m.Replaced = [5]bool{}
	m.revealPending = false
	m.revealOrder = nil
	m.revealStep = 0
	m.opponentRevealPending = false
	m.opponentRevealScheduled = false
	m.SelectedCard = 0
	m.Hand = 1
	m.BetAmount = 0
	m.LevelTransition = run.Transition{}
	m.clearShowdown()
	m.syncEnginePresentation()
	m.beginDealReveal()
	return nil
}

// abandonRun discards the active in-memory match and resets transient
// gameplay state so no resumable session remains. It is the single lifecycle
// boundary for the Quit-to-Main-Menu confirmation's "Yes" path. Long-term
// progress, ownership, preferences and the current character/collection
// selection are deliberately left untouched.
func (m *Model) abandonRun() {
	m.cancelOpponent()
	m.resetRunScore()
	m.Game = nil
	m.Bot = nil
	m.Stage = run.MinLevel
	m.LevelTransition = run.Transition{}
	m.clearShowdown()
	m.dealReveal = false
	m.dealRevealed = 0
	m.dealToken = 0
	m.revealPending = false
	m.revealOrder = nil
	m.revealStep = 0
	m.opponentRevealPending = false
	m.opponentRevealScheduled = false
	m.opponentRevealToken = 0
	m.Hand = 0
	m.Held = [5]bool{true, true, true, true, true}
	m.Replaced = [5]bool{}
	m.SelectedCard = 0
	m.BetAmount = 0
	m.Action = 0
	m.Phase = "new"
	m.Scroll = 0
}

func (m *Model) decisionView() opponent.DecisionView {
	s := m.Game.Snapshot(game.Opponent)
	return opponent.DecisionView{
		OwnHand: s.Hand, Phase: s.Phase, Stack: s.Stacks[game.Opponent],
		OpponentStack: s.Stacks[game.Human], Pot: s.Pot, ToCall: s.ToCall,
		MinBet: m.Game.MinBet(), MinRaiseTo: m.Game.MinRaiseTo(game.Opponent),
		MaxCommitTo: m.Game.MaxCommitTo(game.Opponent),
		Raises:      s.Raises, Dealer: s.Dealer == game.Opponent,
		OpponentDrawCount: s.OpponentDrawCount, History: s.History, Legal: s.Legal,
	}
}

func (m *Model) applyOpponentDecision(ctx context.Context) error {
	if m.Game == nil || m.Bot == nil || m.Game.Actor() != game.Opponent {
		return errors.New("ui: opponent has no current decision")
	}
	view := m.decisionView()
	var err error
	switch m.Game.Phase() {
	case game.Drawing:
		var indexes []int
		indexes, err = m.Bot.ChooseDraw(ctx, view)
		if err == nil {
			err = m.Game.Draw(game.Opponent, indexes, m.Game.Token())
		}
	case game.BettingBeforeDraw, game.BettingAfterDraw:
		var action game.Action
		var amount int
		action, amount, err = m.Bot.ChooseBet(ctx, view)
		if err == nil {
			err = m.Game.Act(game.Opponent, action, amount, m.Game.Token())
		}
	default:
		err = errors.New("ui: opponent decision requested in non-decision phase")
	}
	if err != nil {
		return err
	}
	m.syncEnginePresentation()
	return nil
}

func (m *Model) applyHumanAction(action game.Action, amount int) error {
	if m.Game == nil {
		return errors.New("ui: no real match")
	}
	if err := m.Game.Act(game.Human, action, amount, m.Game.Token()); err != nil {
		return err
	}
	m.clearReplacementIndicators()
	m.syncEnginePresentation()
	return nil
}

func (m *Model) applyHumanDraw(indexes []int) error {
	if m.Game == nil {
		return errors.New("ui: no real match")
	}
	if err := m.Game.Draw(game.Human, indexes, m.Game.Token()); err != nil {
		return err
	}
	m.Held = [5]bool{true, true, true, true, true}
	m.beginReplacementReveal(indexes)
	m.syncEnginePresentation()
	return nil
}

// beginReplacementReveal marks the discarded slots as NEW and, unless reduced
// motion is on, schedules a left-to-right flip of the freshly dealt cards.
func (m *Model) beginReplacementReveal(indexes []int) {
	m.Replaced = [5]bool{}
	for _, i := range indexes {
		if i >= 0 && i < 5 {
			m.Replaced[i] = true
		}
	}
	sorted := slices.Clone(indexes)
	slices.Sort(sorted)
	m.revealOrder = sorted
	m.revealStep = 0
	if m.Config.ReducedMotion || len(sorted) == 0 {
		m.revealPending = false
		m.revealStep = len(sorted)
		if len(sorted) > 0 {
			m.sfx(audio.CardSlide)
		}
	} else {
		m.revealPending = true
	}
}

func (m *Model) clearReplacementIndicators() {
	m.Replaced = [5]bool{}
	m.revealPending = false
	m.revealOrder = nil
	m.revealStep = 0
}

func (m *Model) syncEnginePresentation() {
	if m.Game == nil {
		return
	}
	phase := m.Game.Phase()
	if (phase == game.BettingBeforeDraw || phase == game.BettingAfterDraw || phase == game.Drawing) && m.Game.Actor() == game.Opponent {
		m.Phase = "opponent"
	} else {
		switch phase {
		case game.BettingBeforeDraw, game.BettingAfterDraw:
			m.Phase = "bet"
			m.initBetAmount()
		case game.Drawing:
			m.Phase = "hold"
		case game.HandResult:
			result := m.Game.Result()
			if result.Tie {
				m.Phase = "tie"
			} else if result.Winner == game.Human {
				m.Phase = "win"
			} else {
				m.Phase = "loss"
			}
			// Count a lost hand exactly once per settled hand: a fold by the
			// player or a showdown loss. Ties cost nothing.
			if !result.Tie && result.Winner == game.Opponent {
				m.countLostHand()
			}
			if result.Folded {
				m.enterShowdown(result)
			} else {
				m.beginOpponentReveal()
			}
		case game.MatchResult:
			m.Phase = "match"
		}
	}
	m.Action = 0
	if m.DrawerOpen {
		m.Focus = m.phaseFocus()
	}
	m.opponentEpoch++
	m.opponentPending = false
}

// earnArtwork is the only UI seam that turns a proven real-game milestone
// into local Gallery progress. Demo presentation state, commerce state, and
// installed assets never call through it.
func (m *Model) earnArtwork(image int) {
	if m.Config.Demo || !m.progressEligible {
		return
	}
	if image == progress.FinalArtworkImage {
		m.Characters[m.characterIndexByID(m.character().ID)].Completed = true
	}
	if m.Save.Earn(progressNamespace(m.character().Collection), m.character().ID, image) {
		m.persistProgress()
	}
}

// markCompleted is kept as the completion milestone adapter used by victory.
func (m *Model) markCompleted() {
	m.earnArtwork(progress.FinalArtworkImage)
}

// resolveLevelEnd evaluates a settled bust and either shows level-transition
// feedback or enters the victory state. It is the single transition entry point.
func (m *Model) resolveLevelEnd() {
	if m.Game == nil || !m.Game.Result().MatchOver || m.LevelTransition.Kind != 0 || m.Phase == "victory" {
		return
	}
	playerBusted := m.Game.Stacks()[game.Human] == 0
	tr := run.ResolveBust(m.Stage, playerBusted)
	m.LevelTransition = tr
	switch tr.Kind {
	case run.Victory:
		m.Stage = 5
		m.markCompleted()
		m.recordRunScore()
		m.Phase = "victory"
		m.DrawerOpen = false
		m.Reveal = 0
		m.Focus = "art"
		m.openModal("victory")
		m.startVictorySound()
	case run.Advance:
		// A settled opponent bust proves the next image before the player
		// dismisses the result modal, so save it before any extra input.
		m.earnArtwork(tr.NextLevel)
		// turn-win is the scene-advance stinger (1->2, 2->3, 3->4), not a
		// poker-hand win sound. It fires once here at the committed progression
		// transition; a hand win, a re-render or a resize never triggers it.
		m.sfx(audio.TurnWin)
		m.Phase = "match"
		m.openModal("levelresult")
	case run.Retreat, run.Retry:
		m.countLostStage()
		if tr.Kind == run.Retreat {
			m.sfx(audio.Lost)
		}
		m.Phase = "match"
		m.openModal("levelresult")
	}
}

// applyLevelTransition commits a level transition and starts the next attempt.
func (m *Model) applyLevelTransition() {
	tr := m.LevelTransition
	if tr.NextLevel < run.MinLevel || tr.NextLevel > run.MaxLevel {
		return
	}
	m.Stage = tr.NextLevel
	m.LevelTransition = run.Transition{}
	m.Modal = ""
	m.Scroll = 0
	if err := m.startRealMatch(); err != nil {
		m.Notice = err.Error()
	}
}

// playAgain starts a new run at image 1 with the same character.
func (m *Model) playAgain() {
	m.beginRun()
	m.Stage = run.MinLevel
	m.progressEligible = !m.Config.Demo
	m.LevelTransition = run.Transition{}
	m.Modal = ""
	m.Scroll = 0
	m.stopVictorySound()
	if err := m.startRealMatch(); err != nil {
		m.Notice = err.Error()
		return
	}
	m.sfx(audio.NewGameStart)
	m.goScreen("game")
}

func (m *Model) nextRealHand() error {
	if m.Game == nil {
		return errors.New("ui: no real match")
	}
	if err := m.Game.NextHand(); err != nil {
		return err
	}
	m.Hand++
	m.Held = [5]bool{true, true, true, true, true}
	m.clearReplacementIndicators()
	m.syncEnginePresentation()
	m.beginDealReveal()
	return nil
}

func (m *Model) selectedRealAction() (game.Action, bool) {
	if m.Game == nil {
		return game.Check, false
	}
	legal := m.Game.LegalActions(game.Human)
	if len(legal) == 0 {
		return game.Check, false
	}
	return legal[min(m.Action, len(legal)-1)], true
}

func (m *Model) selectedDiscards() []int {
	var out []int
	for i, held := range m.Held {
		if !held {
			out = append(out, i)
		}
	}
	return out
}

// toggleDiscard flips one card between KEEP and DISCARD, enforcing the legal
// zero-to-five draw limit without silently changing an unrelated selection.
func (m *Model) toggleDiscard(i int) {
	if i < 0 || i >= 5 {
		return
	}
	if m.Held[i] {
		if len(m.selectedDiscards()) >= 5 {
			m.Notice = "You may exchange at most five cards."
			return
		}
		m.Held[i] = false
	} else {
		m.Held[i] = true
	}
}

// betAmountRange returns the legal min/max "commit to" total for the human's
// bet or raise, and whether an amount picker applies.
func (m *Model) betAmountRange() (minTo, maxTo int, ok bool) {
	if m.Game == nil || m.Game.Actor() != game.Human {
		return 0, 0, false
	}
	switch m.Game.Phase() {
	case game.BettingBeforeDraw, game.BettingAfterDraw:
	default:
		return 0, 0, false
	}
	legal := m.Game.LegalActions(game.Human)
	if slices.Contains(legal, game.Bet) {
		minTo = m.Game.MinBet()
		maxTo = m.Game.Stacks()[game.Human]
		if minTo > maxTo {
			minTo = maxTo
		}
		return minTo, maxTo, true
	}
	if slices.Contains(legal, game.Raise) {
		minTo = m.Game.MinRaiseTo(game.Human)
		maxTo = m.Game.MaxCommitTo(game.Human)
		if maxTo < minTo {
			minTo = maxTo // short all-in is the only "raise"
		}
		return minTo, maxTo, true
	}
	return 0, 0, false
}

func (m *Model) initBetAmount() {
	if minTo, _, ok := m.betAmountRange(); ok {
		m.BetAmount = minTo
	} else {
		m.BetAmount = 0
	}
}

func (m *Model) adjustBetAmount(delta int) {
	if m.dealReveal {
		return
	}
	minTo, maxTo, ok := m.betAmountRange()
	if !ok {
		return
	}
	m.flashBetAdjustment(delta, time.Now())
	before := m.BetAmount
	step := m.Game.BetStep()
	m.BetAmount = min(maxTo, max(minTo, m.BetAmount+delta*step))
	if m.BetAmount != before {
		m.sfx(audio.ChipLay)
	}
}

func (m *Model) betAllIn() {
	if m.dealReveal {
		return
	}
	if _, maxTo, ok := m.betAmountRange(); ok {
		if m.BetAmount != maxTo {
			m.BetAmount = maxTo
			m.sfx(audio.ChipLay)
		}
	}
}

// betActionLabels renders legal betting actions in LegalActions order, with the
// concrete amount for Bet/Raise. Index-aligned with selectedRealAction.
func (m *Model) betActionLabels() []string {
	legal := m.Game.LegalActions(game.Human)
	labels := make([]string, 0, len(legal))
	for _, action := range legal {
		switch action {
		case game.Check:
			labels = append(labels, "Check")
		case game.Call:
			stack := m.Game.Stacks()[game.Human]
			cost := min(m.Game.ToCall(game.Human), stack)
			label := fmt.Sprintf("Call %d", cost)
			if cost == stack {
				label += " (all-in)"
			}
			labels = append(labels, label)
		case game.Bet:
			labels = append(labels, fmt.Sprintf("Bet %d", m.BetAmount))
		case game.Raise:
			labels = append(labels, fmt.Sprintf("Raise to %d", m.BetAmount))
		case game.Fold:
			labels = append(labels, "Fold")
		}
	}
	return labels
}

// handResultAction returns the single continue label after a settled hand.
func (m *Model) handResultAction() string {
	if m.Game != nil && m.Game.Result().MatchOver {
		return "Continue"
	}
	return "Next hand"
}

// levelTransitionText renders the concise feedback for the pending transition.
func (m *Model) levelTransitionText() (title, body string) {
	switch m.LevelTransition.Kind {
	case run.Advance:
		return "LEVEL CLEARED", fmt.Sprintf("You won this round. %s wants a rematch.\n\nNext level: %d · 500 chips each.", m.character().Name, m.LevelTransition.NextLevel)
	case run.Retreat:
		return "HAND LOST", fmt.Sprintf("You ran out of chips. Back to level %d.\n\nBoth stacks reset to 500 chips.", m.LevelTransition.NextLevel)
	case run.Retry:
		return "TRY AGAIN", "You ran out of chips on level 1.\n\nFree retry: both stacks reset to 500 chips."
	case run.Victory:
		return "YOU WON!", fmt.Sprintf("You defeated %s on level 4.", m.character().Name)
	}
	return "CONTINUE", "Ready for the next hand."
}

func (m *Model) realGameSummary(t Theme) string {
	s := m.Game.Snapshot(game.Human)
	return paint(" You ", t.Text, t.Panel, false) +
		paint(fmt.Sprintf("%d", s.Stacks[game.Human]), t.Good, t.Panel, true) +
		paint(" · Pot ", t.Text, t.Panel, false) +
		paint(fmt.Sprintf("%d", s.Pot), t.Accent, t.Panel, true) +
		paint(" · "+m.character().Name+" ", t.Text, t.Panel, false) +
		paint(fmt.Sprintf("%d", s.Stacks[game.Opponent]), t.Bad, t.Panel, true)
}

func (m *Model) phaseText() string {
	name := m.character().Name
	if m.opponentRevealPending && m.Game != nil && m.Game.Phase() == game.HandResult {
		return "Showdown… " + name + " reveals her hand."
	}
	if m.Game == nil {
		return map[string]string{
			"new":      "Your hand",
			"hold":     "Choose cards to exchange",
			"bet":      "Your turn",
			"opponent": name + " is thinking…",
			"win":      "You win the hand",
			"loss":     name + " wins the hand",
			"tie":      "Split pot",
			"match":    "Level result",
			"victory":  "You won!",
		}[m.Phase]
	}
	if m.Game.Actor() == game.Opponent && (m.Game.Phase() == game.Drawing || m.Game.Phase() == game.BettingBeforeDraw || m.Game.Phase() == game.BettingAfterDraw) {
		return name + " is thinking…"
	}
	switch m.Game.Phase() {
	case game.BettingBeforeDraw, game.BettingAfterDraw:
		return "Your turn"
	case game.Drawing:
		return "Choose cards to exchange"
	case game.HandResult:
		r := m.Game.Result()
		if r.Folded {
			if r.Winner == game.Human {
				return "You win — " + name + " folded"
			}
			return "You folded"
		}
		if r.Tie {
			return "Split pot"
		}
		if r.Winner == game.Human {
			return "You win — " + r.HumanRank.Category.String()
		}
		return name + " wins — " + r.OpponentRank.Category.String()
	default:
		return "Midnight Royale"
	}
}

func (m *Model) cancelOpponent() {
	if m.opponentCancel != nil {
		m.opponentCancel()
		m.opponentCancel = nil
	}
}

func (m *Model) activateRealGame() {
	if m.opponentRevealPending {
		// The hand has settled but the opponent's cards are still hidden behind
		// the reveal pause; swallow advance input so the player cannot skip to
		// the next hand before seeing the result.
		return
	}
	if !m.DrawerOpen || m.Game == nil || m.dealReveal {
		return
	}
	var err error
	switch m.Game.Phase() {
	case game.Drawing:
		if m.Game.Actor() != game.Human {
			return
		}
		if m.Focus == "cards" {
			m.toggleDiscard(m.SelectedCard)
			return
		}
		err = m.applyHumanDraw(m.selectedDiscards())
	case game.BettingBeforeDraw, game.BettingAfterDraw:
		if m.Game.Actor() != game.Human {
			return
		}
		if m.Focus == "bet-plus" {
			m.adjustBetAmount(1)
			return
		}
		if m.Focus == "bet-minus" {
			m.adjustBetAmount(-1)
			return
		}
		if action, ok := m.selectedRealAction(); ok {
			err = m.applyHumanAction(action, m.BetAmount)
		}
	case game.HandResult:
		if m.Showdown != nil {
			m.showdownActivate()
			return
		}
		if m.Game.Result().MatchOver {
			m.resolveLevelEnd()
			return
		}
		err = m.nextRealHand()
	}
	if err != nil {
		m.Notice = err.Error()
	}
}

func (m *Model) makeOpponentCommand(epoch int, token uint64) tea.Cmd {
	view := m.decisionView()
	bot := m.Bot
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelOpponent()
	m.opponentCancel = cancel
	return func() tea.Msg {
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return opponentMsg{Epoch: epoch, Token: token, Err: ctx.Err()}
		case <-timer.C:
		}
		msg := opponentMsg{Epoch: epoch, Token: token}
		if view.Phase == game.Drawing {
			msg.IsDraw = true
			msg.Draw, msg.Err = bot.ChooseDraw(ctx, view)
		} else {
			msg.Action, msg.Amount, msg.Err = bot.ChooseBet(ctx, view)
		}
		return msg
	}
}

func (m *Model) applyOpponentMessage(msg opponentMsg) {
	m.opponentPending = false
	m.opponentCancel = nil
	if msg.Err != nil || m.Game == nil || msg.Epoch != m.opponentEpoch || msg.Token != m.Game.Token() || m.Screen != "game" || m.Modal != "" || m.Game.Actor() != game.Opponent {
		return
	}
	var err error
	if msg.IsDraw {
		err = m.Game.Draw(game.Opponent, msg.Draw, msg.Token)
	} else {
		err = m.Game.Act(game.Opponent, msg.Action, msg.Amount, msg.Token)
	}
	if err != nil {
		m.Notice = err.Error()
		return
	}
	m.syncEnginePresentation()
}
