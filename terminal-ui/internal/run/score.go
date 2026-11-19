package run

// Royal Score is a dominance score: a run starts at RoyalScoreBase and is
// reduced by RoyalScoreLostHand per lost hand and RoyalScoreLostStage per lost
// stage, floored at zero. Extra hands, chip stacks, and timers never affect it.
const (
	RoyalScoreBase      = 10000
	RoyalScoreLostHand  = 250
	RoyalScoreLostStage = 1000
)

// RoyalScore returns the run score for a given number of lost hands and lost
// stages. Both deductions are negative values by convention, so any negative
// lost count simply cancels out. The result is never below zero.
func RoyalScore(lostHands, lostStages int) int {
	score := RoyalScoreBase - RoyalScoreLostHand*lostHands - RoyalScoreLostStage*lostStages
	if score < 0 {
		return 0
	}
	return score
}
