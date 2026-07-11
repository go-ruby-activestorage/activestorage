package activestorage

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignedIDRailsRoundTrip(t *testing.T) {
	cfg, _ := railsConfig(t)
	b, err := cfg.CreateAndUpload(strings.NewReader("hi"), BlobParams{Filename: "a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	sid, err := b.SignedID()
	if err != nil {
		t.Fatal(err)
	}
	again, err := cfg.FindSignedBlob(sid)
	if err != nil || again.ID != b.ID {
		t.Fatalf("round trip = %v, %v", again, err)
	}
}

func TestFindSignedBlobRailsErrors(t *testing.T) {
	cfg, _ := railsConfig(t)
	v := cfg.Signer.(*RailsVerifier)

	// Verified failure (tampered token).
	if _, err := cfg.FindSignedBlob("garbage"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("verify err = %v", err)
	}
	// Parse failure: a validly signed but non-numeric (JSON string) payload.
	bad, _ := v.Generate("not-a-number", signedIDPurpose, time.Time{})
	if _, err := cfg.FindSignedBlob(bad); err == nil {
		t.Fatal("expected parse error")
	}
	// FindBlob failure: signed id for a row that does not exist.
	gone, _ := v.Generate(int64(9999), signedIDPurpose, time.Time{})
	if _, err := cfg.FindSignedBlob(gone); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("err = %v, want ErrBlobNotFound", err)
	}
}

// errVerifier is a Verifier whose Generate always fails.
type errVerifier struct{}

func (errVerifier) Generate(any, string, time.Time) (string, error) { return "", errInjected }
func (errVerifier) Verified(string, string) ([]byte, error)         { return nil, errInjected }
func (errVerifier) Sign(string, string) (string, error)             { return "", errInjected }
func (errVerifier) Verify(string, string) (string, error)           { return "", errInjected }

func TestDiskServiceDirectUploadGenerateError(t *testing.T) {
	disk := NewDiskService("local", t.TempDir())
	disk.Verifier = errVerifier{}
	if _, err := disk.URLForDirectUpload("k", DirectUploadOptions{}); !errors.Is(err, errInjected) {
		t.Fatalf("generate err = %v", err)
	}
}

func TestBlobDirectUploadNotDirectUploadable(t *testing.T) {
	// A service that implements Service but not DirectUploadService.
	svc := newFakeService(t, "test")
	cfg := &Config{
		Store:    NewMemStore(),
		Services: NewRegistry().Register(svc),
		Signer:   NewRailsVerifier(oracleSecret()),
	}
	b, _ := cfg.CreateBeforeDirectUpload(BlobParams{Filename: "x", ByteSize: 1, Checksum: "c"})
	if _, err := b.DirectUpload(0); !errors.Is(err, ErrNotDirectUploadable) {
		t.Fatalf("err = %v, want ErrNotDirectUploadable", err)
	}
}

func TestBlobDirectUploadURLError(t *testing.T) {
	// A blob whose service is a DirectUploadService but whose token minting fails
	// exercises Blob.DirectUpload's URL error branch.
	disk := NewDiskService("local", t.TempDir())
	disk.Verifier = errVerifier{}
	cfg := &Config{
		Store:    NewMemStore(),
		Services: NewRegistry().Register(disk),
		Signer:   NewRailsVerifier(oracleSecret()),
	}
	b, _ := cfg.CreateBeforeDirectUpload(BlobParams{Filename: "x", ByteSize: 1, Checksum: "c"})
	if _, err := b.DirectUpload(0); !errors.Is(err, errInjected) {
		t.Fatalf("url err = %v", err)
	}
}

// ContentType detection is wired into the upload path: an unidentified upload of
// PNG magic bytes is stored as image/png even with a misleading name.
func TestBlobContentTypeDetection(t *testing.T) {
	cfg, _ := railsConfig(t)
	b, err := cfg.CreateAndUpload(strings.NewReader("\x89PNG\r\n\x1a\nrest"), BlobParams{Filename: "mystery"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ContentType != "image/png" {
		t.Fatalf("detected content type = %q", b.ContentType)
	}

	// identify:false (default when a content type is supplied) trusts the caller.
	b2, _ := cfg.CreateAndUpload(strings.NewReader("\x89PNG\r\n\x1a\n"), BlobParams{Filename: "x", ContentType: "application/custom"})
	if b2.ContentType != "application/custom" {
		t.Fatalf("trusted content type = %q", b2.ContentType)
	}

	// Identify:true forces re-detection over a supplied content type.
	b3, _ := cfg.CreateAndUpload(strings.NewReader("%PDF-1.7"), BlobParams{Filename: "x", ContentType: "application/custom", Identify: true})
	if b3.ContentType != "application/pdf" {
		t.Fatalf("re-identified content type = %q", b3.ContentType)
	}
}
