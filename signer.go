package activestorage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// ErrInvalidSignature is returned by a Signer.Verify when a token is malformed,
// tampered with, or signed for a different purpose.
var ErrInvalidSignature = errors.New("activestorage: invalid signature")

// Signer is the message-signing seam. In Rails a blob's signed_id is produced by
// ActiveSupport::MessageVerifier; hosts can plug their own verifier here so signed
// ids interoperate with an existing application. The purpose argument scopes a
// token (ActiveStorage uses "blob_id") so a token minted for one purpose cannot
// be replayed for another.
type Signer interface {
	Sign(data, purpose string) (string, error)
	Verify(token, purpose string) (string, error)
}

// HMACSigner is the default Signer: it authenticates a payload with HMAC-SHA256
// and encodes the token as "purpose.base64url(data).base64url(mac)". It is not a
// drop-in for Rails' exact wire format (that is a MessageVerifier concern), but
// it is a complete, self-contained default for standalone use.
type HMACSigner struct {
	Secret []byte
}

// NewHMACSigner returns an HMACSigner keyed by secret.
func NewHMACSigner(secret []byte) *HMACSigner { return &HMACSigner{Secret: secret} }

func (s *HMACSigner) mac(msg string) string {
	h := hmac.New(sha256.New, s.Secret)
	h.Write([]byte(msg))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// Sign returns a signed token binding data to purpose.
func (s *HMACSigner) Sign(data, purpose string) (string, error) {
	payload := purpose + "." + base64.RawURLEncoding.EncodeToString([]byte(data))
	return payload + "." + s.mac(payload), nil
}

// Verify checks a token's signature and purpose, returning the original data.
func (s *HMACSigner) Verify(token, purpose string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidSignature
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidSignature
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(s.mac(payload)), []byte(parts[2])) {
		return "", ErrInvalidSignature
	}
	if parts[0] != purpose {
		return "", ErrInvalidSignature
	}
	return string(data), nil
}
