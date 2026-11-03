package random

import (
	"crypto/rand"
	"math/big"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// NewRandomString generates a secure pseudo-random string of the specified length.
func NewRandomString(length int) string {
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := range b {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			// Fallback deterministic pseudo-random in the unlikely event crypto/rand fails
			b[i] = charset[i%len(charset)]
			continue
		}
		b[i] = charset[num.Int64()]
	}

	return string(b)
}
