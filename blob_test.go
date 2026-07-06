package activestorage

import (
	"bytes"
	"errors"
	"mime"
	"strings"
	"testing"
	"time"
)

func configWith(store ModelStore, svc Service) *Config {
	return &Config{
		Store:    store,
		Services: NewRegistry().Register(svc),
		Signer:   NewHMACSigner([]byte("s")),
	}
}

func TestChecksumMatchesRailsFormat(t *testing.T) {
	// Ruby: Digest::MD5.base64digest("hello world") => "XrY7u+Ae7tCTyyK7j1rNww=="
	if got := checksumOf([]byte("hello world")); got != "XrY7u+Ae7tCTyyK7j1rNww==" {
		t.Fatalf("checksum = %q, want XrY7u+Ae7tCTyyK7j1rNww==", got)
	}
}

func TestCreateAndUploadHappy(t *testing.T) {
	cfg, disk := newTestConfig(t)
	data := []byte("hello world")
	blob, err := cfg.CreateAndUpload(bytes.NewReader(data), BlobParams{
		Filename:    "greeting.txt",
		ContentType: "text/plain",
		Metadata:    map[string]any{"note": "hi"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if blob.ID == 0 {
		t.Fatal("blob not persisted (ID == 0)")
	}
	if blob.ByteSize != int64(len(data)) {
		t.Fatalf("ByteSize = %d", blob.ByteSize)
	}
	if blob.Checksum != "XrY7u+Ae7tCTyyK7j1rNww==" {
		t.Fatalf("Checksum = %q", blob.Checksum)
	}
	if len(blob.Key) != minimumKeyLength {
		t.Fatalf("Key len = %d", len(blob.Key))
	}
	if ok, _ := disk.Exist(blob.Key); !ok {
		t.Fatal("bytes not uploaded to service")
	}
	// Round-trip the content back through the model.
	got, err := blob.Download()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("Download = %q, %v", got, err)
	}
}

func TestBuildBlobDefaults(t *testing.T) {
	cfg, _ := newTestConfig(t)
	clock := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	cfg.Clock = func() time.Time { return clock }

	// ContentType inferred from a registered extension; metadata defaulted.
	_ = mime.AddExtensionType(".dat", "application/x-dat")
	b, err := cfg.buildBlob(BlobParams{Filename: "x.dat"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ContentType != "application/x-dat" {
		t.Fatalf("ContentType = %q", b.ContentType)
	}
	if b.Metadata == nil {
		t.Fatal("Metadata not defaulted")
	}
	if !b.CreatedAt.Equal(clock) {
		t.Fatalf("CreatedAt = %v, want %v", b.CreatedAt, clock)
	}
	if b.ServiceName != "test" {
		t.Fatalf("ServiceName = %q", b.ServiceName)
	}
}

func TestBuildBlobExplicit(t *testing.T) {
	cfg, _ := newTestConfig(t)
	b, err := cfg.buildBlob(BlobParams{
		Key:         "explicitkey0000000000000000",
		ServiceName: "test",
		ContentType: "image/png",
		Metadata:    map[string]any{"a": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Key != "explicitkey0000000000000000" || b.ContentType != "image/png" {
		t.Fatalf("explicit fields not honored: %+v", b)
	}
}

func TestBuildBlobKeyGenError(t *testing.T) {
	cfg, _ := newTestConfig(t)
	cfg.Random = &fakeRandom{bytesErr: errInjected}
	if _, err := cfg.buildBlob(BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBuildBlobNoDefaultService(t *testing.T) {
	cfg := &Config{Store: NewMemStore(), Services: NewRegistry(), Signer: NewHMACSigner([]byte("s"))}
	if _, err := cfg.buildBlob(BlobParams{Key: "k000000000000000000000000000"}); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("err = %v, want ErrServiceNotFound", err)
	}
}

func TestCreateAndUploadErrors(t *testing.T) {
	// unfurl (ReadAll) error.
	cfg, _ := newTestConfig(t)
	if _, err := cfg.CreateAndUpload(errReader{}, BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("unfurl err = %v", err)
	}

	// buildBlob error (random fails), with a valid reader.
	cfg2, _ := newTestConfig(t)
	cfg2.Random = &fakeRandom{bytesErr: errInjected}
	if _, err := cfg2.CreateAndUpload(strings.NewReader("x"), BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("build err = %v", err)
	}

	// InsertBlob error.
	fs := newFailStore()
	fs.insertBlobErr = errInjected
	cfg3 := configWith(fs, NewDiskService("test", t.TempDir()))
	if _, err := cfg3.CreateAndUpload(strings.NewReader("x"), BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("insert err = %v", err)
	}

	// Upload error.
	svc := newFakeService(t, "test")
	svc.uploadErr = errInjected
	cfg4 := configWith(NewMemStore(), svc)
	if _, err := cfg4.CreateAndUpload(strings.NewReader("x"), BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("upload err = %v", err)
	}
}

func TestBuildAfterUpload(t *testing.T) {
	cfg, disk := newTestConfig(t)
	blob, err := cfg.BuildAfterUpload(strings.NewReader("payload"), BlobParams{ServiceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if blob.ID != 0 {
		t.Fatal("BuildAfterUpload must not persist the row")
	}
	if ok, _ := disk.Exist(blob.Key); !ok {
		t.Fatal("bytes not uploaded")
	}

	// unfurl error.
	if _, err := cfg.BuildAfterUpload(errReader{}, BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("unfurl err = %v", err)
	}
	// buildBlob error.
	cfg2, _ := newTestConfig(t)
	cfg2.Random = &fakeRandom{bytesErr: errInjected}
	if _, err := cfg2.BuildAfterUpload(strings.NewReader("x"), BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("build err = %v", err)
	}
	// upload error.
	svc := newFakeService(t, "test")
	svc.uploadErr = errInjected
	cfg3 := configWith(NewMemStore(), svc)
	if _, err := cfg3.BuildAfterUpload(strings.NewReader("x"), BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("upload err = %v", err)
	}
}

func TestCreateBeforeDirectUpload(t *testing.T) {
	cfg, disk := newTestConfig(t)
	blob, err := cfg.CreateBeforeDirectUpload(BlobParams{
		Filename: "big.bin",
		ByteSize: 1024,
		Checksum: "precomputed==",
	})
	if err != nil {
		t.Fatal(err)
	}
	if blob.ID == 0 {
		t.Fatal("row not persisted")
	}
	// No bytes uploaded yet.
	if ok, _ := disk.Exist(blob.Key); ok {
		t.Fatal("direct-upload blob should have no stored bytes")
	}

	// buildBlob error.
	cfg2, _ := newTestConfig(t)
	cfg2.Random = &fakeRandom{bytesErr: errInjected}
	if _, err := cfg2.CreateBeforeDirectUpload(BlobParams{}); !errors.Is(err, errInjected) {
		t.Fatalf("build err = %v", err)
	}
	// InsertBlob error.
	fs := newFailStore()
	fs.insertBlobErr = errInjected
	cfg3 := configWith(fs, NewDiskService("test", t.TempDir()))
	if _, err := cfg3.CreateBeforeDirectUpload(BlobParams{Key: "k000000000000000000000000000"}); !errors.Is(err, errInjected) {
		t.Fatalf("insert err = %v", err)
	}
}

func TestFindBlobAndSignedID(t *testing.T) {
	cfg, _ := newTestConfig(t)
	blob, err := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{})
	if err != nil {
		t.Fatal(err)
	}
	found, err := cfg.FindBlob(blob.ID)
	if err != nil || found.ID != blob.ID {
		t.Fatalf("FindBlob = %v, %v", found, err)
	}

	sid, err := blob.SignedID()
	if err != nil {
		t.Fatal(err)
	}
	signed, err := cfg.FindSignedBlob(sid)
	if err != nil || signed.ID != blob.ID {
		t.Fatalf("FindSignedBlob = %v, %v", signed, err)
	}

	// Verify failure.
	if _, err := cfg.FindSignedBlob("not-a-token"); err == nil {
		t.Fatal("expected verify error")
	}
	// Parse failure: a validly-signed but non-numeric payload.
	bad, _ := cfg.Signer.Sign("not-a-number", signedIDPurpose)
	if _, err := cfg.FindSignedBlob(bad); err == nil {
		t.Fatal("expected parse error")
	}
	// FindBlob failure: signed id for a row that no longer exists.
	gone, _ := cfg.Signer.Sign("9999", signedIDPurpose)
	if _, err := cfg.FindSignedBlob(gone); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("err = %v, want ErrBlobNotFound", err)
	}
}

func TestBlobUpload(t *testing.T) {
	cfg, disk := newTestConfig(t)
	blob, err := cfg.CreateBeforeDirectUpload(BlobParams{Filename: "f.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := blob.Upload(strings.NewReader("late bytes")); err != nil {
		t.Fatal(err)
	}
	if blob.ByteSize != int64(len("late bytes")) {
		t.Fatalf("ByteSize = %d", blob.ByteSize)
	}
	if ok, _ := disk.Exist(blob.Key); !ok {
		t.Fatal("bytes not uploaded")
	}

	// ReadAll error.
	if err := blob.Upload(errReader{}); !errors.Is(err, errInjected) {
		t.Fatalf("upload readall err = %v", err)
	}
	// Service resolution error (uploadBytes -> Service).
	blob.ServiceName = "missing"
	if err := blob.Upload(strings.NewReader("x")); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("upload service err = %v", err)
	}
}

func TestBlobDownloadAndChunk(t *testing.T) {
	cfg, _ := newTestConfig(t)
	blob, err := cfg.CreateAndUpload(strings.NewReader("0123456789"), BlobParams{})
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := blob.DownloadChunk(3, 4)
	if err != nil || string(chunk) != "3456" {
		t.Fatalf("chunk = %q, %v", chunk, err)
	}

	// Service resolution errors.
	bad := &Blob{ServiceName: "nope", cfg: cfg}
	if _, err := bad.Download(); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Download service err = %v", err)
	}
	if _, err := bad.DownloadChunk(0, 1); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Chunk service err = %v", err)
	}

	// Download stream error.
	svc := newFakeService(t, "test")
	svc.downloadErr = errInjected
	cfg2 := configWith(NewMemStore(), svc)
	b2 := &Blob{ServiceName: "test", Key: testKey, cfg: cfg2}
	if _, err := b2.Download(); !errors.Is(err, errInjected) {
		t.Fatalf("Download err = %v", err)
	}
}

func TestBlobURL(t *testing.T) {
	cfg, _ := newTestConfig(t)
	blob, err := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{
		Filename:    "doc.pdf",
		ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Defaults pulled from the blob.
	u, err := blob.URL(URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "doc.pdf") || !strings.Contains(u, "inline") || !strings.Contains(u, "content_type=application") {
		t.Fatalf("default url = %q", u)
	}
	// Explicit overrides.
	u, err = blob.URL(URLOptions{Disposition: "attachment", Filename: NewFilename("other.pdf"), ContentType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "other.pdf") || !strings.Contains(u, "attachment") {
		t.Fatalf("explicit url = %q", u)
	}
	// Service error.
	bad := &Blob{ServiceName: "nope", cfg: cfg}
	if _, err := bad.URL(URLOptions{}); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("URL service err = %v", err)
	}
}

func TestBlobPurge(t *testing.T) {
	cfg, disk := newTestConfig(t)
	blob, err := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{})
	if err != nil {
		t.Fatal(err)
	}
	if err := blob.Purge(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := disk.Exist(blob.Key); ok {
		t.Fatal("object not removed from service")
	}
	if _, err := cfg.FindBlob(blob.ID); !errors.Is(err, ErrBlobNotFound) {
		t.Fatal("row not removed")
	}

	// Service resolution error.
	bad := &Blob{ServiceName: "nope", cfg: cfg}
	if err := bad.Purge(); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("Purge service err = %v", err)
	}

	// Service delete error.
	svc := newFakeService(t, "test")
	svc.deleteErr = errInjected
	cfg2 := configWith(NewMemStore(), svc)
	b2 := &Blob{ServiceName: "test", Key: testKey, cfg: cfg2}
	if err := b2.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge delete err = %v", err)
	}

	// Store delete error (service delete succeeds).
	fs := newFailStore()
	fs.deleteBlobErr = errInjected
	cfg3 := configWith(fs, NewDiskService("test", t.TempDir()))
	b3, err := cfg3.CreateAndUpload(strings.NewReader("x"), BlobParams{})
	if err != nil {
		t.Fatal(err)
	}
	if err := b3.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge store err = %v", err)
	}
}
