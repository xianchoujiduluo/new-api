package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

func main() {
	privateKey, err := decodePrivateKey(os.Getenv("BACKEND_UPDATE_SIGNING_KEY"))
	if err != nil {
		fatal(err)
	}
	if len(os.Args) == 2 && os.Args[1] == "public-key" {
		publicKey := privateKey.Public().(ed25519.PublicKey)
		fmt.Println(base64.StdEncoding.EncodeToString(publicKey))
		return
	}
	if len(os.Args) != 3 || os.Args[1] != "sign" {
		fatal(fmt.Errorf("usage: backend-release-sign public-key | sign <manifest>"))
	}

	manifest, err := os.ReadFile(os.Args[2])
	if err != nil {
		fatal(fmt.Errorf("read manifest: %w", err))
	}
	encoded := signManifest(privateKey, manifest) + "\n"
	if err := os.WriteFile(os.Args[2]+".sig", []byte(encoded), 0644); err != nil {
		fatal(fmt.Errorf("write signature: %w", err))
	}
}

func signManifest(privateKey ed25519.PrivateKey, manifest []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, manifest))
}

func decodePrivateKey(encoded string) (ed25519.PrivateKey, error) {
	raw := []byte(strings.TrimSpace(encoded))
	if block, _ := pem.Decode(raw); block != nil {
		return parsePKCS8PrivateKey(block.Bytes)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, fmt.Errorf("BACKEND_UPDATE_SIGNING_KEY must be Base64 raw key material or a PKCS#8 PEM key")
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	}
	if block, _ := pem.Decode(decoded); block != nil {
		decoded = block.Bytes
	}
	return parsePKCS8PrivateKey(decoded)
}

func parsePKCS8PrivateKey(der []byte) (ed25519.PrivateKey, error) {
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("BACKEND_UPDATE_SIGNING_KEY must encode an Ed25519 seed, private key, or PKCS#8 key")
	}
	privateKey, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("BACKEND_UPDATE_SIGNING_KEY is not an Ed25519 private key")
	}
	return privateKey, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "backend release signer:", err)
	os.Exit(1)
}
