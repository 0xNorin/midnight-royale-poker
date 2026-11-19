// Package simulation runs deterministic, headless pacing studies. A "study"
// plays every character against a small set of fixed reference player policies
// (not claims about real humans) and reports per-hand and per-attempt pacing
// metrics: hands per level, fold rates by actor and phase, showdown fraction,
// pot-size distribution, all-in frequency, bot action distribution, and
// intentional bluff frequency. Single-level results and full multi-level
// attempts (with regression and free level-1 retries) are reported separately.
package simulation

import (
	"context"
	"errors"
	"sort"

	"midnight-royale/internal/game"
	"midnight-royale/internal/opponent"
)

// PlayerPolicy is a fixed reference player strategy, documented for tests and
// comparisons only. It is deliberately not a claim about how real people play.
type PlayerPolicy string

const (
	PolicyTight      PlayerPolicy = "tight"      // conservative: folds weak hands, never bluffs
	PolicyValue      PlayerPolicy = "value"      // straightforward: bets pairs+, calls fair prices
	PolicyLoose      PlayerPolicy = "loose"      // loose/passive: calls too much, rarely bets
	PolicyAggressive PlayerPolicy = "aggressive" // bets/raises often, bluffs with bounded frequency
)

func Policies() []PlayerPolicy {
	return []PlayerPolicy{PolicyTight, PolicyValue, PolicyLoose, PolicyAggressive}
}

// StudyConfig controls a pacing study. Profiles and Policies default to the
// full fixed roster and all four reference policies when empty. Stack overrides
// the shared starting stack when non-zero, so the same tooling can compare
// 1,000-chip and 500-chip baselines without changing the game defaults.
type StudyConfig struct {
	Profiles           []opponent.Profile
	Policies           []PlayerPolicy
	LevelMatches       int // single-level matches per (profile, policy)
	Attempts           int // full multi-level attempts per (profile, policy)
	MaxHandsPerLevel   int // cap for a single-level match
	MaxHandsPerAttempt int // cap for a full attempt
	Seed               int64
	Stack              int // 0 = use game.DefaultConfig().StartingStack
}

func (c *StudyConfig) applyDefaults() {
	if len(c.Profiles) == 0 {
		c.Profiles = opponent.DefaultProfiles()
	}
	if len(c.Policies) == 0 {
		c.Policies = Policies()
	}
	if c.LevelMatches < 1 {
		c.LevelMatches = 200
	}
	if c.Attempts < 1 {
		c.Attempts = 100
	}
	if c.MaxHandsPerLevel < 1 {
		c.MaxHandsPerLevel = 150
	}
	if c.MaxHandsPerAttempt < 1 {
		c.MaxHandsPerAttempt = 400
	}
}

func (c *StudyConfig) gameConfig() game.Config {
	cfg := game.DefaultConfig()
	if c.Stack > 0 {
		cfg.StartingStack = c.Stack
	}
	return cfg
}

// LevelMetrics aggregates single-level matches for one (character, policy).
type LevelMetrics struct {
	CharacterID string              `json:"character_id"`
	Difficulty  opponent.Difficulty `json:"difficulty"`
	Band        string              `json:"band"`
	Policy      PlayerPolicy        `json:"policy"`

	Matches   int `json:"matches"`
	Completed int `json:"completed"`
	Capped    int `json:"capped"`
	Hands     int `json:"hands"`

	HandsPerLevelMean   float64 `json:"hands_per_level_mean"`
	HandsPerLevelMedian float64 `json:"hands_per_level_median"`
	HandsPerLevelP90    float64 `json:"hands_per_level_p90"`

	BotLevelWins    int     `json:"bot_level_wins"`
	BotLevelWinRate float64 `json:"bot_level_win_rate"`

	Showdowns        int     `json:"showdowns"`
	ShowdownFraction float64 `json:"showdown_fraction"`

	FoldsHumanPre  int `json:"folds_human_pre"`
	FoldsHumanPost int `json:"folds_human_post"`
	FoldsBotPre    int `json:"folds_bot_pre"`
	FoldsBotPost   int `json:"folds_bot_post"`

	PotMean   float64 `json:"pot_mean"`
	PotMedian float64 `json:"pot_median"`
	PotP90    float64 `json:"pot_p90"`

	AllInHuman int `json:"allin_human"`
	AllInBot   int `json:"allin_bot"`

	BotCheck int `json:"bot_check"`
	BotBet   int `json:"bot_bet"`
	BotCall  int `json:"bot_call"`
	BotRaise int `json:"bot_raise"`
	BotFold  int `json:"bot_fold"`

	BotBluffs    int     `json:"bot_bluffs"`
	BotBluffRate float64 `json:"bot_bluff_rate"` // bluffs / aggressive (bet+raise) actions

	ChipConservationFailures int `json:"chip_conservation_failures"`
}

// AttemptMetrics aggregates full multi-level attempts for one (character, policy).
type AttemptMetrics struct {
	CharacterID string              `json:"character_id"`
	Difficulty  opponent.Difficulty `json:"difficulty"`
	Band        string              `json:"band"`
	Policy      PlayerPolicy        `json:"policy"`

	Attempts  int `json:"attempts"`
	Completed int `json:"completed"`
	Capped    int `json:"capped"`

	HandsToVictoryMean   float64 `json:"hands_to_victory_mean"`
	HandsToVictoryMedian float64 `json:"hands_to_victory_median"`
	HandsToVictoryP90    float64 `json:"hands_to_victory_p90"`

	Advances  int `json:"advances"`
	Retreats  int `json:"retreats"`
	Retries   int `json:"retries"`
	Victories int `json:"victories"`
}

// StudyResult is one row of a study: the level and attempt aggregates for a
// single (character, policy) pair.
type StudyResult struct {
	Level   LevelMetrics   `json:"level"`
	Attempt AttemptMetrics `json:"attempt"`
}

// RunStudy runs the full study and returns one result per (profile, policy).
// It is deterministic for a fixed seed, stack, and profile/policy set.
func RunStudy(ctx context.Context, cfg StudyConfig) ([]StudyResult, error) {
	cfg.applyDefaults()
	results := make([]StudyResult, 0, len(cfg.Profiles)*len(cfg.Policies))
	seed := cfg.Seed
	for _, profile := range cfg.Profiles {
		for _, policy := range cfg.Policies {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			level, err := runLevels(ctx, profile, policy, cfg, seed)
			if err != nil {
				return nil, err
			}
			attempt, err := runAttempts(ctx, profile, policy, cfg, seed+1)
			if err != nil {
				return nil, err
			}
			results = append(results, StudyResult{Level: level, Attempt: attempt})
			seed += 7919
		}
	}
	return results, nil
}

// levelSample is the raw per-level data collected before aggregation.
type levelSample struct {
	hands                                                    int
	completed                                                bool
	botWon                                                   bool
	pots                                                     []int
	foldsHumanPre, foldsHumanPost, foldsBotPre, foldsBotPost int
	showdowns                                                int
	allinHuman, allinBot                                     int
	botCheck, botBet, botCall, botRaise, botFold             int
	botBluffs, botAggressive                                 int
	chipFailures                                             int
}

func runLevels(ctx context.Context, profile opponent.Profile, policy PlayerPolicy, cfg StudyConfig, seed int64) (LevelMetrics, error) {
	gcfg := cfg.gameConfig()
	m := LevelMetrics{
		CharacterID: profile.CharacterID,
		Difficulty:  profile.Difficulty,
		Band:        profile.Band.String(),
		Policy:      policy,
		Matches:     cfg.LevelMatches,
	}
	var handsPerLevel []int
	var pots []int
	for match := 0; match < cfg.LevelMatches; match++ {
		if err := ctx.Err(); err != nil {
			return m, err
		}
		s, err := playLevel(ctx, profile, policy, gcfg, seed+int64(match)*104729)
		if err != nil {
			return m, err
		}
		m.Hands += s.hands
		if s.completed {
			m.Completed++
			if s.botWon {
				m.BotLevelWins++
			}
			handsPerLevel = append(handsPerLevel, s.hands)
		} else {
			m.Capped++
		}
		m.Showdowns += s.showdowns
		m.FoldsHumanPre += s.foldsHumanPre
		m.FoldsHumanPost += s.foldsHumanPost
		m.FoldsBotPre += s.foldsBotPre
		m.FoldsBotPost += s.foldsBotPost
		m.AllInHuman += s.allinHuman
		m.AllInBot += s.allinBot
		m.BotCheck += s.botCheck
		m.BotBet += s.botBet
		m.BotCall += s.botCall
		m.BotRaise += s.botRaise
		m.BotFold += s.botFold
		m.BotBluffs += s.botBluffs
		m.ChipConservationFailures += s.chipFailures
		pots = append(pots, s.pots...)
	}
	m.HandsPerLevelMean, m.HandsPerLevelMedian, m.HandsPerLevelP90 = meanMedianP90(handsPerLevel)
	if m.Completed > 0 {
		m.BotLevelWinRate = float64(m.BotLevelWins) / float64(m.Completed)
	}
	m.ShowdownFraction = ratio(m.Showdowns, m.Hands)
	m.PotMean, m.PotMedian, m.PotP90 = meanMedianP90(pots)
	m.BotBluffRate = ratio(m.BotBluffs, m.BotBet+m.BotRaise)
	return m, nil
}

func runAttempts(ctx context.Context, profile opponent.Profile, policy PlayerPolicy, cfg StudyConfig, seed int64) (AttemptMetrics, error) {
	gcfg := cfg.gameConfig()
	a := AttemptMetrics{
		CharacterID: profile.CharacterID,
		Difficulty:  profile.Difficulty,
		Band:        profile.Band.String(),
		Policy:      policy,
		Attempts:    cfg.Attempts,
	}
	var handsToVictory []int
	for attempt := 0; attempt < cfg.Attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return a, err
		}
		hands, completed, adv, ret, retry, vic, err := playAttempt(ctx, profile, policy, gcfg, seed+int64(attempt)*69997)
		if err != nil {
			return a, err
		}
		if completed > 0 {
			a.Completed++
			handsToVictory = append(handsToVictory, hands)
		} else {
			a.Capped++
		}
		a.Advances += adv
		a.Retreats += ret
		a.Retries += retry
		a.Victories += vic
	}
	a.HandsToVictoryMean, a.HandsToVictoryMedian, a.HandsToVictoryP90 = meanMedianP90(handsToVictory)
	return a, nil
}

// playLevel plays one level (play until a bust or the hand cap) and returns the
// raw sample. The bot is the opponent; the reference policy is the human.
func playLevel(ctx context.Context, profile opponent.Profile, policy PlayerPolicy, gcfg game.Config, seed int64) (levelSample, error) {
	s := levelSample{}
	g, err := game.New(gcfg, game.NewSeededRNG(seed))
	if err != nil {
		return s, err
	}
	bot := opponent.New(profile, game.NewSeededRNG(seed+1))
	playerRNG := game.NewSeededRNG(seed + 2)
	total := gcfg.StartingStack * 2
	if err := g.StartHand(); err != nil {
		return s, err
	}
	handMaxPot := 0
	for s.hands < 150 {
		if err := ctx.Err(); err != nil {
			return s, err
		}
		switch g.Phase() {
		case game.BettingBeforeDraw, game.BettingAfterDraw:
			pre := g.Phase()
			actor := g.Actor()
			var action game.Action
			var amount int
			if actor == game.Human {
				action, amount = playerBet(policy, g, playerRNG)
			} else {
				action, amount, err = bot.ChooseBet(ctx, botView(g))
				if err != nil {
					return s, err
				}
			}
			handMaxPot = max(handMaxPot, g.Pot())
			if action == game.Fold {
				if actor == game.Human {
					if pre == game.BettingBeforeDraw {
						s.foldsHumanPre++
					} else {
						s.foldsHumanPost++
					}
				} else {
					if pre == game.BettingBeforeDraw {
						s.foldsBotPre++
					} else {
						s.foldsBotPost++
					}
				}
			}
			if actor == game.Opponent {
				recordBotAction(&s, g, action, amount)
			} else if (action == game.Bet || action == game.Raise) && amount == g.MaxCommitTo(game.Human) {
				s.allinHuman++
			}
			if err := g.Act(actor, action, amount, g.Token()); err != nil {
				return s, err
			}
		case game.Drawing:
			actor := g.Actor()
			handMaxPot = max(handMaxPot, g.Pot())
			if actor == game.Human {
				if err := g.Draw(game.Human, playerDraw(policy, g.Hand(game.Human), playerRNG), g.Token()); err != nil {
					return s, err
				}
			} else {
				discard, err := bot.ChooseDraw(ctx, botView(g))
				if err != nil {
					return s, err
				}
				if err := g.Draw(game.Opponent, discard, g.Token()); err != nil {
					return s, err
				}
			}
		case game.HandResult:
			s.hands++
			if g.Result().Reason == "showdown" {
				s.showdowns++
			}
			if g.TotalChips() != total {
				s.chipFailures++
			}
			s.pots = append(s.pots, handMaxPot)
			handMaxPot = 0
			if g.Result().MatchOver {
				s.completed = true
				s.botWon = g.Stacks()[game.Opponent] > 0
				return s, nil
			}
			if err := g.NextHand(); err != nil {
				return s, err
			}
		case game.MatchResult:
			s.completed = true
			s.botWon = g.Stacks()[game.Opponent] > 0
			return s, nil
		default:
			return s, errors.New("simulation: unexpected game phase")
		}
	}
	return s, nil
}

// playAttempt plays a full run from level 1 until victory (image 5) or the
// attempt hand cap, counting forward transitions and victories. Because a
// player bust moves back a level and a level-1 bust retries for free, a full
// attempt can regress; the reference policies below rarely bust, but the
// accounting is honest about every transition that occurs.
func playAttempt(ctx context.Context, profile opponent.Profile, policy PlayerPolicy, gcfg game.Config, seed int64) (hands, completed, advances, retreats, retries, victories int, err error) {
	level := 1
	seedBase := seed
	for hands < 400 {
		if err := ctx.Err(); err != nil {
			return hands, completed, advances, retreats, retries, victories, err
		}
		h, opponentBusted, capped, err := playLevelBounded(ctx, profile, policy, gcfg, seedBase, 400-hands)
		if err != nil {
			return hands, completed, advances, retreats, retries, victories, err
		}
		hands += h
		if capped {
			// Level never reached a bust within the remaining budget.
			return hands, completed, advances, retreats, retries, victories, nil
		}
		if !opponentBusted {
			// Player busted: retreat (or free retry on level 1).
			if level <= 1 {
				retries++
				level = 1
			} else {
				retreats++
				level--
			}
			seedBase += 1000003
			continue
		}
		// opponentBusted: the player won the level.
		if level >= 4 {
			victories++
			completed++
			return hands, completed, advances, retreats, retries, victories, nil
		}
		advances++
		level++
		seedBase += 1000003
	}
	return hands, completed, advances, retreats, retries, victories, nil
}

// playLevelBounded plays one level, returning (hands, opponentBusted, capped).
// opponentBusted is true when the opponent busted (player won the level); false
// when the player busted or the level capped. capped is true only when the hand
// budget ran out without a bust.
func playLevelBounded(ctx context.Context, profile opponent.Profile, policy PlayerPolicy, gcfg game.Config, seed int64, maxHands int) (int, bool, bool, error) {
	g, err := game.New(gcfg, game.NewSeededRNG(seed))
	if err != nil {
		return 0, false, false, err
	}
	bot := opponent.New(profile, game.NewSeededRNG(seed+1))
	playerRNG := game.NewSeededRNG(seed + 2)
	if err := g.StartHand(); err != nil {
		return 0, false, false, err
	}
	hands := 0
	for hands < maxHands {
		if err := ctx.Err(); err != nil {
			return 0, false, false, err
		}
		switch g.Phase() {
		case game.BettingBeforeDraw, game.BettingAfterDraw:
			actor := g.Actor()
			var action game.Action
			var amount int
			if actor == game.Human {
				action, amount = playerBet(policy, g, playerRNG)
			} else {
				action, amount, err = bot.ChooseBet(ctx, botView(g))
				if err != nil {
					return 0, false, false, err
				}
			}
			if err := g.Act(actor, action, amount, g.Token()); err != nil {
				return 0, false, false, err
			}
		case game.Drawing:
			actor := g.Actor()
			if actor == game.Human {
				if err := g.Draw(game.Human, playerDraw(policy, g.Hand(game.Human), playerRNG), g.Token()); err != nil {
					return 0, false, false, err
				}
			} else {
				discard, err := bot.ChooseDraw(ctx, botView(g))
				if err != nil {
					return 0, false, false, err
				}
				if err := g.Draw(game.Opponent, discard, g.Token()); err != nil {
					return 0, false, false, err
				}
			}
		case game.HandResult:
			hands++
			if g.Result().MatchOver {
				return hands, g.Stacks()[game.Opponent] == 0, false, nil
			}
			if err := g.NextHand(); err != nil {
				return 0, false, false, err
			}
		case game.MatchResult:
			return hands, false, false, nil
		default:
			return 0, false, false, errors.New("simulation: unexpected phase in attempt")
		}
	}
	return hands, false, false, nil
}

func recordBotAction(s *levelSample, g *game.Game, action game.Action, amount int) {
	switch action {
	case game.Check:
		s.botCheck++
	case game.Bet:
		s.botBet++
		s.botAggressive++
		if amount == g.MaxCommitTo(game.Opponent) {
			s.allinBot++
		}
		if isBluff(g) {
			s.botBluffs++
		}
	case game.Call:
		s.botCall++
	case game.Raise:
		s.botRaise++
		s.botAggressive++
		if amount == g.MaxCommitTo(game.Opponent) {
			s.allinBot++
		}
		if isBluff(g) {
			s.botBluffs++
		}
	case game.Fold:
		s.botFold++
	}
}

// isBluff reports whether the bot's current decision is a bluff under the
// documented definition: a Bet or Raise with a high-card hand (no pair or
// better). This is the same definition used for the strategy tests.
func isBluff(g *game.Game) bool {
	score, err := (game.InternalEvaluator{}).Evaluate(g.Hand(game.Opponent))
	if err != nil {
		return false
	}
	return score.Category == game.HighCard
}

// playerBet is a fixed reference betting policy. All four are documented in the
// study docs; none is a claim about human behavior.
func playerBet(p PlayerPolicy, g *game.Game, rng game.RNG) (game.Action, int) {
	s := g.Snapshot(game.Human)
	score, _ := (game.InternalEvaluator{}).Evaluate(s.Hand)
	legal := s.Legal
	stack := g.Stacks()[game.Human]
	switch p {
	case PolicyTight:
		if has(legal, game.Check) {
			if score.Category >= game.TwoPair && has(legal, game.Bet) {
				return game.Bet, min(g.MinBet(), stack)
			}
			return game.Check, 0
		}
		if has(legal, game.Call) && score.Category >= game.TwoPair {
			return game.Call, 0
		}
		if has(legal, game.Call) && score.Category >= game.OnePair && s.ToCall*4 <= max(1, s.Pot) {
			return game.Call, 0
		}
		if has(legal, game.Fold) {
			return game.Fold, 0
		}
	case PolicyValue:
		if has(legal, game.Check) {
			if score.Category >= game.OnePair && has(legal, game.Bet) {
				return game.Bet, min(g.MinBet(), stack)
			}
			return game.Check, 0
		}
		if score.Category >= game.ThreeOfAKind && has(legal, game.Raise) && roll(rng, 40) {
			return game.Raise, min(g.MinRaiseTo(game.Human), g.MaxCommitTo(game.Human))
		}
		if has(legal, game.Call) && (score.Category >= game.OnePair || s.ToCall*3 <= max(1, s.Pot)) {
			return game.Call, 0
		}
		if has(legal, game.Fold) {
			return game.Fold, 0
		}
	case PolicyLoose:
		if has(legal, game.Check) {
			if score.Category >= game.TwoPair && has(legal, game.Bet) && roll(rng, 50) {
				return game.Bet, min(g.MinBet(), stack)
			}
			return game.Check, 0
		}
		if has(legal, game.Call) && (score.Category >= game.HighCard || s.ToCall <= s.Pot) {
			return game.Call, 0
		}
		if has(legal, game.Fold) {
			return game.Fold, 0
		}
	case PolicyAggressive:
		if has(legal, game.Check) {
			if has(legal, game.Bet) && (score.Category >= game.OnePair || roll(rng, 25)) {
				size := g.MinBet() * 3
				if size < g.MinBet() {
					size = g.MinBet()
				}
				return game.Bet, min(stack, size)
			}
			return game.Check, 0
		}
		if has(legal, game.Raise) && (score.Category >= game.TwoPair || roll(rng, 15)) {
			return game.Raise, min(g.MaxCommitTo(game.Human), g.MinRaiseTo(game.Human)+g.MinBet())
		}
		if has(legal, game.Call) && (score.Category >= game.OnePair || s.ToCall*2 <= max(1, s.Pot)) {
			return game.Call, 0
		}
		if has(legal, game.Fold) {
			return game.Fold, 0
		}
	}
	return legal[0], 0
}

// playerDraw is the reference draw policy. It keeps made straights or better
// and duplicate ranks, and otherwise discards down to two high cards (tight and
// value also chase a four-flush; aggressive chases four-flush and four-straight;
// loose sometimes clings to a third card).
func playerDraw(p PlayerPolicy, hand [5]game.Card, rng game.RNG) []int {
	score, _ := (game.InternalEvaluator{}).Evaluate(hand)
	if score.Category >= game.Straight {
		return nil
	}
	counts := map[game.Rank][]int{}
	for i, card := range hand {
		counts[card.Rank] = append(counts[card.Rank], i)
	}
	keep := [5]bool{}
	for _, idx := range counts {
		if len(idx) >= 2 {
			for _, i := range idx {
				keep[i] = true
			}
		}
	}
	if p == PolicyAggressive {
		if idx, ok := fourFlushIdx(hand); ok {
			return keepAllBut(hand, idx)
		}
		if idx, ok := fourStraightIdx(hand); ok {
			return keepAllBut(hand, idx)
		}
	}
	if p == PolicyValue || p == PolicyTight {
		if idx, ok := fourFlushIdx(hand); ok {
			return keepAllBut(hand, idx)
		}
	}
	kept := 0
	for _, v := range keep {
		if v {
			kept++
		}
	}
	if kept == 0 {
		order := []int{0, 1, 2, 3, 4}
		sort.Slice(order, func(i, j int) bool { return hand[order[i]].Rank > hand[order[j]].Rank })
		keep[order[0]] = true
		keep[order[1]] = true
		if p == PolicyLoose && roll(rng, 20) {
			keep[order[2]] = true // loose players sometimes cling to a third card
		}
	}
	return discardNotKept(keep)
}

func keepAllBut(hand [5]game.Card, discard int) []int {
	keep := [5]bool{}
	for i := range hand {
		if i != discard {
			keep[i] = true
		}
	}
	return discardNotKept(keep)
}

func fourFlushIdx(hand [5]game.Card) (int, bool) {
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

func fourStraightIdx(hand [5]game.Card) (int, bool) {
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

func discardNotKept(keep [5]bool) []int {
	out := make([]int, 0, 5)
	for i, v := range keep {
		if !v {
			out = append(out, i)
		}
	}
	return out
}

func roll(rng game.RNG, percent int) bool {
	v, err := rng.Intn(100)
	return err == nil && v < max(0, min(100, percent))
}

func has(actions []game.Action, want game.Action) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}

func ratio(num, den int) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func meanMedianP90(values []int) (mean, median, p90 float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	var sum int64
	for _, v := range sorted {
		sum += int64(v)
	}
	mean = float64(sum) / float64(len(sorted))
	n := len(sorted)
	if n%2 == 1 {
		median = float64(sorted[n/2])
	} else {
		median = float64(sorted[n/2-1]+sorted[n/2]) / 2
	}
	idx := int(float64(n)*0.90 + 0.5)
	if idx < 1 {
		idx = 1
	}
	if idx > n {
		idx = n
	}
	p90 = float64(sorted[idx-1])
	return
}
