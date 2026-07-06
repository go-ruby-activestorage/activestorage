package activestorage

import (
	"errors"
	"io"
	"testing"
)

var errInjected = errors.New("injected error")

// fakeRandom is a deterministic RandomSource for tests. It serves a fixed byte
// slice (padded with zeros) and a cycling list of numbers, with optional errors.
type fakeRandom struct {
	bytesVal []byte
	bytesErr error
	numbers  []int
	numIdx   int
	numErr   error
}

func (f *fakeRandom) RandomBytes(n int) ([]byte, error) {
	if f.bytesErr != nil {
		return nil, f.bytesErr
	}
	out := make([]byte, n)
	copy(out, f.bytesVal)
	return out, nil
}

func (f *fakeRandom) RandomNumber(max int) (int, error) {
	if f.numErr != nil {
		return 0, f.numErr
	}
	if len(f.numbers) == 0 {
		return 0, nil
	}
	v := f.numbers[f.numIdx%len(f.numbers)]
	f.numIdx++
	return v, nil
}

// fakeService is a Service whose every method can be made to fail, for exercising
// the model logic's error branches. Non-failing methods delegate to an embedded
// DiskService created under a temp dir.
type fakeService struct {
	disk        *DiskService
	uploadErr   error
	downloadErr error
	chunkErr    error
	deleteErr   error
	existErr    error
	urlErr      error
	sizeErr     error
}

func newFakeService(t *testing.T, name string) *fakeService {
	t.Helper()
	return &fakeService{disk: NewDiskService(name, t.TempDir())}
}

func (f *fakeService) Name() string { return f.disk.Name() }

func (f *fakeService) Upload(key string, r io.Reader, checksum string) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	return f.disk.Upload(key, r, checksum)
}

func (f *fakeService) Download(key string) (io.ReadCloser, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return f.disk.Download(key)
}

func (f *fakeService) DownloadChunk(key string, offset, length int64) ([]byte, error) {
	if f.chunkErr != nil {
		return nil, f.chunkErr
	}
	return f.disk.DownloadChunk(key, offset, length)
}

func (f *fakeService) Delete(key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.disk.Delete(key)
}

func (f *fakeService) Exist(key string) (bool, error) {
	if f.existErr != nil {
		return false, f.existErr
	}
	return f.disk.Exist(key)
}

func (f *fakeService) Url(key string, opts URLOptions) (string, error) {
	if f.urlErr != nil {
		return "", f.urlErr
	}
	return f.disk.Url(key, opts)
}

func (f *fakeService) Size(key string) (int64, error) {
	if f.sizeErr != nil {
		return 0, f.sizeErr
	}
	return f.disk.Size(key)
}

// failStore wraps a MemStore and injects errors on selected operations.
type failStore struct {
	*MemStore
	insertBlobErr error
	findBlobErr   error
	updateBlobErr error
	deleteBlobErr error
	insertAttErr  error
	findAttErr    error
	deleteAttErr  error
}

func newFailStore() *failStore { return &failStore{MemStore: NewMemStore()} }

func (s *failStore) InsertBlob(b *Blob) error {
	if s.insertBlobErr != nil {
		return s.insertBlobErr
	}
	return s.MemStore.InsertBlob(b)
}

func (s *failStore) FindBlob(id int64) (*Blob, error) {
	if s.findBlobErr != nil {
		return nil, s.findBlobErr
	}
	return s.MemStore.FindBlob(id)
}

func (s *failStore) UpdateBlob(b *Blob) error {
	if s.updateBlobErr != nil {
		return s.updateBlobErr
	}
	return s.MemStore.UpdateBlob(b)
}

func (s *failStore) DeleteBlob(id int64) error {
	if s.deleteBlobErr != nil {
		return s.deleteBlobErr
	}
	return s.MemStore.DeleteBlob(id)
}

func (s *failStore) InsertAttachment(a *Attachment) error {
	if s.insertAttErr != nil {
		return s.insertAttErr
	}
	return s.MemStore.InsertAttachment(a)
}

func (s *failStore) FindAttachments(recordType string, recordID int64, name string) ([]*Attachment, error) {
	if s.findAttErr != nil {
		return nil, s.findAttErr
	}
	return s.MemStore.FindAttachments(recordType, recordID, name)
}

func (s *failStore) DeleteAttachment(id int64) error {
	if s.deleteAttErr != nil {
		return s.deleteAttErr
	}
	return s.MemStore.DeleteAttachment(id)
}

// errReader always fails on Read, for exercising io error branches.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errInjected }

// newTestConfig returns a Config wired to a MemStore, a registered DiskService
// (under a temp dir) named "test", an HMAC signer, and a deterministic clock.
func newTestConfig(t *testing.T) (*Config, *DiskService) {
	t.Helper()
	disk := NewDiskService("test", t.TempDir())
	reg := NewRegistry().Register(disk)
	return &Config{
		Store:    NewMemStore(),
		Services: reg,
		Signer:   NewHMACSigner([]byte("test-secret")),
	}, disk
}
