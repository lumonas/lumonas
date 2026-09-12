package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
)

func EncryptCredentials(credentials Credentials, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("backup credential encryption key is required")
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func DecryptCredentials(ciphertext, key []byte) (Credentials, error) {
	if len(key) == 0 {
		return Credentials{}, errors.New("backup credential decryption key is required")
	}
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return Credentials{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Credentials{}, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return Credentials{}, errors.New("encrypted backup credentials are too short")
	}
	plaintext, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil {
		return Credentials{}, errors.New("encrypted backup credentials failed authentication")
	}
	var credentials Credentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return Credentials{}, errors.New("encrypted backup credentials are malformed")
	}
	return credentials, nil
}

func deriveKey(key []byte) []byte {
	digest := sha256.Sum256(key)
	return digest[:]
}
