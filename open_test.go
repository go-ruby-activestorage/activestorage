package activestorage

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// fakeTemp is an injectable temp file for Blob.Open's seam tests.
type fakeTemp struct {
	name     string
	writeErr error
	closeErr error
}

func (f *fakeTemp) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}
func (f *fakeTemp) Close() error { return f.closeErr }
func (f *fakeTemp) Name() string { return f.name }

func newDiskBlob(t *testing.T, content string) (*Config, *Blob) {
	t.Helper()
	cfg, _ := newTestConfig(t)
	blob, err := cfg.CreateAndUpload(strings.NewReader(content), BlobParams{Filename: "f.txt"})
	if err != nil {
		t.Fatal(err)
	}
	return cfg, blob
}

func TestBlobOpenHappy(t *testing.T) {
	_, blob := newDiskBlob(t, "streamed content")
	var seen string
	err := blob.Open(func(f *os.File) error {
		b, err := io.ReadAll(f)
		seen = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != "streamed content" {
		t.Fatalf("read %q from temp file", seen)
	}
}

func TestBlobOpenDownloadError(t *testing.T) {
	svc := newFakeService(t, "test")
	svc.downloadErr = errInjected
	cfg := configWith(NewMemStore(), svc)
	b := &Blob{ServiceName: "test", Key: testKey, cfg: cfg}
	if err := b.Open(func(*os.File) error { return nil }); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBlobOpenChecksumMismatch(t *testing.T) {
	_, blob := newDiskBlob(t, "content")
	blob.Checksum = "wrong=="
	if err := blob.Open(func(*os.File) error { return nil }); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
}

func TestBlobOpenFnError(t *testing.T) {
	_, blob := newDiskBlob(t, "content")
	if err := blob.Open(func(*os.File) error { return errInjected }); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBlobOpenCreateTempError(t *testing.T) {
	defer func(orig func(string, string) (namedWriteCloser, error)) { fsCreateTemp = orig }(fsCreateTemp)
	fsCreateTemp = func(string, string) (namedWriteCloser, error) { return nil, errInjected }
	_, blob := newDiskBlob(t, "content")
	if err := blob.Open(func(*os.File) error { return nil }); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBlobOpenWriteError(t *testing.T) {
	defer func(orig func(string, string) (namedWriteCloser, error)) { fsCreateTemp = orig }(fsCreateTemp)
	fsCreateTemp = func(string, string) (namedWriteCloser, error) {
		return &fakeTemp{name: "irrelevant", writeErr: errInjected}, nil
	}
	_, blob := newDiskBlob(t, "content")
	if err := blob.Open(func(*os.File) error { return nil }); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBlobOpenCloseError(t *testing.T) {
	defer func(orig func(string, string) (namedWriteCloser, error)) { fsCreateTemp = orig }(fsCreateTemp)
	fsCreateTemp = func(string, string) (namedWriteCloser, error) {
		return &fakeTemp{name: "irrelevant", closeErr: errInjected}, nil
	}
	_, blob := newDiskBlob(t, "content")
	if err := blob.Open(func(*os.File) error { return nil }); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBlobOpenReopenError(t *testing.T) {
	defer func(orig func(string, string) (namedWriteCloser, error)) { fsCreateTemp = orig }(fsCreateTemp)
	// Write and Close succeed, but Name() points nowhere so os.Open fails.
	fsCreateTemp = func(string, string) (namedWriteCloser, error) {
		return &fakeTemp{name: "/nonexistent/definitely/missing/file"}, nil
	}
	_, blob := newDiskBlob(t, "content")
	if err := blob.Open(func(*os.File) error { return nil }); err == nil {
		t.Fatal("expected os.Open error")
	}
}
