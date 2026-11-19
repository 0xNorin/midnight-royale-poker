package game

import (
	"fmt"
	"strings"
)

type Rank uint8

type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

type Card struct {
	Rank Rank `json:"rank"`
	Suit Suit `json:"suit"`
}

func (c Card) Valid() bool { return c.Rank >= 2 && c.Rank <= 14 && c.Suit <= Spades }

func (c Card) String() string {
	ranks := "--23456789TJQKA"
	suits := "cdhs"
	if !c.Valid() {
		return "??"
	}
	return string([]byte{ranks[c.Rank], suits[c.Suit]})
}

func ParseCard(s string) (Card, error) {
	if len(s) != 2 {
		return Card{}, fmt.Errorf("game: invalid card %q", s)
	}
	rankText := strings.IndexByte("--23456789TJQKA", byte(strings.ToUpper(s[:1])[0]))
	suitText := strings.IndexByte("cdhs", byte(strings.ToLower(s[1:])[0]))
	c := Card{Rank: Rank(rankText), Suit: Suit(suitText)}
	if rankText < 2 || suitText < 0 || !c.Valid() {
		return Card{}, fmt.Errorf("game: invalid card %q", s)
	}
	return c, nil
}

func ParseHand(text string) ([5]Card, error) {
	var hand [5]Card
	parts := strings.Fields(text)
	if len(parts) != len(hand) {
		return hand, fmt.Errorf("game: hand must contain five cards")
	}
	for i, part := range parts {
		c, err := ParseCard(part)
		if err != nil {
			return hand, err
		}
		hand[i] = c
	}
	return hand, nil
}

func StandardDeck() []Card {
	deck := make([]Card, 0, 52)
	for suit := Clubs; suit <= Spades; suit++ {
		for rank := Rank(2); rank <= 14; rank++ {
			deck = append(deck, Card{Rank: rank, Suit: suit})
		}
	}
	return deck
}
