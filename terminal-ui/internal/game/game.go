package game

import (
	"errors"
	"fmt"
	"slices"
)

type Player uint8

const (
	Human Player = iota
	Opponent
)

func (p Player) String() string {
	if p == Human {
		return "human"
	}
	return "opponent"
}

func Other(p Player) Player          { return Player(1 - p) }
func NonDealer(dealer Player) Player { return Other(dealer) }

type Phase uint8

const (
	NotStarted Phase = iota
	AnteDeal
	BettingBeforeDraw
	Drawing
	BettingAfterDraw
	Showdown
	HandResult
	MatchResult
)

func (p Phase) String() string {
	return [...]string{"not started", "ante/deal", "first betting", "draw", "second betting", "showdown", "hand result", "match result"}[p]
}

type Action uint8

const (
	Check Action = iota
	Bet
	Call
	Raise
	Fold
)

func (a Action) String() string {
	return [...]string{"check", "bet", "call", "raise", "fold"}[a]
}

// PublicAction records one public betting action within the current hand. It is
// the only per-action history the engine exposes; it never carries hidden cards,
// discards, deck order, or any future state.
type PublicAction struct {
	Player Player `json:"player"`
	Action Action `json:"action"`
	Amount int    `json:"amount"` // "bet to"/"raise to" total for Bet/Raise; 0 otherwise
}

var (
	ErrPhase         = errors.New("game: action not legal in current phase")
	ErrTurn          = errors.New("game: not this player's turn")
	ErrIllegal       = errors.New("game: illegal action")
	ErrStaleAction   = errors.New("game: stale hand token")
	ErrDrawLimit     = errors.New("game: may exchange at most five cards")
	ErrDrawIndex     = errors.New("game: invalid or repeated draw index")
	ErrBelowMinBet   = errors.New("game: bet is below the minimum opening bet")
	ErrBelowMinRaise = errors.New("game: raise is below the minimum raise")
	ErrBetRange      = errors.New("game: bet exceeds the available stack")
)

// Config holds per-level game parameters. StartingStack and Ante are the
// approved shared rules; MinBet and BetStep are tuning defaults. The single
// source of truth for the starting stack lives here in DefaultConfig — no UI or
// engine code should duplicate the number.
type Config struct {
	StartingStack  int    `json:"starting_stack"`
	Ante           int    `json:"ante"`
	MinBet         int    `json:"min_bet"`
	BetStep        int    `json:"bet_step"`
	StartingDealer Player `json:"starting_dealer"`
}

// DefaultConfig is the approved 500-chip baseline: 500 chips per player at the
// start of every playable level, ante 25, minimum opening bet 50, and a 50-chip
// adjustment step. The 500-chip value supersedes the historical 1,000-chip
// stack (see docs/GAMEPLAY-RULES.md).
func DefaultConfig() Config {
	return Config{StartingStack: 500, Ante: 25, MinBet: 50, BetStep: 50, StartingDealer: Opponent}
}

type HandResultState struct {
	Settled      bool   `json:"settled"`
	Tie          bool   `json:"tie"`
	Folded       bool   `json:"folded"`
	Winner       Player `json:"winner"`
	Reason       string `json:"reason"`
	HumanRank    Score  `json:"human_rank"`
	OpponentRank Score  `json:"opponent_rank"`
	MatchOver    bool   `json:"match_over"`
}

type Snapshot struct {
	Token             uint64          `json:"token"`
	Phase             Phase           `json:"phase"`
	Dealer            Player          `json:"dealer"`
	Actor             Player          `json:"actor"`
	Stacks            [2]int          `json:"stacks"`
	Pot               int             `json:"pot"`
	Hand              [5]Card         `json:"hand"`
	RevealedOpponent  [5]Card         `json:"revealed_opponent,omitempty"`
	OpponentDrawCount int             `json:"opponent_draw_count"`
	ToCall            int             `json:"to_call"`
	Raises            int             `json:"raises"`
	History           []PublicAction  `json:"history"`
	Legal             []Action        `json:"legal"`
	Result            HandResultState `json:"result"`
}

type Game struct {
	cfg        Config
	rng        RNG
	evaluator  Evaluator
	phase      Phase
	dealer     Player
	actor      Player
	stacks     [2]int
	pot        int
	hands      [2][5]Card
	deck       []Card
	discards   [2][]Card
	drawn      [2]bool
	drawCounts [2]int
	round      [2]int
	checked    bool
	opened     bool
	raises     int
	lastRaise  int
	token      uint64
	settled    bool
	result     HandResultState
	history    []PublicAction
}

func New(cfg Config, rng RNG) (*Game, error) {
	if cfg.StartingStack < 0 || cfg.Ante < 0 || cfg.MinBet < 1 || cfg.BetStep < 1 || cfg.StartingDealer > Opponent {
		return nil, errors.New("game: invalid configuration")
	}
	if rng == nil {
		rng = CryptoRNG{}
	}
	return &Game{cfg: cfg, rng: rng, evaluator: InternalEvaluator{}, phase: NotStarted, dealer: cfg.StartingDealer, stacks: [2]int{cfg.StartingStack, cfg.StartingStack}}, nil
}

func (g *Game) StartHand() error {
	if g.phase != NotStarted {
		return ErrPhase
	}
	if g.stacks[Human] == 0 || g.stacks[Opponent] == 0 {
		g.phase = MatchResult
		return nil
	}
	g.token++
	g.phase = AnteDeal
	g.pot = 0
	g.hands = [2][5]Card{}
	g.discards = [2][]Card{}
	g.drawn = [2]bool{}
	g.drawCounts = [2]int{}
	g.result = HandResultState{}
	g.settled = false
	g.history = nil
	deck, err := ShuffledDeck(g.rng)
	if err != nil {
		g.phase = NotStarted
		return fmt.Errorf("game: shuffle: %w", err)
	}
	g.deck = deck
	ante := min(g.cfg.Ante, g.stacks[Human], g.stacks[Opponent])
	g.contribute(Human, ante)
	g.contribute(Opponent, ante)
	first := NonDealer(g.dealer)
	for i := 0; i < 5; i++ {
		for _, p := range []Player{first, Other(first)} {
			card, err := g.dealCard()
			if err != nil {
				return err
			}
			g.hands[p][i] = card
		}
	}
	if g.stacks[Human] == 0 || g.stacks[Opponent] == 0 {
		g.startDraw()
	} else {
		g.startBetting(BettingBeforeDraw)
	}
	return nil
}

func (g *Game) NextHand() error {
	if g.phase != HandResult {
		return ErrPhase
	}
	if g.result.MatchOver {
		g.phase = MatchResult
		return nil
	}
	g.dealer = Other(g.dealer)
	g.phase = NotStarted
	return g.StartHand()
}

func (g *Game) startBetting(phase Phase) {
	g.phase = phase
	g.actor = NonDealer(g.dealer)
	g.round = [2]int{}
	g.checked = false
	g.opened = false
	g.raises = 0
	g.lastRaise = 0
}

func (g *Game) startDraw() {
	g.phase = Drawing
	g.actor = NonDealer(g.dealer)
	g.drawn = [2]bool{}
}

func (g *Game) toCall(p Player) int {
	return max(0, g.round[Other(p)]-g.round[p])
}

// LegalActions returns the set of legal action types for the current actor.
// Bet/Raise amounts are validated separately (MinBet/MinRaiseTo/All-in).
func (g *Game) LegalActions(p Player) []Action {
	if p != g.actor || (g.phase != BettingBeforeDraw && g.phase != BettingAfterDraw) || g.settled {
		return nil
	}
	other := Other(p)
	toCall := g.toCall(p)
	if toCall == 0 {
		actions := []Action{Check}
		if g.stacks[p] > 0 && g.stacks[other] > 0 {
			actions = append(actions, Bet)
		}
		return actions
	}
	actions := []Action{Call, Fold}
	if g.stacks[p] > toCall && g.stacks[other] > 0 {
		actions = append(actions, Raise)
	}
	return actions
}

// MinRaiseTo returns the smallest total-this-round a full raise may reach.
func (g *Game) MinRaiseTo(p Player) int {
	return g.round[Other(p)] + g.lastRaise
}

// MaxCommitTo returns the largest total-this-round reachable (an all-in).
func (g *Game) MaxCommitTo(p Player) int {
	return g.round[p] + g.stacks[p]
}

func (g *Game) Act(p Player, action Action, amount int, token uint64) error {
	if token != g.token {
		return ErrStaleAction
	}
	if g.phase != BettingBeforeDraw && g.phase != BettingAfterDraw {
		return ErrPhase
	}
	if p != g.actor {
		return ErrTurn
	}
	if !slices.Contains(g.LegalActions(p), action) {
		return ErrIllegal
	}
	switch action {
	case Check:
		if g.checked {
			g.endBettingRound()
		} else {
			g.checked = true
			g.actor = Other(p)
		}
	case Bet:
		if err := g.validateBet(p, amount); err != nil {
			return err
		}
		g.opened = true
		g.checked = false
		g.lastRaise = amount
		g.contributeRound(p, amount)
		g.actor = Other(p)
	case Raise:
		if err := g.validateRaise(p, amount); err != nil {
			return err
		}
		raiseBy := amount - g.round[Other(p)]
		g.lastRaise = raiseBy
		g.raises++
		g.contributeRound(p, amount-g.round[p])
		g.actor = Other(p)
	case Call:
		g.contributeRound(p, min(g.toCall(p), g.stacks[p]))
		g.returnUnmatchedRoundExcess()
		g.endBettingRound()
	case Fold:
		g.settleFold(p)
	}
	g.history = append(g.history, PublicAction{Player: p, Action: action, Amount: publicAmount(action, amount)})
	return nil
}

func publicAmount(action Action, amount int) int {
	if action == Bet || action == Raise {
		return amount
	}
	return 0
}

func (g *Game) validateBet(p Player, amount int) error {
	if amount < g.cfg.MinBet && amount < g.stacks[p] {
		return ErrBelowMinBet
	}
	if amount > g.stacks[p] {
		return ErrBetRange
	}
	return nil
}

func (g *Game) validateRaise(p Player, amount int) error {
	other := Other(p)
	if amount <= g.round[other] {
		return ErrBelowMinRaise
	}
	if amount > g.round[p]+g.stacks[p] {
		return ErrBetRange
	}
	raiseBy := amount - g.round[other]
	if raiseBy < g.lastRaise && amount != g.round[p]+g.stacks[p] {
		return ErrBelowMinRaise
	}
	return nil
}

func (g *Game) endBettingRound() {
	if g.phase == BettingBeforeDraw {
		g.startDraw()
		return
	}
	g.phase = Showdown
	g.settleShowdown()
}

func (g *Game) Draw(p Player, indexes []int, token uint64) error {
	if token != g.token {
		return ErrStaleAction
	}
	if g.phase != Drawing {
		return ErrPhase
	}
	if p != g.actor {
		return ErrTurn
	}
	if len(indexes) > 5 {
		return ErrDrawLimit
	}
	seen := [5]bool{}
	for _, index := range indexes {
		if index < 0 || index >= 5 || seen[index] {
			return ErrDrawIndex
		}
		seen[index] = true
	}
	for _, index := range indexes {
		g.discards[p] = append(g.discards[p], g.hands[p][index])
		card, err := g.dealCard()
		if err != nil {
			return err
		}
		g.hands[p][index] = card
	}
	g.drawn[p] = true
	g.drawCounts[p] = len(indexes)
	other := Other(p)
	if !g.drawn[other] {
		g.actor = other
		return nil
	}
	if g.stacks[Human] == 0 || g.stacks[Opponent] == 0 {
		g.phase = Showdown
		g.settleShowdown()
	} else {
		g.startBetting(BettingAfterDraw)
	}
	return nil
}

func (g *Game) contribute(p Player, amount int) {
	amount = min(max(0, amount), g.stacks[p])
	g.stacks[p] -= amount
	g.pot += amount
}

func (g *Game) contributeRound(p Player, amount int) {
	amount = min(max(0, amount), g.stacks[p])
	g.contribute(p, amount)
	g.round[p] += amount
}

func (g *Game) returnUnmatchedRoundExcess() {
	if g.round[Human] == g.round[Opponent] {
		return
	}
	high := Human
	if g.round[Opponent] > g.round[Human] {
		high = Opponent
	}
	excess := g.round[high] - g.round[Other(high)]
	g.round[high] -= excess
	g.stacks[high] += excess
	g.pot -= excess
}

func (g *Game) settleFold(folder Player) {
	if g.settled {
		return
	}
	g.returnUnmatchedRoundExcess()
	winner := Other(folder)
	g.stacks[winner] += g.pot
	g.pot = 0
	g.settled = true
	g.result = HandResultState{Settled: true, Folded: true, Winner: winner, Reason: "fold"}
	g.result.MatchOver = g.stacks[Human] == 0 || g.stacks[Opponent] == 0
	g.phase = HandResult
}

func (g *Game) settleShowdown() {
	if g.settled {
		return
	}
	g.returnUnmatchedRoundExcess()
	human, errH := g.evaluator.Evaluate(g.hands[Human])
	opponent, errO := g.evaluator.Evaluate(g.hands[Opponent])
	if errH != nil || errO != nil {
		return
	}
	result := HandResultState{Settled: true, HumanRank: human, OpponentRank: opponent, Reason: "showdown"}
	switch Compare(human, opponent) {
	case -1:
		result.Winner = Opponent
		g.stacks[Opponent] += g.pot
	case 1:
		result.Winner = Human
		g.stacks[Human] += g.pot
	default:
		result.Tie = true
		half := g.pot / 2
		g.stacks[Human] += half
		g.stacks[Opponent] += half
		if g.pot%2 == 1 {
			g.stacks[NonDealer(g.dealer)]++
		}
	}
	g.pot = 0
	g.settled = true
	result.MatchOver = g.stacks[Human] == 0 || g.stacks[Opponent] == 0
	g.result = result
	g.phase = HandResult
}

func (g *Game) dealCard() (Card, error) {
	if len(g.deck) == 0 {
		return Card{}, errors.New("game: deck exhausted")
	}
	last := len(g.deck) - 1
	card := g.deck[last]
	g.deck = g.deck[:last]
	return card, nil
}

func (g *Game) Token() uint64              { return g.token }
func (g *Game) Phase() Phase               { return g.phase }
func (g *Game) Dealer() Player             { return g.dealer }
func (g *Game) Actor() Player              { return g.actor }
func (g *Game) Pot() int                   { return g.pot }
func (g *Game) Stacks() [2]int             { return g.stacks }
func (g *Game) Hand(p Player) [5]Card      { return g.hands[p] }
func (g *Game) Result() HandResultState    { return g.result }
func (g *Game) RoundContributions() [2]int { return g.round }
func (g *Game) TotalChips() int            { return g.stacks[Human] + g.stacks[Opponent] + g.pot }
func (g *Game) MinBet() int                { return g.cfg.MinBet }
func (g *Game) BetStep() int               { return g.cfg.BetStep }
func (g *Game) Ante() int                  { return g.cfg.Ante }
func (g *Game) ToCall(p Player) int        { return g.toCall(p) }
func (g *Game) StartingStack() int         { return g.cfg.StartingStack }

func (g *Game) Snapshot(viewer Player) Snapshot {
	s := Snapshot{Token: g.token, Phase: g.phase, Dealer: g.dealer, Actor: g.actor, Stacks: g.stacks, Pot: g.pot, Hand: g.hands[viewer], OpponentDrawCount: g.drawCounts[Other(viewer)], ToCall: g.toCall(viewer), Raises: g.raises, History: g.history, Legal: g.LegalActions(viewer), Result: g.result}
	if g.phase == HandResult || g.phase == MatchResult || g.phase == Showdown {
		s.RevealedOpponent = g.hands[Other(viewer)]
	}
	return s
}

func (g *Game) CardsUnique() bool {
	seen := make(map[Card]bool, 52)
	add := func(card Card) bool {
		if !card.Valid() || seen[card] {
			return false
		}
		seen[card] = true
		return true
	}
	for _, card := range g.deck {
		if !add(card) {
			return false
		}
	}
	for p := Human; p <= Opponent; p++ {
		for _, card := range g.hands[p] {
			if !add(card) {
				return false
			}
		}
		for _, card := range g.discards[p] {
			if !add(card) {
				return false
			}
		}
	}
	return len(seen) == 52
}

func (g *Game) forceForTest(human, opponent [5]Card, pot int) {
	delta := pot - g.pot
	g.stacks[g.dealer] -= delta
	g.pot = pot
	g.hands[Human] = human
	g.hands[Opponent] = opponent
	g.phase = Showdown
	g.settled = false
}

// ForceBustForFixture moves the game straight to a settled hand result where
// the given player is busted. Development fixtures only; normal play never
// calls this and never derives dealing randomness from it.
func (g *Game) ForceBustForFixture(p Player) {
	total := 2 * g.cfg.StartingStack
	g.stacks = [2]int{0, 0}
	g.stacks[Other(p)] = total
	g.pot = 0
	g.settled = true
	g.result = HandResultState{Settled: true, Winner: Other(p), Reason: "fixture", MatchOver: true}
	g.phase = HandResult
}

// ForceFoldForFixture moves the game straight to a settled fold result where
// the given player folded. Development fixtures only; normal play never calls
// this.
func (g *Game) ForceFoldForFixture(p Player) {
	g.settled = true
	g.result = HandResultState{Settled: true, Folded: true, Winner: Other(p), Reason: "fixture-fold"}
	g.phase = HandResult
}
