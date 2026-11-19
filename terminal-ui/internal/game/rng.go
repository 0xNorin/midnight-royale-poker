package game

import (
	cryptorand "crypto/rand"
	"errors"
	"math/big"
	mathrand "math/rand"
)

type RNG interface {
	Intn(int) (int, error)
}

type CryptoRNG struct{}

func (CryptoRNG) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("game: random bound must be positive")
	}
	v, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

type seededRNG struct{ r *mathrand.Rand }

// NewSeededRNG is for tests, simulations, and explicitly labeled demo fixtures.
// Ordinary play uses CryptoRNG and never derives dealing randomness from UI RNG.
func NewSeededRNG(seed int64) RNG { return &seededRNG{r: mathrand.New(mathrand.NewSource(seed))} }

func (r *seededRNG) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, errors.New("game: random bound must be positive")
	}
	return r.r.Intn(n), nil
}

func ShuffledDeck(rng RNG) ([]Card, error) {
	if rng == nil {
		rng = CryptoRNG{}
	}
	deck := StandardDeck()
	for i := len(deck) - 1; i > 0; i-- {
		j, err := rng.Intn(i + 1)
		if err != nil {
			return nil, err
		}
		if j < 0 || j > i {
			return nil, errors.New("game: random source returned out-of-range value")
		}
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck, nil
}
