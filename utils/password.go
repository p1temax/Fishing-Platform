package utils

import (
	"crypto/rand"
	"math/big"
)

// GenerateSecurePassword returns a random password with mixed character classes.
func GenerateSecurePassword(length int) string {
	if length < 12 {
		length = 12
	}

	uppercase := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowercase := "abcdefghijklmnopqrstuvwxyz"
	numbers := "0123456789"
	special := "!@#$%^&*()_+-=[]{}|;:,.<>?"

	password := make([]byte, 0, length)
	password = append(password, uppercase[secureRandInt(len(uppercase))])
	password = append(password, lowercase[secureRandInt(len(lowercase))])
	password = append(password, numbers[secureRandInt(len(numbers))])
	password = append(password, special[secureRandInt(len(special))])

	allChars := uppercase + lowercase + numbers + special
	for i := len(password); i < length; i++ {
		password = append(password, allChars[secureRandInt(len(allChars))])
	}

	for i := len(password) - 1; i > 0; i-- {
		j := secureRandInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password)
}

func secureRandInt(n int) int {
	val, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		panic(err)
	}
	return int(val.Int64())
}
