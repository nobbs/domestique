package sqlite

import (
	"crypto/rand"
	"fmt"
	"io"
)

func (s *Store) encrypt(targetID string, value []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("creating encryption nonce: %w", err)
	}

	return s.aead.Seal(nonce, nonce, value, []byte(targetID)), nil
}

func (s *Store) decrypt(targetID string, value []byte) ([]byte, error) {
	nonceSize := s.aead.NonceSize()
	if len(value) <= nonceSize {
		return nil, ErrStateUnreadable
	}
	decrypted, err := s.aead.Open(nil, value[:nonceSize], value[nonceSize:], []byte(targetID))
	if err != nil {
		return nil, ErrStateUnreadable
	}

	return decrypted, nil
}
