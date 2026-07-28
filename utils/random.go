package utils

import (
	"crypto/rand"
	"math/big"
)

const (
	charset    = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	codeLength = 6
)

// GenerateShortCode generates a cryptographically random, URL-safe code.
func GenerateShortCode(length int) string {
	if length <= 0 {
		length = codeLength
	}

	b := make([]byte, length)
	for i := range b {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			panic("secure random source unavailable: " + err.Error())
		}
		b[i] = charset[index.Int64()]
	}
	return string(b)
}

// GenerateID generates a unique identifier
func GenerateID() string {
	return GenerateShortCode(16)
}
