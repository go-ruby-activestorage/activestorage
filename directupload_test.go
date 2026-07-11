package activestorage

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDiskServiceDirectUploadToken(t *testing.T) {
	disk := NewDiskService("local", t.TempDir())
	disk.Verifier = NewRailsVerifier(oracleSecret())

	url, err := disk.URLForDirectUpload("thekey", DirectUploadOptions{
		ContentType:   "text/plain",
		ContentLength: 11,
		Checksum:      "XrY7u+Ae7tCTyyK7j1rNww==",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "/rails/active_storage/disk/") {
		t.Fatalf("url = %s", url)
	}
	// The token is a verifiable blob_token whose payload round-trips.
	token := strings.TrimPrefix(url, "/rails/active_storage/disk/")
	raw, err := disk.Verifier.Verified(token, "blob_token")
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if !strings.Contains(string(raw), `"key":"thekey"`) || !strings.Contains(string(raw), `"service_name":"local"`) {
		t.Fatalf("token payload = %s", raw)
	}

	// Headers.
	h := disk.HeadersForDirectUpload("thekey", "text/plain")
	if h["Content-Type"] != "text/plain" {
		t.Fatalf("headers = %v", h)
	}
}

func TestDiskServiceDirectUploadExpiry(t *testing.T) {
	disk := NewDiskService("local", t.TempDir())
	disk.Verifier = NewRailsVerifier(oracleSecret())
	disk.Clock = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
	disk.URLPrefix = "/custom/"

	url, err := disk.URLForDirectUpload("k", DirectUploadOptions{ExpiresIn: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "/custom/") {
		t.Fatalf("prefix not honored: %s", url)
	}
}

func TestDiskServiceDirectUploadNoVerifier(t *testing.T) {
	disk := NewDiskService("local", t.TempDir())
	if _, err := disk.URLForDirectUpload("k", DirectUploadOptions{}); !errors.Is(err, ErrNotDirectUploadable) {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigCreateForDirectUpload(t *testing.T) {
	cfg, _ := railsConfig(t)
	blob, du, err := cfg.CreateForDirectUpload(BlobParams{
		Filename: "big.bin",
		ByteSize: 1024,
		Checksum: "precomputed==",
	}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if blob.ID == 0 {
		t.Fatal("blob not persisted")
	}
	if du.SignedID == "" || du.URL == "" || du.Headers["Content-Type"] == "" {
		t.Fatalf("direct upload = %+v", du)
	}
	// The signed id resolves back to the blob.
	again, err := cfg.FindSignedBlob(du.SignedID)
	if err != nil || again.ID != blob.ID {
		t.Fatalf("resolve signed id = %v, %v", again, err)
	}
}

func TestDirectUploadErrors(t *testing.T) {
	// CreateBeforeDirectUpload error (random key gen fails).
	cfg, _ := railsConfig(t)
	cfg.Random = &fakeRandom{bytesErr: errInjected}
	if _, _, err := cfg.CreateForDirectUpload(BlobParams{}, 0); !errors.Is(err, errInjected) {
		t.Fatalf("create err = %v", err)
	}

	// Service does not implement DirectUploadService.
	plainDisk := NewDiskService("test", t.TempDir()) // no Verifier
	cfg2 := &Config{Store: NewMemStore(), Services: NewRegistry().Register(plainDisk), Signer: NewRailsVerifier(oracleSecret())}
	b2, _ := cfg2.CreateBeforeDirectUpload(BlobParams{Filename: "x", ByteSize: 1, Checksum: "c"})
	if _, err := b2.DirectUpload(0); !errors.Is(err, ErrNotDirectUploadable) {
		t.Fatalf("not-uploadable err = %v", err)
	}
	if _, _, err := cfg2.CreateForDirectUpload(BlobParams{Filename: "x", ByteSize: 1, Checksum: "c"}, 0); !errors.Is(err, ErrNotDirectUploadable) {
		t.Fatalf("create not-uploadable err = %v", err)
	}

	// SignedID error: a non-Verifier signer whose Sign fails.
	cfg3 := &Config{Store: NewMemStore(), Services: NewRegistry().Register(NewDiskService("test", t.TempDir())), Signer: failSigner{}}
	b3, _ := cfg3.CreateBeforeDirectUpload(BlobParams{Filename: "x", ByteSize: 1, Checksum: "c"})
	if _, err := b3.DirectUpload(0); !errors.Is(err, errInjected) {
		t.Fatalf("signed id err = %v", err)
	}

	// Service resolution error.
	cfg4, _ := railsConfig(t)
	b4, _ := cfg4.CreateBeforeDirectUpload(BlobParams{Filename: "x", ByteSize: 1, Checksum: "c"})
	b4.ServiceName = "missing"
	if _, err := b4.DirectUpload(0); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("service err = %v", err)
	}
}

// failSigner is a Signer (not a Verifier) whose Sign always fails, to exercise
// the SignedID error path.
type failSigner struct{}

func (failSigner) Sign(string, string) (string, error)   { return "", errInjected }
func (failSigner) Verify(string, string) (string, error) { return "", errInjected }
