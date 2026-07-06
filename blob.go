package activestorage

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// fsCreateTemp is the temp-file creation seam used by Blob.Open, indirected so the
// suite can exercise its error paths (see the fs* seams in disk_service.go).
var fsCreateTemp = func(dir, pattern string) (namedWriteCloser, error) {
	return os.CreateTemp(dir, pattern)
}

// namedWriteCloser is the subset of *os.File that Blob.Open needs from a temp
// file: write, close, and report its path.
type namedWriteCloser interface {
	io.WriteCloser
	Name() string
}

// signedIDPurpose scopes a blob's signed id, matching ActiveStorage's :blob_id.
const signedIDPurpose = "blob_id"

// Config bundles the pluggable seams the model logic depends on: the persistence
// store (ActiveRecord), the service registry, the signer (signed ids), the
// randomness source (storage keys), and a clock. It is the entry point for
// creating and finding blobs and attachments.
type Config struct {
	Store    ModelStore
	Services *Registry
	Signer   Signer
	Random   RandomSource     // defaults to DefaultRandom() when nil
	Clock    func() time.Time // defaults to time.Now().UTC when nil
}

func (c *Config) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now().UTC()
}

func (c *Config) random() RandomSource {
	if c.Random != nil {
		return c.Random
	}
	return DefaultRandom()
}

// Blob is the persisted description of an uploaded file: where it lives (Key,
// ServiceName), what it is (Filename, ContentType, Metadata), and how to verify
// it (ByteSize, Checksum). It mirrors ActiveStorage::Blob's stored columns. The
// underlying database row is managed through the Config's ModelStore seam.
type Blob struct {
	ID          int64
	Key         string
	Filename    string
	ContentType string
	Metadata    map[string]any
	ByteSize    int64
	Checksum    string
	ServiceName string
	CreatedAt   time.Time

	cfg *Config
}

// BlobParams describes a blob to build. Key and ContentType are derived when
// empty (a fresh base36 key; a MIME type inferred from Filename). ByteSize and
// Checksum are set automatically by the upload helpers, or supplied directly for
// a direct-upload blob.
type BlobParams struct {
	Filename    string
	ContentType string
	Metadata    map[string]any
	ServiceName string
	Key         string
	ByteSize    int64
	Checksum    string
}

// buildBlob constructs an in-memory Blob from params, filling in a generated key,
// the default service, an inferred content type, and metadata as needed. It does
// not touch the store.
func (c *Config) buildBlob(p BlobParams) (*Blob, error) {
	key := p.Key
	if key == "" {
		k, err := generateKey(c.random())
		if err != nil {
			return nil, err
		}
		key = k
	}
	service := p.ServiceName
	if service == "" {
		s, err := c.Services.Default()
		if err != nil {
			return nil, err
		}
		service = s.Name()
	}
	contentType := p.ContentType
	if contentType == "" {
		contentType = ContentTypeForFilename(p.Filename)
	}
	metadata := p.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	return &Blob{
		Key:         key,
		Filename:    p.Filename,
		ContentType: contentType,
		Metadata:    metadata,
		ByteSize:    p.ByteSize,
		Checksum:    p.Checksum,
		ServiceName: service,
		CreatedAt:   c.now(),
		cfg:         c,
	}, nil
}

// unfurl reads r fully, recording its size and base64 MD5 checksum into p, and
// returns the buffered bytes. It mirrors ActiveStorage::Blob#unfurl (which
// computes byte_size and checksum from the io).
func unfurl(r io.Reader, p *BlobParams) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	p.ByteSize = int64(len(data))
	p.Checksum = checksumOf(data)
	return data, nil
}

// checksumOf returns the base64-encoded MD5 digest of data, matching Rails'
// Digest::MD5.base64digest.
func checksumOf(data []byte) string {
	sum := md5.Sum(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// CreateAndUpload builds a blob from r and params, persists the row, then uploads
// the bytes to the blob's service — ActiveStorage::Blob.create_and_upload!.
func (c *Config) CreateAndUpload(r io.Reader, p BlobParams) (*Blob, error) {
	data, err := unfurl(r, &p)
	if err != nil {
		return nil, err
	}
	b, err := c.buildBlob(p)
	if err != nil {
		return nil, err
	}
	if err := c.Store.InsertBlob(b); err != nil {
		return nil, err
	}
	if err := b.uploadBytes(data); err != nil {
		return nil, err
	}
	return b, nil
}

// BuildAfterUpload builds a blob from r and params and uploads it, but does not
// persist the row — ActiveStorage::Blob.build_after_upload.
func (c *Config) BuildAfterUpload(r io.Reader, p BlobParams) (*Blob, error) {
	data, err := unfurl(r, &p)
	if err != nil {
		return nil, err
	}
	b, err := c.buildBlob(p)
	if err != nil {
		return nil, err
	}
	if err := b.uploadBytes(data); err != nil {
		return nil, err
	}
	return b, nil
}

// CreateBeforeDirectUpload builds and persists a blob from params (whose ByteSize
// and Checksum are already known) without uploading any bytes — the client will
// PUT them directly. Mirrors ActiveStorage::Blob.create_before_direct_upload!.
func (c *Config) CreateBeforeDirectUpload(p BlobParams) (*Blob, error) {
	b, err := c.buildBlob(p)
	if err != nil {
		return nil, err
	}
	if err := c.Store.InsertBlob(b); err != nil {
		return nil, err
	}
	return b, nil
}

// FindBlob loads a blob row by id.
func (c *Config) FindBlob(id int64) (*Blob, error) {
	return c.Store.FindBlob(id)
}

// FindSignedBlob verifies a signed id and loads the blob it names.
func (c *Config) FindSignedBlob(signedID string) (*Blob, error) {
	data, err := c.Signer.Verify(signedID, signedIDPurpose)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(data, 10, 64)
	if err != nil {
		return nil, err
	}
	return c.Store.FindBlob(id)
}

// Service resolves the blob's service through the registry.
func (b *Blob) Service() (Service, error) {
	return b.cfg.Services.Fetch(b.ServiceName)
}

func (b *Blob) uploadBytes(data []byte) error {
	svc, err := b.Service()
	if err != nil {
		return err
	}
	return svc.Upload(b.Key, bytes.NewReader(data), b.Checksum)
}

// Upload streams r to the blob's service, updating ByteSize and Checksum from the
// bytes read — ActiveStorage::Blob#upload.
func (b *Blob) Upload(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	b.ByteSize = int64(len(data))
	b.Checksum = checksumOf(data)
	return b.uploadBytes(data)
}

// Download returns the blob's full contents from its service.
func (b *Blob) Download() ([]byte, error) {
	svc, err := b.Service()
	if err != nil {
		return nil, err
	}
	rc, err := svc.Download(b.Key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// DownloadChunk returns length bytes of the blob starting at offset.
func (b *Blob) DownloadChunk(offset, length int64) ([]byte, error) {
	svc, err := b.Service()
	if err != nil {
		return nil, err
	}
	return svc.DownloadChunk(b.Key, offset, length)
}

// Open downloads the blob to a temporary file, verifies its checksum, and yields
// the open file positioned at the start. The temp file is always closed before it
// is removed, so the cleanup is safe on Windows (where an open file cannot be
// deleted). Mirrors ActiveStorage::Blob#open.
func (b *Blob) Open(fn func(*os.File) error) error {
	data, err := b.Download()
	if err != nil {
		return err
	}
	if checksumOf(data) != b.Checksum {
		return ErrIntegrity
	}
	pattern := "ActiveStorage-" + strconv.FormatInt(b.ID, 10) + "-*" + filepath.Ext(b.Filename)
	tmp, err := fsCreateTemp("", pattern)
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		fsRemove(name)
		return writeErr
	}
	if closeErr != nil {
		fsRemove(name)
		return closeErr
	}
	f, err := os.Open(name)
	if err != nil {
		fsRemove(name)
		return err
	}
	fnErr := fn(f)
	f.Close()      // close before remove: Windows-safe
	fsRemove(name) // best-effort cleanup
	return fnErr
}

// URL returns a service URL for the blob, defaulting the filename, content type,
// and disposition from the blob when the options leave them unset.
func (b *Blob) URL(opts URLOptions) (string, error) {
	svc, err := b.Service()
	if err != nil {
		return "", err
	}
	if opts.Filename.raw == "" {
		opts.Filename = NewFilename(b.Filename)
	}
	if opts.ContentType == "" {
		opts.ContentType = b.ContentType
	}
	if opts.Disposition == "" {
		opts.Disposition = "inline"
	}
	return svc.Url(b.Key, opts)
}

// SignedID returns a tamper-evident, purpose-scoped id for the blob, suitable for
// embedding in a URL — ActiveStorage::Blob#signed_id.
func (b *Blob) SignedID() (string, error) {
	return b.cfg.Signer.Sign(strconv.FormatInt(b.ID, 10), signedIDPurpose)
}

// Purge deletes the blob's stored object and its database row —
// ActiveStorage::Blob#purge. Variant/attachment cascade purging is deferred (see
// the roadmap).
func (b *Blob) Purge() error {
	svc, err := b.Service()
	if err != nil {
		return err
	}
	if err := svc.Delete(b.Key); err != nil {
		return err
	}
	return b.cfg.Store.DeleteBlob(b.ID)
}
