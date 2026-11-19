// Package simulation runs deterministic, headless bot evaluation against a
// documented simple baseline. Results compare policies, not human players.
// Each simulated "match" is one level (play until a bust), so mean hands per
// match is also mean hands per level.
package simulation

import (
	"context"
	"errors"
	"math"
	"time"

	"midnight-royale/internal/game"
	"midnight-royale/internal/opponent"
)

type Config struct {
	Matches          int   `json:"matches"`
	MaxHandsPerMatch int   `json:"max_hands_per_match"`
	Seed             int64 `json:"seed"`
}

type Result struct {
	CharacterID              string              `json:"character_id"`
	Difficulty               opponent.Difficulty `json:"difficulty"`
	Matches                  int                 `json:"matches"`
	Hands                    int                 `json:"hands"`
	BotMatchWins             int                 `json:"bot_match_wins"`
	BaselineMatchWins        int                 `json:"baseline_match_wins"`
	TiedOrCappedMatches      int                 `json:"tied_or_capped_matches"`
	BotHandWins              int                 `json:"bot_hand_wins"`
	BaselineHandWins         int                 `json:"baseline_hand_wins"`
	TiedHands                int                 `json:"tied_hands"`
	BotChipDelta             int                 `json:"bot_chip_delta"`
	MeanHandsPerMatch        float64             `json:"mean_hands_per_match"`
	BotMatchWinRate          float64             `json:"bot_match_win_rate"`
	MatchWinStdError         float64             `json:"match_win_standard_error"`
	Decisions                int                 `json:"decisions"`
	Folds                    int                 `json:"folds"`
	FoldRate                 float64             `json:"fold_rate"`
	MeanDecisionMicros       float64             `json:"mean_decision_micros"`
	ChipConservationFailures int                 `json:"chip_conservation_failures"`
}

func (r Result) WithoutLatency() Result {
	r.MeanDecisionMicros = 0
	return r
}

func RunDifficultySuite(ctx context.Context, cfg Config) ([]Result, error) {
	profiles := opponent.DefaultProfiles()
	indexes := []int{0, 3, 6, 9}
	results := make([]Result, 0, len(indexes))
	for _, index := range indexes {
		copyCfg := cfg
		profile := profiles[index]
		// Hold personality constant here so the suite measures difficulty policy,
		// not whether the representative character happens to be aggressive.
		profile.Personality = opponent.Personality{Aggression: 45, Calling: 45, Bluffing: 10}
		result, err := Run(ctx, profile, copyCfg)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func Run(ctx context.Context, profile opponent.Profile, cfg Config) (Result, error) {
	if cfg.Matches < 1 {
		return Result{}, errors.New("simulation: matches must be positive")
	}
	if cfg.MaxHandsPerMatch < 1 {
		cfg.MaxHandsPerMatch = 300
	}
	gcfg := game.DefaultConfig()
	startStack := gcfg.StartingStack
	startChips := startStack * 2
	result := Result{CharacterID: profile.CharacterID, Difficulty: profile.Difficulty, Matches: cfg.Matches}
	var decisionTime time.Duration
	for match := 0; match < cfg.Matches; match++ {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		g, err := game.New(gcfg, game.NewSeededRNG(cfg.Seed+int64(match)*7919))
		if err != nil {
			return Result{}, err
		}
		bot := opponent.New(profile, game.NewSeededRNG(cfg.Seed+int64(match)*7919+1))
		if err := g.StartHand(); err != nil {
			return Result{}, err
		}
		ended := false
		for handNo := 0; handNo < cfg.MaxHandsPerMatch; {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			switch g.Phase() {
			case game.BettingBeforeDraw, game.BettingAfterDraw:
				if g.Actor() == game.Human {
					action, amount := baselineBet(g)
					if err := g.Act(game.Human, action, amount, g.Token()); err != nil {
						return Result{}, err
					}
				} else {
					view := botView(g)
					started := time.Now()
					action, amount, err := bot.ChooseBet(ctx, view)
					decisionTime += time.Since(started)
					result.Decisions++
					if err != nil {
						return Result{}, err
					}
					if action == game.Fold {
						result.Folds++
					}
					if err := g.Act(game.Opponent, action, amount, g.Token()); err != nil {
						return Result{}, err
					}
				}
			case game.Drawing:
				if g.Actor() == game.Human {
					if err := g.Draw(game.Human, baselineDraw(g.Hand(game.Human)), g.Token()); err != nil {
						return Result{}, err
					}
				} else {
					view := botView(g)
					started := time.Now()
					discard, err := bot.ChooseDraw(ctx, view)
					decisionTime += time.Since(started)
					result.Decisions++
					if err != nil {
						return Result{}, err
					}
					if err := g.Draw(game.Opponent, discard, g.Token()); err != nil {
						return Result{}, err
					}
				}
			case game.HandResult:
				handNo++
				result.Hands++
				r := g.Result()
				if r.Tie {
					result.TiedHands++
				} else if r.Winner == game.Opponent {
					result.BotHandWins++
				} else {
					result.BaselineHandWins++
				}
				if g.TotalChips() != startChips {
					result.ChipConservationFailures++
				}
				if r.MatchOver {
					if g.Stacks()[game.Opponent] > 0 {
						result.BotMatchWins++
					} else {
						result.BaselineMatchWins++
					}
					result.BotChipDelta += g.Stacks()[game.Opponent] - startStack
					ended = true
				} else if err := g.NextHand(); err != nil {
					return Result{}, err
				}
			case game.MatchResult:
				ended = true
			default:
				return Result{}, errors.New("simulation: unexpected game phase")
			}
			if ended {
				break
			}
		}
		if !ended {
			stacks := g.Stacks()
			result.BotChipDelta += stacks[game.Opponent] - startStack
			if stacks[game.Opponent] > stacks[game.Human] {
				result.BotMatchWins++
			} else if stacks[game.Opponent] < stacks[game.Human] {
				result.BaselineMatchWins++
			} else {
				result.TiedOrCappedMatches++
			}
		}
	}
	result.MeanHandsPerMatch = float64(result.Hands) / float64(result.Matches)
	result.BotMatchWinRate = float64(result.BotMatchWins) / float64(result.Matches)
	result.MatchWinStdError = math.Sqrt(result.BotMatchWinRate * (1 - result.BotMatchWinRate) / float64(result.Matches))
	if result.Decisions > 0 {
		result.FoldRate = float64(result.Folds) / float64(result.Decisions)
		result.MeanDecisionMicros = float64(decisionTime.Microseconds()) / float64(result.Decisions)
	}
	return result, nil
}

func botView(g *game.Game) opponent.DecisionView {
	s := g.Snapshot(game.Opponent)
	return opponent.DecisionView{
		OwnHand: s.Hand, Phase: s.Phase, Stack: s.Stacks[game.Opponent],
		OpponentStack: s.Stacks[game.Human], Pot: s.Pot, ToCall: s.ToCall,
		MinBet: g.MinBet(), MinRaiseTo: g.MinRaiseTo(game.Opponent),
		MaxCommitTo: g.MaxCommitTo(game.Opponent),
		Raises:      s.Raises, Dealer: s.Dealer == game.Opponent,
		OpponentDrawCount: s.OpponentDrawCount, History: s.History, Legal: s.Legal,
	}
}

// baselineBet is intentionally simple and documented: check when possible, open
// with two pair or better at the minimum bet, call with a pair or better or when
// the price is at most one fifth of the pot, and never raise.
func baselineBet(g *game.Game) (game.Action, int) {
	s := g.Snapshot(game.Human)
	score, _ := (game.InternalEvaluator{}).Evaluate(s.Hand)
	if containsAction(s.Legal, game.Check) {
		if score.Category >= game.TwoPair && containsAction(s.Legal, game.Bet) {
			return game.Bet, min(g.MinBet(), g.Stacks()[game.Human])
		}
		return game.Check, 0
	}
	if containsAction(s.Legal, game.Call) && (score.Category >= game.OnePair || s.ToCall*5 <= max(1, s.Pot)) {
		return game.Call, 0
	}
	if containsAction(s.Legal, game.Fold) {
		return game.Fold, 0
	}
	return s.Legal[0], 0
}

// baselineDraw keeps all made straights or better, all duplicate ranks, and
// otherwise the two highest cards. It may exchange any number up to five.
func baselineDraw(hand [5]game.Card) []int {
	score, _ := (game.InternalEvaluator{}).Evaluate(hand)
	if score.Category >= game.Straight {
		return nil
	}
	counts := map[game.Rank]int{}
	for _, card := range hand {
		counts[card.Rank]++
	}
	keep := [5]bool{}
	for i, card := range hand {
		if counts[card.Rank] > 1 {
			keep[i] = true
		}
	}
	kept := 0
	for _, value := range keep {
		if value {
			kept++
		}
	}
	if kept == 0 {
		first, second := -1, -1
		for i, card := range hand {
			if first < 0 || card.Rank > hand[first].Rank {
				second, first = first, i
			} else if second < 0 || card.Rank > hand[second].Rank {
				second = i
			}
		}
		keep[first], keep[second] = true, true
	}
	var out []int
	for i, value := range keep {
		if !value {
			out = append(out, i)
		}
	}
	return out
}

func containsAction(actions []game.Action, want game.Action) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}
