package gitlab

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// Cipher encrypts and decrypts bytes using the control-plane KMS.
type Cipher interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// AESCipher implements Cipher using AES-GCM with a static key.
type AESCipher struct {
	key []byte
}

// NewAESCipher returns an AES-GCM cipher backed by the provided key.
func NewAESCipher(key []byte) (*AESCipher, error) {
	length := len(key)
	if length != 16 && length != 24 && length != 32 {
		return nil, fmt.Errorf("gitlab signer: aes key must be 16, 24, or 32 bytes")
	}
	copied := make([]byte, length)
	copy(copied, key)
	return &AESCipher{key: copied}, nil
}

// Encrypt applies AES-GCM to the plaintext and returns nonce+ciphertext.
func (a *AESCipher) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	data := gcm.Seal(nonce, nonce, plaintext, nil)
	return data, nil
}

// Decrypt reverses the AES-GCM encryption applied by Encrypt.
func (a *AESCipher) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("gitlab signer: ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	data := ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}
