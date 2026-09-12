package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const iterations = 210_000

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", fmt.Errorf("password must contain at least 12 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived := derive([]byte(password), salt, iterations)
	return fmt.Sprintf("lumonas-pbkdf2-sha256$%d$%s$%s", iterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "lumonas-pbkdf2-sha256" {
		return false
	}
	count, err := strconv.Atoi(parts[1])
	if err != nil || count < 100_000 || count > 1_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := derive([]byte(password), salt, count)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func NewToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func TokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func derive(password, salt []byte, count int) []byte {
	mac := hmac.New(sha256.New, password)
	mac.Write(salt)
	mac.Write([]byte{0, 0, 0, 1})
	previous := mac.Sum(nil)
	result := append([]byte(nil), previous...)
	for i := 1; i < count; i++ {
		mac = hmac.New(sha256.New, password)
		mac.Write(previous)
		previous = mac.Sum(nil)
		for index := range result {
			result[index] ^= previous[index]
		}
	}
	return result
}
