// Package run encodes Midnight Royale's level lifecycle as a single explicit,
// testable transition function. It holds no I/O and no poker rules: it maps a
// settled bust to the next level state.
package run

// Levels 1..4 are playable; 5 is the victory image (never playable).
const (
	MinLevel     = 1
	MaxLevel     = 4
	VictoryLevel = 5
)

type Kind uint8

const (
	None Kind = iota
	Advance
	Retreat
	Retry
	Victory
)

func (k Kind) String() string {
	return [...]string{"none", "advance", "retreat", "retry", "victory"}[k]
}

type Transition struct {
	Kind      Kind
	NextLevel int
}

// ResolveBust evaluates one settled hand-end where exactly one side busted.
// playerBusted reports whether the human reached zero chips. Levels never move
// below 1 or above the victory image.
func ResolveBust(currentLevel int, playerBusted bool) Transition {
	if currentLevel < MinLevel {
		currentLevel = MinLevel
	}
	if currentLevel > VictoryLevel {
		currentLevel = VictoryLevel
	}
	if playerBusted {
		if currentLevel <= MinLevel {
			return Transition{Kind: Retry, NextLevel: MinLevel}
		}
		return Transition{Kind: Retreat, NextLevel: currentLevel - 1}
	}
	if currentLevel >= MaxLevel {
		return Transition{Kind: Victory, NextLevel: VictoryLevel}
	}
	return Transition{Kind: Advance, NextLevel: currentLevel + 1}
}
