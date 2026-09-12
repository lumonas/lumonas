package notify

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type Channel struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Label      string    `json:"label"`
	Target     string    `json:"target,omitempty"`
	Configured bool      `json:"configured"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Credentials struct {
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Address  string `json:"address,omitempty"`
}

var channelTypes = map[string]bool{"webhook": true, "ntfy": true, "telegram": true, "slack": true, "discord": true, "gotify": true, "smtp": true}

func (c Channel) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Label) == "" {
		return errors.New("notification channel id and label are required")
	}
	if !channelTypes[c.Type] {
		return fmt.Errorf("unsupported notification channel type %q", c.Type)
	}
	if strings.TrimSpace(c.Target) == "" {
		return errors.New("notification channel target is required")
	}
	if strings.ContainsAny(c.ID+c.Label+c.Target, "\x00\r\n") {
		return errors.New("notification channel contains unsupported control characters")
	}
	return nil
}

func EncryptCredentials(value Credentials, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("notification encryption key is required")
	}
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(key)
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, nil), nil
}

func DecryptCredentials(ciphertext, key []byte) (Credentials, error) {
	if len(key) == 0 {
		return Credentials{}, errors.New("notification decryption key is required")
	}
	digest := sha256.Sum256(key)
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return Credentials{}, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return Credentials{}, err
	}
	if len(ciphertext) < aead.NonceSize() {
		return Credentials{}, errors.New("notification credentials are malformed")
	}
	plain, err := aead.Open(nil, ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():], nil)
	if err != nil {
		return Credentials{}, errors.New("notification credentials could not be decrypted")
	}
	var value Credentials
	if err := json.Unmarshal(plain, &value); err != nil {
		return Credentials{}, errors.New("notification credentials are malformed")
	}
	return value, nil
}

func HasCredentials(value Credentials) bool {
	return value.Token != "" || value.Username != "" || value.Password != "" || value.Address != ""
}
