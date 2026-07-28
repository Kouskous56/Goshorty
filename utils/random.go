package utils

import (
	"math/rand"
	"time"
)

const (
	charset     = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	codeLength  = 6
)

var seededRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// GenerateShortCode generates a random short code
func GenerateShortCode(length int) string {
	if length <= 0 {
		length = codeLength
	}
	
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// GenerateID generates a unique identifier
func GenerateID() string {
	return GenerateShortCode(16)
}
