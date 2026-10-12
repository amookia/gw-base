package jwe

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// Encrypter encrypts for one recipient. It is safe for concurrent use: every
// Encrypt call builds its own go-jose encrypter, so nothing mutable is shared.
type Encrypter struct {
	key any
	alg jose.KeyAlgorithm
	enc jose.ContentEncryption
}

// NewEncrypter parses a PEM public key (PKIX, "BEGIN PUBLIC KEY") and checks
// that alg and enc suit it, so a bad setup fails at startup, not on a request.
func NewEncrypter(publicKeyPEM []byte, alg, enc string) (*Encrypter, error) {
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return nil, errors.New("public key is not valid PEM")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing public key: %w", err)
	}

	e := &Encrypter{key: key, alg: jose.KeyAlgorithm(alg), enc: jose.ContentEncryption(enc)}
	if _, err := e.build(); err != nil {
		return nil, err
	}
	return e, nil
}

// Encrypt returns the compact serialization (header.key.iv.ciphertext.tag).
// The protected header holds alg, enc and the ephemeral key; there is no kid.
func (e *Encrypter) Encrypt(payload []byte) (string, error) {
	enc, err := e.build()
	if err != nil {
		return "", err
	}
	obj, err := enc.Encrypt(payload)
	if err != nil {
		return "", fmt.Errorf("encrypting: %w", err)
	}
	return obj.CompactSerialize()
}

// EncryptData encrypts the Shahkar-style claim object:
//
//	{"data": "<raw>", "iat": <unix seconds>}
//
// matching the Java JWEECDHCrypto / Python jwcrypto client behaviour.
func (e *Encrypter) EncryptData(raw string) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"data": raw,
		"iat":  time.Now().Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("building jwe payload: %w", err)
	}
	return e.Encrypt(payload)
}

func (e *Encrypter) build() (jose.Encrypter, error) {
	enc, err := jose.NewEncrypter(e.enc, jose.Recipient{Algorithm: e.alg, Key: e.key}, nil)
	if err != nil {
		return nil, fmt.Errorf("alg %q / enc %q do not fit this public key: %w", e.alg, e.enc, err)
	}
	return enc, nil
}
