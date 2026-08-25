package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodePrivateKeyAcceptsSeedAndExpandedKey(t *testing.T) {
	seed := bytes.Repeat([]byte{0x2a}, ed25519.SeedSize)
	want := ed25519.NewKeyFromSeed(seed)

	fromSeed, err := decodePrivateKey(base64.StdEncoding.EncodeToString(seed))
	require.NoError(t, err)
	assert.Equal(t, want, fromSeed)

	fromExpanded, err := decodePrivateKey(base64.StdEncoding.EncodeToString(want))
	require.NoError(t, err)
	assert.Equal(t, want, fromExpanded)
}

func TestDecodePrivateKeyRejectsInvalidLength(t *testing.T) {
	_, err := decodePrivateKey(base64.StdEncoding.EncodeToString([]byte("too short")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Ed25519")
}

func TestDecodePrivateKeyAcceptsPKCS8PEM(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x3b}, ed25519.SeedSize))
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	decoded, err := decodePrivateKey(string(encoded))
	require.NoError(t, err)
	assert.Equal(t, privateKey, decoded)
}

func TestSignManifestProducesDetachedBase64Signature(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x17}, ed25519.SeedSize))
	manifest := []byte(`{"schema_version":1}`)

	encoded := signManifest(privateKey, manifest)
	signature, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	assert.True(t, ed25519.Verify(publicKey, manifest, signature))
}
