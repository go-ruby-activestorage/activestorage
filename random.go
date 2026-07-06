package activestorage

import (
	"crypto/rand"
	"math/big"
)

// minimumKeyLength mirrors ActiveStorage::Blob::MINIMUM_TOKEN_LENGTH (28). Rails
// derives a blob's storage key from SecureRandom.base36(MINIMUM_TOKEN_LENGTH).
const minimumKeyLength = 28

// base36Alphabet is the alphabet used by Ruby's SecureRandom.base36:
// "0".."9" followed by "a".."z".
const base36Alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// RandomSource is the randomness seam. The default implementation is backed by
// crypto/rand; tests (and deterministic hosts) can inject their own. It mirrors
// the two primitives Ruby's SecureRandom.base36 relies on: random_bytes and
// random_number.
type RandomSource interface {
	// RandomBytes returns n cryptographically random bytes.
	RandomBytes(n int) ([]byte, error)
	// RandomNumber returns a uniform integer in [0, max).
	RandomNumber(max int) (int, error)
}

type cryptoRandom struct{}

func (cryptoRandom) RandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}

func (cryptoRandom) RandomNumber(max int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64()), err
}

// DefaultRandom returns the crypto/rand-backed RandomSource used when a Config
// leaves Random unset.
func DefaultRandom() RandomSource { return cryptoRandom{} }

// base36 reproduces Ruby's SecureRandom.base36(n): it draws n random bytes and,
// for each, takes the byte modulo 64; bytes whose residue is >= 36 are re-drawn
// with a uniform random_number(36) so the output is unbiased over the 36-symbol
// alphabet.
func base36(src RandomSource, n int) (string, error) {
	raw, err := src.RandomBytes(n)
	if err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, bb := range raw {
		idx := int(bb) % 64
		if idx >= 36 {
			idx, err = src.RandomNumber(36)
			if err != nil {
				return "", err
			}
		}
		out[i] = base36Alphabet[idx]
	}
	return string(out), nil
}

// generateKey returns a fresh 28-character base36 storage key, matching the
// shape of ActiveStorage::Blob.generate_unique_secure_token.
func generateKey(src RandomSource) (string, error) {
	return base36(src, minimumKeyLength)
}
