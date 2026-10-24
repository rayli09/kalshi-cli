package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func ed25519PEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func rsaPEM(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der := x509.MarshalPKCS1PrivateKey(priv)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
}

func rsaPKCS8PEM(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestDetectEd25519(t *testing.T) {
	s, err := ParseSigner("key-id-1", ed25519PEM(t))
	if err != nil {
		t.Fatal(err)
	}
	if s.KeyType() != KeyTypeEd25519 {
		t.Fatalf("key type = %q, want ed25519", s.KeyType())
	}
}

func TestDetectRSA(t *testing.T) {
	for name, pemBytes := range map[string][]byte{"pkcs1": rsaPEM(t), "pkcs8": rsaPKCS8PEM(t)} {
		s, err := ParseSigner("key-id-2", pemBytes)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s.KeyType() != KeyTypeRSA {
			t.Fatalf("%s: key type = %q, want rsa", name, s.KeyType())
		}
	}
}

func TestSignInputStripsQueryAndUppercases(t *testing.T) {
	got := SignInput(1730000000123, "get", "/trade-api/v2/markets?limit=10&cursor=abc")
	want := "1730000000123GET/trade-api/v2/markets"
	if got != want {
		t.Fatalf("SignInput = %q, want %q", got, want)
	}
}

func TestSignInputNoQuery(t *testing.T) {
	got := SignInput(1, "DELETE", "/trade-api/v2/portfolio/orders/batched")
	if got != "1DELETE/trade-api/v2/portfolio/orders/batched" {
		t.Fatalf("SignInput = %q", got)
	}
}

func TestEd25519SignRoundTrip(t *testing.T) {
	s, err := ParseSigner("k", ed25519PEM(t))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := s.Sign(1730000000000, "POST", "/trade-api/v2/portfolio/events/orders")
	if err != nil {
		t.Fatal(err)
	}
	if !s.VerifyForTest(1730000000000, "POST", "/trade-api/v2/portfolio/events/orders", sig) {
		t.Fatal("ed25519 signature did not verify")
	}
	if s.VerifyForTest(1730000000000, "POST", "/trade-api/v2/portfolio/orders", sig) {
		t.Fatal("ed25519 signature verified against the wrong path")
	}
	if s.VerifyForTest(1730000000001, "POST", "/trade-api/v2/portfolio/events/orders", sig) {
		t.Fatal("ed25519 signature verified against the wrong timestamp")
	}
}

func TestRSASignRoundTrip(t *testing.T) {
	s, err := ParseSigner("k", rsaPEM(t))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := s.Sign(1730000000000, "GET", "/trade-api/v2/portfolio/balance?foo=bar")
	if err != nil {
		t.Fatal(err)
	}
	// Query string is stripped on both sides, so this must verify.
	if !s.VerifyForTest(1730000000000, "GET", "/trade-api/v2/portfolio/balance", sig) {
		t.Fatal("RSA-PSS signature did not verify")
	}
	if s.VerifyForTest(1730000000000, "GET", "/trade-api/v2/portfolio/positions", sig) {
		t.Fatal("RSA-PSS signature verified against the wrong path")
	}
}

func TestHeadersAt(t *testing.T) {
	s, err := ParseSigner("my-key-id", ed25519PEM(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1730000000456)
	h, err := s.HeadersAt(now, "get", "/trade-api/v2/markets?limit=1")
	if err != nil {
		t.Fatal(err)
	}
	if got := h.Get("KALSHI-ACCESS-KEY"); got != "my-key-id" {
		t.Fatalf("KEY header = %q", got)
	}
	if got := h.Get("KALSHI-ACCESS-TIMESTAMP"); got != "1730000000456" {
		t.Fatalf("TIMESTAMP header = %q", got)
	}
	sig := h.Get("KALSHI-ACCESS-SIGNATURE")
	if sig == "" {
		t.Fatal("empty SIGNATURE header")
	}
	if !s.VerifyForTest(1730000000456, "GET", "/trade-api/v2/markets", sig) {
		t.Fatal("header signature does not verify")
	}
}

func TestFingerprintStableAndSafe(t *testing.T) {
	p := ed25519PEM(t)
	s1, err := ParseSigner("k", p)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := ParseSigner("k", p)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Fingerprint() != s2.Fingerprint() {
		t.Fatal("fingerprint not stable for the same key")
	}
	if !strings.HasPrefix(s1.Fingerprint(), "sha256:") || len(s1.Fingerprint()) != len("sha256:")+16 {
		t.Fatalf("unexpected fingerprint shape: %q", s1.Fingerprint())
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := ParseSigner("k", []byte("not a pem")); err == nil {
		t.Fatal("expected error for non-PEM input")
	}
	bad := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{1, 2, 3}})
	if _, err := ParseSigner("k", bad); err == nil {
		t.Fatal("expected error for certificate PEM")
	}
}
