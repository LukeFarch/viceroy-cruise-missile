// SPDX-License-Identifier: GPL-3.0-or-later
package protocol

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"strings"
)

// LoadPrivateKey reads an Ed25519 PKCS#8 PEM private key from disk.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	edKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("not an Ed25519 key")
	}
	return edKey, nil
}

// LoadPublicKeyBase64 decodes a base64-encoded PKIX public key.
func LoadPublicKeyBase64(b64 string) (ed25519.PublicKey, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("not an Ed25519 public key")
	}
	return edPub, nil
}

// signatureInput builds the material that gets signed:
// destination + source + msg + msg_type + nonce
func signatureInput(t *Transmission) []byte {
	parts := []string{t.Destination, t.Source, t.Msg, t.MsgType, t.Nonce}
	return []byte(strings.Join(parts, ""))
}

// Sign sets msg_sig and nonce on the transmission. Nonce is generated if empty.
func Sign(t *Transmission, priv ed25519.PrivateKey) {
	if priv == nil {
		return
	}
	if t.Nonce == "" {
		t.Nonce = GenerateNonce()
	}
	sig := ed25519.Sign(priv, signatureInput(t))
	t.MsgSig = base64.StdEncoding.EncodeToString(sig)
}

// Verify checks the transmission signature against a public key.
func Verify(t *Transmission, pub ed25519.PublicKey) bool {
	if pub == nil || t.MsgSig == "" {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(t.MsgSig)
	if err != nil {
		return false
	}
	return ed25519.Verify(pub, signatureInput(t), sig)
}
