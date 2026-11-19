// The evaluator is a small original five-card implementation. Midnight Royale
// does not need Hold'em/Omaha deal machinery or large lookup tables. Keeping the
// Evaluator interface local makes exhaustive tests straightforward and avoids a
// runtime dependency for nine conventional hand categories.
package game

import (
	"errors"
	"sort"
)

type Category uint8

const (
	HighCard Category = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
)

func (c Category) String() string {
	return [...]string{"High card", "One pair", "Two pair", "Three of a kind", "Straight", "Flush", "Full house", "Four of a kind", "Straight flush"}[c]
}

type Score struct {
	Category Category `json:"category"`
	Tiebreak [5]int   `json:"tiebreak"`
}

type rankGroup struct{ count, rank int }

type Evaluator interface {
	Evaluate([5]Card) (Score, error)
}

type InternalEvaluator struct{}

func (InternalEvaluator) Evaluate(hand [5]Card) (Score, error) {
	counts := [15]int{}
	seen := make(map[Card]struct{}, 5)
	flush := true
	for i, card := range hand {
		if !card.Valid() {
			return Score{}, errors.New("game: invalid card in hand")
		}
		if _, ok := seen[card]; ok {
			return Score{}, errors.New("game: duplicate card in hand")
		}
		seen[card] = struct{}{}
		counts[card.Rank]++
		if i > 0 && card.Suit != hand[0].Suit {
			flush = false
		}
	}

	distinct := make([]int, 0, 5)
	for rank := 14; rank >= 2; rank-- {
		if counts[rank] > 0 {
			distinct = append(distinct, rank)
		}
	}
	straightHigh := 0
	if len(distinct) == 5 {
		if distinct[0]-distinct[4] == 4 {
			straightHigh = distinct[0]
		} else if distinct[0] == 14 && distinct[1] == 5 && distinct[4] == 2 {
			straightHigh = 5
		}
	}
	if straightHigh > 0 && flush {
		return Score{Category: StraightFlush, Tiebreak: [5]int{straightHigh}}, nil
	}

	groups := make([]rankGroup, 0, len(distinct))
	for rank := 14; rank >= 2; rank-- {
		if counts[rank] > 0 {
			groups = append(groups, rankGroup{counts[rank], rank})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})
	if groups[0].count == 4 {
		return Score{Category: FourOfAKind, Tiebreak: [5]int{groups[0].rank, groups[1].rank}}, nil
	}
	if groups[0].count == 3 && groups[1].count == 2 {
		return Score{Category: FullHouse, Tiebreak: [5]int{groups[0].rank, groups[1].rank}}, nil
	}
	if flush {
		return Score{Category: Flush, Tiebreak: ranksDescending(counts)}, nil
	}
	if straightHigh > 0 {
		return Score{Category: Straight, Tiebreak: [5]int{straightHigh}}, nil
	}
	if groups[0].count == 3 {
		return Score{Category: ThreeOfAKind, Tiebreak: groupRanks(groups)}, nil
	}
	if groups[0].count == 2 && groups[1].count == 2 {
		return Score{Category: TwoPair, Tiebreak: groupRanks(groups)}, nil
	}
	if groups[0].count == 2 {
		return Score{Category: OnePair, Tiebreak: groupRanks(groups)}, nil
	}
	return Score{Category: HighCard, Tiebreak: ranksDescending(counts)}, nil
}

func ranksDescending(counts [15]int) (out [5]int) {
	i := 0
	for rank := 14; rank >= 2; rank-- {
		for n := 0; n < counts[rank]; n++ {
			out[i] = rank
			i++
		}
	}
	return out
}

func groupRanks(groups []rankGroup) [5]int {
	var out [5]int
	for i, g := range groups {
		out[i] = g.rank
	}
	return out
}

func Compare(a, b Score) int {
	if a.Category < b.Category {
		return -1
	}
	if a.Category > b.Category {
		return 1
	}
	for i := range a.Tiebreak {
		if a.Tiebreak[i] < b.Tiebreak[i] {
			return -1
		}
		if a.Tiebreak[i] > b.Tiebreak[i] {
			return 1
		}
	}
	return 0
}
