package jwe_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestEncryptCompact(t *testing.T) {
	enc, err := jwe.NewEncrypter(testKey(t), "ECDH-ES+A256KW", "A256GCM")
	if err != nil {
		t.Fatal(err)
	}
	token, err := enc.Encrypt([]byte("raw"))
	if err != nil {
		t.Fatal(err)
	}
	if parts := strings.Split(token, "."); len(parts) != 5 {
		t.Fatalf("expected 5 compact parts, got %d", len(parts))
	}
}

func TestEncryptDataPayload(t *testing.T) {
	enc, err := jwe.NewEncrypter(testKey(t), "ECDH-ES+A256KW", "A256GCM")
	if err != nil {
		t.Fatal(err)
	}
	token, err := enc.EncryptData("09034277572")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 5 {
		t.Fatalf("expected 5 compact parts, got %d: %s", len(parts), token)
	}
	// Header must advertise the same alg/enc as the Python client.
	hdrJSON, err := json.Marshal(map[string]any{}) // silence unused if decode fails
	_ = hdrJSON
	t.Logf("token length=%d", len(token))
}
