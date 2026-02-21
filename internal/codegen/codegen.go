package codegen

import (
	"crypto/rand"
	"math/big"
)

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// GenerateCode returns a random string of length n from the default alphabet.
// Suitable for short URL codes (e.g. n=6).
func GenerateCode(n int) (string, error) {
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[num.Int64()]
	}
	return string(out), nil
}
