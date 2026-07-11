package activestorage

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// oracleSecret is Ruby's "0"*64 — the secret used to capture the golden tokens
// from ActiveSupport::MessageVerifier.new(secret, serializer: JSON, url_safe: true).
func oracleSecret() []byte {
	s := make([]byte, 64)
	for i := range s {
		s[i] = '0'
	}
	return s
}

func TestRailsVerifierGoldenTokens(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())

	// Bare value, no purpose/expiry: url-safe base64 of the JSON plus SHA1 HMAC.
	if got, _ := v.Generate("123", "", time.Time{}); got != "IjEyMyI--50aa660eb3d66633a42754d65ff18b169e2ed9d5" {
		t.Fatalf("plain = %s", got)
	}
	// Integer id with purpose "blob_id" — a blob's signed_id.
	if got, _ := v.Generate(int64(123), "blob_id", time.Time{}); got != "eyJfcmFpbHMiOnsibWVzc2FnZSI6Ik1USXoiLCJleHAiOm51bGwsInB1ciI6ImJsb2JfaWQifX0--9884475c91ef73aba1b2392795ed6d8aff402151" {
		t.Fatalf("id123 = %s", got)
	}
	if got, _ := v.Generate(int64(1), "blob_id", time.Time{}); got != "eyJfcmFpbHMiOnsibWVzc2FnZSI6Ik1RPT0iLCJleHAiOm51bGwsInB1ciI6ImJsb2JfaWQifX0--c8f1c7cbaf80c4a64e24c6278b8c000401ae6beb" {
		t.Fatalf("id1 = %s", got)
	}
	// String value with purpose and an explicit expiry.
	exp := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, _ := v.Generate("123", "blob_id", exp); got != "eyJfcmFpbHMiOnsibWVzc2FnZSI6IklqRXlNeUk9IiwiZXhwIjoiMjAzMC0wMS0wMVQwMDowMDowMC4wMDBaIiwicHVyIjoiYmxvYl9pZCJ9fQ--fa8eb1ae604c1f181e0b2112e4cdb59e2c4f212d" {
		t.Fatalf("exp = %s", got)
	}
}

func TestRailsVerifierRoundTrip(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	tok, _ := v.Generate(int64(42), "blob_id", time.Time{})
	raw, err := v.Verified(tok, "blob_id")
	if err != nil || string(raw) != "42" {
		t.Fatalf("verified = %q, %v", raw, err)
	}
	// Plain (no purpose) round trip returns the raw serialized value.
	pt, _ := v.Generate("hi", "", time.Time{})
	praw, err := v.Verified(pt, "")
	if err != nil || string(praw) != `"hi"` {
		t.Fatalf("plain verified = %q, %v", praw, err)
	}
}

func TestRailsVerifierSignVerifyString(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	tok, err := v.Sign("hello", "greeting")
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Verify(tok, "greeting")
	if err != nil || got != "hello" {
		t.Fatalf("verify = %q, %v", got, err)
	}
	// Verify of a non-string payload fails to unmarshal into a string.
	num, _ := v.Generate(int64(7), "greeting", time.Time{})
	if _, err := v.Verify(num, "greeting"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("verify(num) err = %v", err)
	}
	// Verify surfaces a Verified failure (tampered token).
	if _, err := v.Verify("garbage", "greeting"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("verify(garbage) err = %v", err)
	}
}

func TestRailsVerifierNonURLSafe(t *testing.T) {
	v := &RailsVerifier{Secret: oracleSecret(), URLSafe: false}
	tok, _ := v.Generate("hi", "p", time.Time{})
	if raw, err := v.Verified(tok, "p"); err != nil || string(raw) != `"hi"` {
		t.Fatalf("non-urlsafe round trip = %q, %v", raw, err)
	}
}

func TestRailsVerifierExpiry(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	// Fix "now" so an in-the-past expiry is rejected and a future one accepted.
	v.Now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
	past, _ := v.Generate("x", "p", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, err := v.Verified(past, "p"); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired err = %v", err)
	}
	future, _ := v.Generate("x", "p", time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
	if _, err := v.Verified(future, "p"); err != nil {
		t.Fatalf("future err = %v", err)
	}
}

func TestRailsVerifierDefaultClock(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	// No Now set -> now() uses time.Now; a far-future expiry must verify.
	tok, _ := v.Generate("x", "p", time.Now().Add(time.Hour))
	if _, err := v.Verified(tok, "p"); err != nil {
		t.Fatalf("default clock err = %v", err)
	}
}

func TestRailsVerifierVerifiedErrors(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	good, _ := v.Generate(int64(1), "blob_id", time.Time{})

	// No separator.
	if _, err := v.Verified("nodashes", "blob_id"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("no sep = %v", err)
	}
	// Tampered signature.
	if _, err := v.Verified(good[:len(good)-1]+"0", "blob_id"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("bad sig = %v", err)
	}
	// Wrong purpose.
	if _, err := v.Verified(good, "other"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("bad purpose = %v", err)
	}
	// Undecodable payload base64: keep a valid-looking signature over garbage.
	bad := "@@@--" + v.digest("@@@")
	if _, err := v.Verified(bad, ""); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("bad b64 = %v", err)
	}
	// Purpose requested but payload is not a metadata envelope (plain JSON).
	plain, _ := v.Generate("x", "", time.Time{})
	if _, err := v.Verified(plain, "blob_id"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("plain-as-envelope = %v", err)
	}
}

func TestRailsVerifierBadExpFormat(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	// Hand-build an envelope whose exp is not ISO-8601, then sign it.
	env := Hash{{Key: "_rails", Value: Hash{
		{Key: "message", Value: "MQ=="},
		{Key: "exp", Value: "not-a-date"},
		{Key: "pur", Value: "p"},
	}}}
	payload, _ := rubyJSON(env)
	enc := v.encodePayload(payload)
	tok := enc + "--" + v.digest(enc)
	if _, err := v.Verified(tok, "p"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("bad exp = %v", err)
	}
}

func TestRailsVerifierBadMessageBase64(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	env := Hash{{Key: "_rails", Value: Hash{
		{Key: "message", Value: "!!!not-base64"},
		{Key: "exp", Value: nil},
		{Key: "pur", Value: "p"},
	}}}
	payload, _ := rubyJSON(env)
	enc := v.encodePayload(payload)
	tok := enc + "--" + v.digest(enc)
	if _, err := v.Verified(tok, "p"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("bad msg b64 = %v", err)
	}
}

func TestRailsVerifierGenerateError(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	if _, err := v.Generate(notEncodable{}, "", time.Time{}); err == nil {
		t.Fatal("expected serialize error (plain)")
	}
	if _, err := v.Generate(notEncodable{}, "p", time.Time{}); err == nil {
		t.Fatal("expected serialize error (envelope)")
	}
}

func TestRailsVerifierImplementsInterfaces(t *testing.T) {
	var _ Signer = (*RailsVerifier)(nil)
	var _ Verifier = (*RailsVerifier)(nil)
	// A base64url payload can legitimately contain "--"; LastIndex must find the
	// real separator (the hex digest never contains '-').
	v := NewRailsVerifier(oracleSecret())
	tok, _ := v.Generate("anything", "", time.Time{})
	if !strings.Contains(tok, "--") {
		t.Fatal("token missing separator")
	}
}
