// Package opponent contains bounded, local poker decisions. DecisionView is the
// information boundary: callers can provide only the bot's own hand and public
// table state (stacks, pot, to-call, public raise count, dealer, public draw
// count, and public betting history) — never the opponent's hidden hand,
// discard identities, deck order, or seed.
//
// Strategy is deliberately small and explicit. It separates hand evaluation
// (reused from game), draw decisions, value betting, bluff decisions, and the
// personality/skill parameters that shape them. Bluffing and calling are
// probabilities bounded by a difficulty band; no profile is forced to win or
// lose through hidden information.
package opponent

import (
	"context"
	"errors"
	"sort"

	"midnight-royale/internal/game"
)

// Difficulty is the coarse skill tier that shapes call/fold discipline and draw
// quality. Four tiers give sensible progression within the three player-facing
// bands (see Band).
type Difficulty string

const (
	Beginner  Difficulty = "beginner"
	Casual    Difficulty = "casual"
	Competent Difficulty = "competent"
	Confident Difficulty = "confident"
)

// Band is the coarse, player-facing difficulty label. It is fixed per character
// (never user-selectable) and is shown on the character card and introduction.
type Band string

const (
	BandEasy        Band = "easy"
	BandMedium      Band = "medium"
	BandChallenging Band = "challenging"
)

func (b Band) String() string {
	switch b {
	case BandEasy:
		return "Easy"
	case BandMedium:
		return "Medium"
	case BandChallenging:
		return "Challenging"
	}
	return "Medium"
}

// Personality is character-level variance, independent of skill: a more
// aggressive character is not automatically more skilled, and vice versa.
type Personality struct {
	Aggression int `json:"aggression"` // percentage: raise/aggressive lean
	Calling    int `json:"calling"`    // percentage: loose-call lean
	Bluffing   int `json:"bluffing"`   // percentage: base bluff lean
}

type Profile struct {
	CharacterID string      `json:"character_id"`
	Difficulty  Difficulty  `json:"difficulty"`
	Band        Band        `json:"band"`
	Personality Personality `json:"personality"`
}

// DefaultProfiles returns the fixed 36-character roster in intended challenge
// order (12 Grand Riviera, 12 Azure Palms, 12 Starlight Lodge). Order and profiles are stable through an attempt; selecting a
// character never changes their difficulty band.
func DefaultProfiles() []Profile {
	return []Profile{
		{"amy", Beginner, BandEasy, Personality{30, 55, 8}},
		{"chloe", Beginner, BandEasy, Personality{40, 55, 10}},
		{"ruby", Beginner, BandEasy, Personality{25, 50, 6}},
		{"jenny", Casual, BandEasy, Personality{35, 48, 8}},
		{"sophie", Casual, BandMedium, Personality{30, 42, 9}},
		{"vanessa", Casual, BandMedium, Personality{50, 44, 12}},
		{"candy", Competent, BandMedium, Personality{55, 42, 14}},
		{"roxanne", Competent, BandMedium, Personality{45, 38, 12}},
		{"tiffany", Competent, BandChallenging, Personality{38, 38, 13}},
		{"scarlett", Confident, BandChallenging, Personality{60, 34, 16}},
		{"victoria", Confident, BandChallenging, Personality{45, 32, 14}},
		{"electra", Confident, BandChallenging, Personality{55, 34, 18}},
		{"amber", Beginner, BandEasy, Personality{32, 56, 9}},
		{"bianca", Beginner, BandEasy, Personality{38, 52, 10}},
		{"celeste", Beginner, BandEasy, Personality{27, 51, 7}},
		{"elena", Casual, BandEasy, Personality{36, 47, 9}},
		{"leila", Casual, BandMedium, Personality{31, 43, 8}},
		{"maren", Casual, BandMedium, Personality{48, 45, 11}},
		{"marina", Competent, BandMedium, Personality{54, 41, 13}},
		{"nadia", Competent, BandMedium, Personality{46, 39, 11}},
		{"priya", Competent, BandChallenging, Personality{39, 37, 14}},
		{"simone", Confident, BandChallenging, Personality{58, 33, 15}},
		{"talia", Confident, BandChallenging, Personality{44, 31, 13}},
		{"vivienne", Confident, BandChallenging, Personality{56, 33, 17}},
		{"sora", Beginner, BandEasy, Personality{34, 54, 7}},
		{"yasmin", Beginner, BandEasy, Personality{28, 55, 8}},
		{"amara", Beginner, BandEasy, Personality{36, 52, 9}},
		{"elise", Casual, BandEasy, Personality{39, 46, 9}},
		{"noura", Casual, BandMedium, Personality{32, 43, 8}},
		{"solveig", Casual, BandMedium, Personality{46, 42, 11}},
		{"leona", Competent, BandMedium, Personality{49, 40, 12}},
		{"valentina", Competent, BandMedium, Personality{53, 39, 13}},
		{"nadja", Competent, BandChallenging, Personality{40, 37, 14}},
		{"imani", Confident, BandChallenging, Personality{57, 34, 15}},
		{"samira", Confident, BandChallenging, Personality{47, 32, 14}},
		{"lucia", Confident, BandChallenging, Personality{59, 31, 17}},
	}
}

// ProfileFor returns the fixed profile for a character id.
func ProfileFor(id string) (Profile, bool) {
	for _, p := range DefaultProfiles() {
		if p.CharacterID == id {
			return p, true
		}
	}
	return Profile{}, false
}

type DecisionView struct {
	OwnHand           [5]game.Card        `json:"own_hand"`
	Phase             game.Phase          `json:"phase"`
	Stack             int                 `json:"stack"`
	OpponentStack     int                 `json:"opponent_stack"`
	Pot               int                 `json:"pot"`
	ToCall            int                 `json:"to_call"`
	MinBet            int                 `json:"min_bet"`
	MinRaiseTo        int                 `json:"min_raise_to"`
	MaxCommitTo       int                 `json:"max_commit_to"`
	Raises            int                 `json:"raises"`
	Dealer            bool                `json:"dealer"`
	OpponentDrawCount int                 `json:"opponent_draw_count"`
	History           []game.PublicAction `json:"history"`
	Legal             []game.Action       `json:"legal"`
}

type Bot struct {
	profile Profile
	rng     game.RNG
	eval    game.Evaluator
}

func New(profile Profile, rng game.RNG) *Bot {
	if rng == nil {
		rng = game.CryptoRNG{}
	}
	return &Bot{profile: profile, rng: rng, eval: game.InternalEvaluator{}}
}

// Profile returns the bot's fixed profile (used by tests and diagnostics).
func (b *Bot) Profile() Profile { return b.profile }

// ChooseDraw returns zero-to-five distinct indices to exchange. Premium made
// hands (straight or better) are never broken; duplicate ranks are retained;
// higher tiers chase four-flush and four-straight draws; early tiers may make a
// bounded mistake on genuinely uncertain high-card hands.
func (b *Bot) ChooseDraw(ctx context.Context, view DecisionView) ([]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if view.Phase != game.Drawing {
		return nil, errors.New("opponent: draw decision outside draw phase")
	}
	score, err := b.eval.Evaluate(view.OwnHand)
	if err != nil {
		return nil, err
	}
	if score.Category >= game.Straight {
		return nil, nil
	}
	counts := map[game.Rank][]int{}
	for i, card := range view.OwnHand {
		counts[card.Rank] = append(counts[card.Rank], i)
	}
	keep := [5]bool{}
	groups := make([]struct {
		rank  game.Rank
		index []int
	}, 0, len(counts))
	for rank, indexes := range counts {
		groups = append(groups, struct {
			rank  game.Rank
			index []int
		}{rank, indexes})
	}
	sort.Slice(groups, func(i, j int) bool {
		if len(groups[i].index) != len(groups[j].index) {
			return len(groups[i].index) > len(groups[j].index)
		}
		return groups[i].rank > groups[j].rank
	})
	if len(groups[0].index) >= 2 {
		for _, group := range groups {
			if len(group.index) >= 2 {
				for _, index := range group.index {
					keep[index] = true
				}
			}
		}
		return discardNotKept(keep), nil
	}
	switch b.profile.Difficulty {
	case Competent, Confident:
		if discard, ok := fourFlush(view.OwnHand); ok {
			return []int{discard}, nil
		}
		if discard, ok := fourStraight(view.OwnHand); ok {
			return []int{discard}, nil
		}
	case Casual:
		if discard, ok := fourFlush(view.OwnHand); ok {
			return []int{discard}, nil
		}
	}
	indexes := []int{0, 1, 2, 3, 4}
	sort.Slice(indexes, func(i, j int) bool { return view.OwnHand[indexes[i]].Rank > view.OwnHand[indexes[j]].Rank })
	keep[indexes[0]], keep[indexes[1]] = true, true
	// Bounded mistake on an uncertain high-card hand: early tiers occasionally
	// drop the second-best high card they would otherwise keep.
	if b.mistakesDraw() {
		keep[indexes[1]] = false
	}
	return discardNotKept(keep), nil
}

// mistakesDraw reports whether this tier commits the bounded draw mistake.
func (b *Bot) mistakesDraw() bool {
	switch b.profile.Difficulty {
	case Beginner:
		return b.roll(12)
	case Casual:
		return b.roll(6)
	}
	return false
}

func discardNotKept(keep [5]bool) []int {
	out := make([]int, 0, 5)
	for i := range keep {
		if !keep[i] {
			out = append(out, i)
		}
	}
	return out
}

func fourFlush(hand [5]game.Card) (int, bool) {
	counts := [4]int{}
	for _, card := range hand {
		counts[card.Suit]++
	}
	for suit, count := range counts {
		if count == 4 {
			for i, card := range hand {
				if int(card.Suit) != suit {
					return i, true
				}
			}
		}
	}
	return 0, false
}

func fourStraight(hand [5]game.Card) (int, bool) {
	for discard := 0; discard < 5; discard++ {
		present := map[int]bool{}
		for i, card := range hand {
			if i != discard {
				present[int(card.Rank)] = true
				if card.Rank == 14 {
					present[1] = true
				}
			}
		}
		for low := 1; low <= 10; low++ {
			matches := 0
			for rank := low; rank < low+5; rank++ {
				if present[rank] {
					matches++
				}
			}
			if matches == 4 {
				return discard, true
			}
		}
	}
	return 0, false
}

// ChooseBet returns an action plus a "bet to"/"raise to" total for Bet and
// Raise. The amount is ignored for Check, Call, and Fold.
func (b *Bot) ChooseBet(ctx context.Context, view DecisionView) (game.Action, int, error) {
	if err := ctx.Err(); err != nil {
		return game.Check, 0, err
	}
	if view.Phase != game.BettingBeforeDraw && view.Phase != game.BettingAfterDraw {
		return game.Check, 0, errors.New("opponent: betting decision outside betting phase")
	}
	if len(view.Legal) == 0 {
		return game.Check, 0, errors.New("opponent: no legal action")
	}
	score, err := b.eval.Evaluate(view.OwnHand)
	if err != nil {
		return game.Check, 0, err
	}
	cat := score.Category
	strong := cat >= game.ThreeOfAKind
	medium := cat >= game.OnePair

	if view.ToCall == 0 {
		// Open position: check, value-bet, or bluff.
		if hasAction(view.Legal, game.Bet) {
			if cat >= game.OnePair {
				if b.missesValueBet() {
					return firstLegal(view.Legal, game.Check), 0, nil
				}
				return game.Bet, b.valueBetSize(cat, view), nil
			}
			if b.rollsBluff() {
				return game.Bet, b.bluffBetSize(view), nil
			}
		}
		return firstLegal(view.Legal, game.Check), 0, nil
	}

	// Facing a bet.
	if strong {
		if hasAction(view.Legal, game.Raise) && b.roll(b.profile.Personality.Aggression) {
			return game.Raise, b.raiseSize(cat, view), nil
		}
		return firstLegal(view.Legal, game.Call), 0, nil
	}

	if b.shouldCall(medium, view) {
		return firstLegal(view.Legal, game.Call), 0, nil
	}
	// Raise-bluff: later opponents occasionally turn a fold into a selective
	// bluff raise when the pot is still small and the player has not shown
	// aggression this hand.
	if b.canRaiseBluff(view) {
		return game.Raise, b.bluffRaiseSize(view), nil
	}
	return firstLegal(view.Legal, game.Fold), 0, nil
}

// missesValueBet implements the documented "missed value bet" mistake for early
// tiers: they occasionally check a value hand instead of betting it.
func (b *Bot) missesValueBet() bool {
	switch b.profile.Difficulty {
	case Beginner:
		return b.roll(20)
	case Casual:
		return b.roll(8)
	}
	return false
}

// rollsBluff reports whether the bot bluffs when it opens with a weak hand. The
// base personality bluff rate is scaled down for early tiers and up for later
// tiers, but always stays bounded (never constant).
func (b *Bot) rollsBluff() bool {
	mult := 1.0
	switch b.profile.Difficulty {
	case Beginner:
		mult = 0.5
	case Casual:
		mult = 0.8
	case Competent:
		mult = 1.1
	case Confident:
		mult = 1.3
	}
	chance := int(float64(b.profile.Personality.Bluffing) * mult)
	return b.roll(chance)
}

// valueBetSize scales an opening value bet by hand strength, with a bounded
// sizing jitter for early tiers.
func (b *Bot) valueBetSize(cat game.Category, view DecisionView) int {
	var base int
	switch {
	case cat >= game.FullHouse:
		base = view.MaxCommitTo
	case cat >= game.Straight:
		base = view.MinBet * 8
	case cat >= game.ThreeOfAKind:
		base = view.MinBet * 5
	case cat >= game.TwoPair:
		base = view.MinBet * 3
	default:
		base = view.MinBet * 2
	}
	base = b.jitterSize(base, view)
	return min(view.MaxCommitTo, max(view.MinBet, base))
}

// jitterSize applies a bounded ±1-step sizing mistake for early tiers.
func (b *Bot) jitterSize(base int, view DecisionView) int {
	chance := 0
	switch b.profile.Difficulty {
	case Beginner:
		chance = 30
	case Casual:
		chance = 15
	}
	if chance > 0 && b.roll(chance) {
		step := view.MinBet
		if b.rngCoin() {
			base += step
		} else {
			base -= step
		}
	}
	return base
}

// bluffBetSize sizes a weak-hand opening bluff. Later tiers occasionally bluff
// bolder, but an open bluff is never an all-in.
func (b *Bot) bluffBetSize(view DecisionView) int {
	size := view.MinBet * 2
	if b.profile.Difficulty == Confident && b.roll(30) {
		size = view.MinBet * 3
	}
	return min(view.MaxCommitTo, max(view.MinBet, size))
}

// shouldCall decides whether a non-strong hand calls a bet. Early tiers call
// pairs too much (paying off stronger hands); later tiers are price-aware but
// less easily intimidated by raw size. A weak-hand call requires a cheap price.
// Later tiers also apply a bounded suspicion adjustment when the player drew
// four or more cards and then bet (public, observable exchange count).
func (b *Bot) shouldCall(medium bool, view DecisionView) bool {
	// A short caller risks only its remaining stack. The opponent's excess
	// will be refunded by the engine and cannot be won, so exclude it from
	// the price calculation too. Strategy thresholds/probabilities stay intact.
	toCall := min(view.ToCall, view.Stack)
	pot := view.Pot - (view.ToCall - toCall)
	if b.profile.Difficulty == Competent || b.profile.Difficulty == Confident {
		// Weak public exchange signal: a player who drew 4+ cards and then bet
		// may be improved or may be bluffing; widen the call slightly, bounded.
		if view.Phase == game.BettingAfterDraw && view.OpponentDrawCount >= 4 {
			pot = pot * 6 / 5
		}
	}
	if medium {
		switch b.profile.Difficulty {
		case Beginner:
			return toCall <= 3*pot || b.roll(b.profile.Personality.Calling/2)
		case Casual:
			return toCall <= 2*pot || b.roll(b.profile.Personality.Calling/3)
		case Competent:
			return toCall <= pot || b.roll(b.profile.Personality.Calling/4)
		case Confident:
			return toCall <= pot || (toCall <= 2*pot && b.roll(40))
		}
	}
	switch b.profile.Difficulty {
	case Beginner:
		return toCall*2 <= max(1, pot) || b.roll(b.profile.Personality.Calling/4)
	case Casual:
		return toCall*2 <= max(1, pot) || b.roll(b.profile.Personality.Calling/6)
	case Competent:
		return toCall*3 <= max(1, pot) || b.roll(b.profile.Personality.Calling/8)
	case Confident:
		return toCall*3 <= max(1, pot) || b.roll(b.profile.Personality.Calling/10)
	}
	return false
}

// canRaiseBluff reports whether the bot may turn a fold into a bluff raise.
// Only later tiers do this, only in small pots, and not into a player who has
// already bet or raised this hand.
func (b *Bot) canRaiseBluff(view DecisionView) bool {
	if b.profile.Difficulty != Competent && b.profile.Difficulty != Confident {
		return false
	}
	if !hasAction(view.Legal, game.Raise) {
		return false
	}
	if view.Pot > 6*view.MinBet || view.ToCall > 2*view.MinBet {
		return false
	}
	if playerShownAggression(view.History) {
		return false
	}
	chance := b.profile.Personality.Bluffing / 4
	return b.roll(chance)
}

// bluffRaiseSize sizes a bluff raise; a real raise but never an all-in.
func (b *Bot) bluffRaiseSize(view DecisionView) int {
	extra := view.MinBet
	if b.profile.Difficulty == Confident {
		extra = view.MinBet * 3
	}
	return min(view.MaxCommitTo, view.MinRaiseTo+extra)
}

// playerShownAggression reports whether the human has already bet or raised in
// the current hand, from public betting history only.
func playerShownAggression(history []game.PublicAction) bool {
	for _, action := range history {
		if action.Player == game.Human && (action.Action == game.Bet || action.Action == game.Raise) {
			return true
		}
	}
	return false
}

// raiseSize picks a raise-to amount; the minimum full raise unless the hand is
// strong enough to justify more, up to all-in.
func (b *Bot) raiseSize(cat game.Category, view DecisionView) int {
	switch {
	case cat >= game.FullHouse:
		return view.MaxCommitTo
	case cat >= game.ThreeOfAKind:
		return min(view.MaxCommitTo, view.MinRaiseTo+view.MinBet*4)
	default:
		return min(view.MaxCommitTo, view.MinRaiseTo)
	}
}

func (b *Bot) roll(percent int) bool {
	value, err := b.rng.Intn(100)
	return err == nil && value < max(0, min(100, percent))
}

// rngCoin is a 50/50 draw from the bot's own randomness, separate from the deck.
func (b *Bot) rngCoin() bool {
	return b.roll(50)
}

func hasAction(actions []game.Action, want game.Action) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}

func firstLegal(actions []game.Action, preferred game.Action) game.Action {
	if hasAction(actions, preferred) {
		return preferred
	}
	return actions[0]
}
