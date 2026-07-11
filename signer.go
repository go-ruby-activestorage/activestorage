package activestorage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
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

// ErrExpired is returned by a Verifier when a token carried an expiry that has
// already passed (ActiveSupport::MessageVerifier::InvalidSignature on expiry).
var ErrExpired = errors.New("activestorage: signed message has expired")

// Verifier is the message-verifier seam for the parts of Active Storage whose
// payloads are typed values rather than opaque strings — a blob's signed_id (an
// integer id), a variation key (a transformations Hash), and the disk service's
// direct-upload/blob-key tokens (an ordered Hash). It mirrors
// ActiveSupport::MessageVerifier#generate / #verified with a purpose and an
// optional expiry, serializing the value the way Rails does. A Signer that also
// implements Verifier (RailsVerifier does) is used through this richer interface
// where a byte-exact wire format matters.
type Verifier interface {
	// Generate returns a signed token binding value to purpose, optionally
	// expiring at expiresAt (pass the zero time for no expiry).
	Generate(value any, purpose string, expiresAt time.Time) (string, error)
	// Verified checks a token's signature, purpose, and expiry, and returns the
	// serialized (JSON) form of the original value.
	Verified(token, purpose string) ([]byte, error)
}

// RailsVerifier reproduces ActiveSupport::MessageVerifier byte-for-byte for the
// serializer Rails uses by default (JSON) with a SHA-1 HMAC. Tokens have the wire
// form "<payload>--<hex-hmac>", where <payload> is url-safe (or strict) Base64 of
// either the bare serialized value or, when a purpose or expiry is present, the
// {"_rails":{"message":…,"exp":…,"pur":…}} metadata envelope. It implements both
// Signer (for string payloads) and Verifier (for typed payloads), so it can be a
// drop-in Config.Signer that makes blob signed_ids and variation keys match a
// running Rails app configured with the same secret.
type RailsVerifier struct {
	Secret  []byte
	URLSafe bool             // url-safe Base64 payload (Rails' message_verifier default)
	Now     func() time.Time // clock for expiry checks; defaults to time.Now
}

// NewRailsVerifier returns a RailsVerifier keyed by secret, using the url-safe
// Base64 encoding Rails' application message verifier uses.
func NewRailsVerifier(secret []byte) *RailsVerifier {
	return &RailsVerifier{Secret: secret, URLSafe: true}
}

func (v *RailsVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v *RailsVerifier) digest(data string) string {
	h := hmac.New(sha1.New, v.Secret)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func (v *RailsVerifier) encodePayload(data []byte) string {
	if v.URLSafe {
		return base64.RawURLEncoding.EncodeToString(data)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func (v *RailsVerifier) decodePayload(s string) ([]byte, error) {
	if v.URLSafe {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.StdEncoding.DecodeString(s)
}

// wrapMetadata builds the metadata envelope Rails uses when a purpose or expiry
// is present: {"_rails":{"message":<strict-base64 of the serialized value>,
// "exp":<null|ISO-8601 ms>,"pur":<purpose>}}. Every field is a known-safe string
// (or null), so it is written directly and cannot fail.
func wrapMetadata(serialized []byte, purpose string, expiresAt time.Time) []byte {
	var b bytes.Buffer
	b.WriteString(`{"_rails":{"message":`)
	rjString(&b, base64.StdEncoding.EncodeToString(serialized))
	b.WriteString(`,"exp":`)
	if expiresAt.IsZero() {
		b.WriteString("null")
	} else {
		rjString(&b, expiresAt.UTC().Format("2006-01-02T15:04:05.000Z"))
	}
	b.WriteString(`,"pur":`)
	rjString(&b, purpose)
	b.WriteString("}}")
	return b.Bytes()
}

// Generate implements Verifier.
func (v *RailsVerifier) Generate(value any, purpose string, expiresAt time.Time) (string, error) {
	serialized, err := rubyJSON(value)
	if err != nil {
		return "", err
	}
	payload := serialized
	if !(purpose == "" && expiresAt.IsZero()) {
		payload = wrapMetadata(serialized, purpose, expiresAt)
	}
	encoded := v.encodePayload(payload)
	return encoded + "--" + v.digest(encoded), nil
}

// Verified implements Verifier.
func (v *RailsVerifier) Verified(token, purpose string) ([]byte, error) {
	i := strings.LastIndex(token, "--")
	if i < 0 {
		return nil, ErrInvalidSignature
	}
	encoded, sig := token[:i], token[i+2:]
	if !hmac.Equal([]byte(v.digest(encoded)), []byte(sig)) {
		return nil, ErrInvalidSignature
	}
	payload, err := v.decodePayload(encoded)
	if err != nil {
		return nil, ErrInvalidSignature
	}
	if purpose == "" {
		return payload, nil
	}
	var env struct {
		Rails struct {
			Message string  `json:"message"`
			Exp     *string `json:"exp"`
			Pur     string  `json:"pur"`
		} `json:"_rails"`
	}
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, ErrInvalidSignature
	}
	if env.Rails.Pur != purpose {
		return nil, ErrInvalidSignature
	}
	if env.Rails.Exp != nil {
		exp, err := time.Parse("2006-01-02T15:04:05.000Z", *env.Rails.Exp)
		if err != nil {
			return nil, ErrInvalidSignature
		}
		if !v.now().UTC().Before(exp) {
			return nil, ErrExpired
		}
	}
	msg, err := base64.StdEncoding.DecodeString(env.Rails.Message)
	if err != nil {
		return nil, ErrInvalidSignature
	}
	return msg, nil
}

// Sign implements Signer by treating data as a string value (Rails serializes it
// as a JSON string). For typed payloads use Generate.
func (v *RailsVerifier) Sign(data, purpose string) (string, error) {
	return v.Generate(data, purpose, time.Time{})
}

// Verify implements Signer, returning the original string value.
func (v *RailsVerifier) Verify(token, purpose string) (string, error) {
	raw, err := v.Verified(token, purpose)
	if err != nil {
		return "", err
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", ErrInvalidSignature
	}
	return s, nil
}
