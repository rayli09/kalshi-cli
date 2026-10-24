// Package auth implements Kalshi's per-request API-key signing.
//
// Every private Kalshi request carries three headers:
//
//	KALSHI-ACCESS-KEY:       the API key ID
//	KALSHI-ACCESS-TIMESTAMP: current time in milliseconds
//	KALSHI-ACCESS-SIGNATURE: base64 signature over
//	                         timestamp + METHOD + path
//
// where path is the full request path from the API root WITHOUT the query
// string (e.g. /trade-api/v2/portfolio/balance). Two key types exist:
// Ed25519 (default for keys created in the web app since 2026-10-01),
// which signs the message directly, and RSA, which signs with RSA-PSS
// (SHA-256, MGF1-SHA256, salt length = digest length). The type is
// auto-detected from the PEM contents. Key material is never printed or
// logged anywhere in this package or its callers; Fingerprint exposes
// only a hash of the public key.
package auth

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// KeyType identifies which signing algorithm a loaded key uses.
type KeyType string

const (
	KeyTypeEd25519 KeyType = "ed25519"
	KeyTypeRSA     KeyType = "rsa"
)

// Signer holds a loaded private key and produces Kalshi auth headers.
type Signer struct {
	KeyID     string
	keyType   KeyType
	ed25519   ed25519.PrivateKey
	rsa       *rsa.PrivateKey
	publicDER []byte // PKIX encoding of the public key, for fingerprinting
}

// LoadSigner reads a PEM private key file and detects its type. It
// accepts PKCS#8 ("BEGIN PRIVATE KEY") for both Ed25519 and RSA, PKCS#1
// ("BEGIN RSA PRIVATE KEY"), and SEC1 EC blocks are rejected with a clear
// error (Kalshi does not issue EC keys).
func LoadSigner(keyID, pemPath string) (*Signer, error) {
	data, err := os.ReadFile(pemPath)
	if err != nil {
		return nil, fmt.Errorf("reading private key file: %w", err)
	}
	return ParseSigner(keyID, data)
}

// ParseSigner parses PEM-encoded private key material held in memory.
func ParseSigner(keyID string, pemData []byte) (*Signer, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("private key file contains no PEM block")
	}
	s := &Signer{KeyID: keyID}
	var pub crypto.PublicKey
	switch block.Type {
	case "PRIVATE KEY", "RSA PRIVATE KEY", "ED25519 PRIVATE KEY":
		key, err := parsePrivateKeyDER(block.Bytes)
		if err != nil {
			return nil, err
		}
		switch k := key.(type) {
		case ed25519.PrivateKey:
			s.keyType = KeyTypeEd25519
			s.ed25519 = k
			pub = k.Public()
		case *rsa.PrivateKey:
			s.keyType = KeyTypeRSA
			s.rsa = k
			pub = k.Public()
		default:
			return nil, fmt.Errorf("unsupported private key type %T: Kalshi keys are Ed25519 or RSA", key)
		}
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q: expected a private key", block.Type)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("encoding public key: %w", err)
	}
	s.publicDER = der
	return s, nil
}

func parsePrivateKeyDER(der []byte) (crypto.PrivateKey, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("could not parse private key (tried PKCS#8 and PKCS#1)")
}

// KeyType reports the detected key type.
func (s *Signer) KeyType() KeyType { return s.keyType }

// Fingerprint returns a short, non-sensitive identifier for the key:
// "sha256:" + the first 16 hex chars of the SHA-256 of the public key's
// PKIX DER encoding. Safe to print and log.
func (s *Signer) Fingerprint() string {
	sum := sha256.Sum256(s.publicDER)
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

// SignInput builds the exact string Kalshi expects to be signed:
// timestamp + UPPERCASE method + path with any query string stripped.
// Exported so both REST and WebSocket code — and tests — share it.
func SignInput(timestampMs int64, method, path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return strconv.FormatInt(timestampMs, 10) + strings.ToUpper(method) + path
}

// Sign produces the base64 signature for one request at the given time.
func (s *Signer) Sign(timestampMs int64, method, path string) (string, error) {
	msg := []byte(SignInput(timestampMs, method, path))
	switch s.keyType {
	case KeyTypeEd25519:
		return base64.StdEncoding.EncodeToString(ed25519.Sign(s.ed25519, msg)), nil
	case KeyTypeRSA:
		digest := sha256.Sum256(msg)
		sig, err := rsa.SignPSS(rand.Reader, s.rsa, crypto.SHA256, digest[:], &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
			Hash:       crypto.SHA256,
		})
		if err != nil {
			return "", fmt.Errorf("RSA-PSS signing: %w", err)
		}
		return base64.StdEncoding.EncodeToString(sig), nil
	}
	return "", fmt.Errorf("unhandled key type %q", s.keyType)
}

// Headers returns the three KALSHI-ACCESS-* headers for a request made
// now. path must be the full path from the API root (with or without a
// query string — it is stripped before signing).
func (s *Signer) Headers(method, path string) (http.Header, error) {
	return s.HeadersAt(time.Now(), method, path)
}

// HeadersAt is Headers with an explicit clock, for tests.
func (s *Signer) HeadersAt(now time.Time, method, path string) (http.Header, error) {
	ts := now.UnixMilli()
	sig, err := s.Sign(ts, method, path)
	if err != nil {
		return nil, err
	}
	h := make(http.Header)
	h.Set("KALSHI-ACCESS-KEY", s.KeyID)
	h.Set("KALSHI-ACCESS-TIMESTAMP", strconv.FormatInt(ts, 10))
	h.Set("KALSHI-ACCESS-SIGNATURE", sig)
	return h, nil
}

// VerifyForTest re-derives the public-key check for a signature produced
// by Sign. It exists for the unit tests (round-trip verification) and is
// not used on any request path.
func (s *Signer) VerifyForTest(timestampMs int64, method, path, sigB64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	msg := []byte(SignInput(timestampMs, method, path))
	switch s.keyType {
	case KeyTypeEd25519:
		return ed25519.Verify(s.ed25519.Public().(ed25519.PublicKey), msg, sig)
	case KeyTypeRSA:
		digest := sha256.Sum256(msg)
		return rsa.VerifyPSS(s.rsa.Public().(*rsa.PublicKey), crypto.SHA256, digest[:], sig, &rsa.PSSOptions{
			SaltLength: rsa.PSSSaltLengthEqualsHash,
			Hash:       crypto.SHA256,
		}) == nil
	}
	return false
}
