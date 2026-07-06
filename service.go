package activestorage

import (
	"errors"
	"io"
	"time"
)

// Service-layer sentinel errors.
var (
	// ErrIntegrity is returned when an uploaded object's checksum does not match
	// the expected value (Rails' ActiveStorage::IntegrityError).
	ErrIntegrity = errors.New("activestorage: integrity check failed")
	// ErrServiceNotFound is returned by a Registry when a named service (or the
	// default) is not configured.
	ErrServiceNotFound = errors.New("activestorage: service not registered")
)

// URLOptions carries the keyword options Rails passes to Service#url — how long a
// generated URL stays valid, and the Content-Disposition/type the download should
// present.
type URLOptions struct {
	ExpiresIn   time.Duration
	Filename    Filename
	ContentType string
	Disposition string // "inline" or "attachment"
}

// Service is the storage-backend seam. DiskService is the built-in local
// implementation. The interface is deliberately key-addressed and streaming —
// upload/download take a key and an io stream, ranged reads go through
// DownloadChunk, and URLs are minted from a key — so cloud services (S3, GCS,
// Azure, a mirror service) can implement the same contract without leaking any
// filesystem assumptions.
type Service interface {
	// Name returns the service's registry name.
	Name() string
	// Upload stores r under key, verifying the base64 MD5 checksum when non-empty.
	Upload(key string, r io.Reader, checksum string) error
	// Download opens the object stored under key for reading.
	Download(key string) (io.ReadCloser, error)
	// DownloadChunk returns length bytes starting at offset.
	DownloadChunk(key string, offset, length int64) ([]byte, error)
	// Delete removes the object under key; missing keys are not an error.
	Delete(key string) error
	// Exist reports whether an object is stored under key.
	Exist(key string) (bool, error)
	// Url returns a URL from which the object can be retrieved.
	Url(key string, opts URLOptions) (string, error)
	// Size returns the stored byte size of the object under key.
	Size(key string) (int64, error)
}

// Registry holds the configured services and the default one
// (Rails config.active_storage.service). It is the resolver a Blob uses to turn
// its service_name into a Service.
type Registry struct {
	services map[string]Service
	def      string
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{services: map[string]Service{}}
}

// Register adds s under its Name(). If no default is set yet, the first
// registered service becomes the default. It returns the Registry for chaining.
func (r *Registry) Register(s Service) *Registry {
	r.services[s.Name()] = s
	if r.def == "" {
		r.def = s.Name()
	}
	return r
}

// SetDefault marks name as the default service.
func (r *Registry) SetDefault(name string) *Registry {
	r.def = name
	return r
}

// Fetch returns the service registered under name, or ErrServiceNotFound.
func (r *Registry) Fetch(name string) (Service, error) {
	s, ok := r.services[name]
	if !ok {
		return nil, ErrServiceNotFound
	}
	return s, nil
}

// Default returns the default service, or ErrServiceNotFound if none is set.
func (r *Registry) Default() (Service, error) {
	if r.def == "" {
		return nil, ErrServiceNotFound
	}
	return r.Fetch(r.def)
}
