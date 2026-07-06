package activestorage

import (
	"errors"
	"strings"
	"testing"
)

func TestHMACSignerRoundTrip(t *testing.T) {
	s := NewHMACSigner([]byte("secret"))
	tok, err := s.Sign("42", "blob_id")
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.Verify(tok, "blob_id")
	if err != nil {
		t.Fatal(err)
	}
	if data != "42" {
		t.Fatalf("data = %q, want 42", data)
	}
}

func TestHMACSignerWrongParts(t *testing.T) {
	s := NewHMACSigner([]byte("secret"))
	if _, err := s.Verify("only.two", "blob_id"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestHMACSignerBadBase64(t *testing.T) {
	s := NewHMACSigner([]byte("secret"))
	// parts[1] "!!!" is not valid base64url.
	if _, err := s.Verify("blob_id.!!!.sig", "blob_id"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestHMACSignerTampered(t *testing.T) {
	s := NewHMACSigner([]byte("secret"))
	tok, _ := s.Sign("42", "blob_id")
	// Replace the MAC with a valid-base64 but wrong signature.
	parts := strings.Split(tok, ".")
	bad := parts[0] + "." + parts[1] + ".AAAA"
	if _, err := s.Verify(bad, "blob_id"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}

func TestHMACSignerWrongPurpose(t *testing.T) {
	s := NewHMACSigner([]byte("secret"))
	tok, _ := s.Sign("42", "blob_id")
	if _, err := s.Verify(tok, "other"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v, want ErrInvalidSignature", err)
	}
}
