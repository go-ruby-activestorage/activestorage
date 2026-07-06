package activestorage

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testKey = "abcd1234567890abcd1234567890" // 28-char base36-shaped key

// okWriteCloser writes to a buffer and never errors.
type okWriteCloser struct{ buf bytes.Buffer }

func (w *okWriteCloser) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *okWriteCloser) Close() error                { return nil }

// closeErrWriteCloser writes fine but fails on Close.
type closeErrWriteCloser struct{ okWriteCloser }

func (w *closeErrWriteCloser) Close() error { return errInjected }

func TestDiskUploadDownloadRoundTrip(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	data := []byte("hello world")
	sum := checksumOf(data)
	if err := d.Upload(testKey, bytes.NewReader(data), sum); err != nil {
		t.Fatal(err)
	}
	// Verify the sharded path is Root/ab/cd/<key>.
	want := filepath.Join(d.Root(), "ab", "cd", testKey)
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected file at %s: %v", want, err)
	}
	rc, err := d.Download(testKey)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("Download = %q, want %q", got, data)
	}
	if n, err := d.Size(testKey); err != nil || n != int64(len(data)) {
		t.Fatalf("Size = %d, %v", n, err)
	}
}

func TestDiskUploadNoChecksum(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	if err := d.Upload(testKey, strings.NewReader("x"), ""); err != nil {
		t.Fatal(err)
	}
}

func TestDiskUploadChecksumMismatch(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	err := d.Upload(testKey, strings.NewReader("hello"), "not-the-checksum")
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	// The bad upload must not be left on disk.
	if ok, _ := d.Exist(testKey); ok {
		t.Fatal("mismatched upload was not removed")
	}
}

func TestDiskUploadMkdirError(t *testing.T) {
	base := t.TempDir()
	// Make Root a regular file so MkdirAll under it fails (ENOTDIR).
	fileAsRoot := filepath.Join(base, "notadir")
	if err := os.WriteFile(fileAsRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := NewDiskService("disk", fileAsRoot)
	if err := d.Upload(testKey, strings.NewReader("x"), ""); err == nil {
		t.Fatal("expected MkdirAll error")
	}
}

func TestDiskUploadCreateError(t *testing.T) {
	defer func(orig func(string) (io.WriteCloser, error)) { fsCreate = orig }(fsCreate)
	fsCreate = func(string) (io.WriteCloser, error) { return nil, errInjected }
	d := NewDiskService("disk", t.TempDir())
	if err := d.Upload(testKey, strings.NewReader("x"), ""); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestDiskUploadCopyError(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	if err := d.Upload(testKey, errReader{}, ""); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestDiskUploadCloseError(t *testing.T) {
	defer func(orig func(string) (io.WriteCloser, error)) { fsCreate = orig }(fsCreate)
	fsCreate = func(string) (io.WriteCloser, error) { return &closeErrWriteCloser{}, nil }
	d := NewDiskService("disk", t.TempDir())
	if err := d.Upload(testKey, strings.NewReader("x"), ""); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestDiskDownloadMissing(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	if _, err := d.Download(testKey); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestDiskDownloadChunk(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	data := []byte("0123456789")
	if err := d.Upload(testKey, bytes.NewReader(data), ""); err != nil {
		t.Fatal(err)
	}
	// Full in-range read.
	chunk, err := d.DownloadChunk(testKey, 2, 3)
	if err != nil || string(chunk) != "234" {
		t.Fatalf("chunk = %q, %v", chunk, err)
	}
	// Read past the end returns the available bytes (EOF is not an error).
	chunk, err = d.DownloadChunk(testKey, 8, 5)
	if err != nil || string(chunk) != "89" {
		t.Fatalf("tail chunk = %q, %v", chunk, err)
	}
	// A negative offset is a genuine (non-EOF) ReadAt error.
	if _, err := d.DownloadChunk(testKey, -1, 3); err == nil {
		t.Fatal("expected error for negative offset")
	}
	// Missing key -> open error.
	if _, err := d.DownloadChunk("zzzz1234567890zzzz1234567890", 0, 1); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestDiskDeleteIdempotent(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	if err := d.Upload(testKey, strings.NewReader("x"), ""); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(testKey); err != nil {
		t.Fatal(err)
	}
	// Deleting a missing key is not an error.
	if err := d.Delete(testKey); err != nil {
		t.Fatalf("second delete = %v", err)
	}
}

func TestDiskDeleteError(t *testing.T) {
	defer func(orig func(string) error) { fsRemove = orig }(fsRemove)
	fsRemove = func(string) error { return errInjected }
	d := NewDiskService("disk", t.TempDir())
	if err := d.Delete(testKey); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestDiskExist(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	if ok, err := d.Exist(testKey); ok || err != nil {
		t.Fatalf("Exist before upload = %v, %v", ok, err)
	}
	if err := d.Upload(testKey, strings.NewReader("x"), ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := d.Exist(testKey); !ok || err != nil {
		t.Fatalf("Exist after upload = %v, %v", ok, err)
	}
}

func TestDiskExistStatError(t *testing.T) {
	defer func(orig func(string) (os.FileInfo, error)) { fsStat = orig }(fsStat)
	fsStat = func(string) (os.FileInfo, error) { return nil, errInjected }
	d := NewDiskService("disk", t.TempDir())
	if _, err := d.Exist(testKey); !errors.Is(err, errInjected) {
		t.Fatalf("Exist err = %v, want errInjected", err)
	}
	if _, err := d.Size(testKey); !errors.Is(err, errInjected) {
		t.Fatalf("Size err = %v, want errInjected", err)
	}
}

func TestDiskURL(t *testing.T) {
	d := NewDiskService("disk", t.TempDir())
	// Defaults: inline disposition, filename falls back to the key.
	u, err := d.Url(testKey, URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, testKey) || !strings.Contains(u, "disposition=inline") || !strings.Contains(u, testKey+"%22") {
		t.Fatalf("default url = %q", u)
	}
	// Explicit options.
	u, err = d.Url(testKey, URLOptions{
		Disposition: "attachment",
		Filename:    NewFilename("report.pdf"),
		ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "attachment") || !strings.Contains(u, "report.pdf") || !strings.Contains(u, "content_type=application") {
		t.Fatalf("explicit url = %q", u)
	}
}
