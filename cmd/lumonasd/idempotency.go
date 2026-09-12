package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func requestIdempotencyKey(r *http.Request) (string, error) {
	value := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if value == "" {
		return "", nil
	}
	if !idempotencyPattern.MatchString(value) {
		return "", errors.New("Idempotency-Key contains unsupported characters")
	}
	return value, nil
}

func idempotencyMetaKey(scope, key string) string {
	digest := sha256.Sum256([]byte(scope + "\x00" + key))
	return "idempotency." + scope + "." + hex.EncodeToString(digest[:])
}
