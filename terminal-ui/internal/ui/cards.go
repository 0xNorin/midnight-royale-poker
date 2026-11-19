package ui

import "midnight-royale/internal/game"

// suitInk returns the foreground colour for a suit's corner and centre symbols.
// Hearts and diamonds are red; clubs and spades stay on the theme's dark card
// ink. The mapping is by suit identity only — never by card position.
func suitInk(suit game.Suit, t Theme) string {
	switch suit {
	case game.Hearts, game.Diamonds:
		return "#B33755"
	default:
		return t.Ink
	}
}
