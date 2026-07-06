package activestorage

import (
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

// Filesystem operation seams. These indirect the handful of syscalls whose error
// paths cannot be provoked portably (a failing Create/Close, an injected Stat or
// Remove error), so the suite can exercise every branch. Production code uses the
// os defaults; tests swap them and restore via defer.
var (
	fsCreate = func(name string) (io.WriteCloser, error) { return os.Create(name) }
	fsStat   = os.Stat
	fsRemove = os.Remove
)

// DiskService is the built-in local-filesystem Service. It stores each object at
// Root/<key[0:2]>/<key[2:4]>/<key>, the same two-level sharding Rails'
// ActiveStorage::Service::DiskService uses to keep directories shallow.
type DiskService struct {
	name string
	root string
}

// NewDiskService returns a DiskService rooted at root and registered as name.
func NewDiskService(name, root string) *DiskService {
	return &DiskService{name: name, root: root}
}

// Name returns the service's registry name.
func (d *DiskService) Name() string { return d.name }

// Root returns the directory under which objects are stored.
func (d *DiskService) Root() string { return d.root }

// folderFor mirrors Rails' folder_for: the first two key characters, then the
// next two, joined as nested directories.
func folderFor(key string) string {
	return filepath.Join(key[0:2], key[2:4])
}

func (d *DiskService) pathFor(key string) string {
	return filepath.Join(d.root, folderFor(key), key)
}

// Upload writes r under key, creating the sharded directory as needed and, when
// checksum is non-empty, verifying the stored bytes' base64 MD5 against it
// (deleting the file and returning ErrIntegrity on mismatch).
func (d *DiskService) Upload(key string, r io.Reader, checksum string) error {
	path := d.pathFor(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := fsCreate(path)
	if err != nil {
		return err
	}
	h := md5.New()
	if _, err := io.Copy(f, io.TeeReader(r, h)); err != nil {
		f.Close()
		fsRemove(path)
		return err
	}
	if err := f.Close(); err != nil {
		fsRemove(path)
		return err
	}
	if checksum != "" {
		got := base64.StdEncoding.EncodeToString(h.Sum(nil))
		if got != checksum {
			fsRemove(path)
			return fmt.Errorf("%w: expected %q, computed %q", ErrIntegrity, checksum, got)
		}
	}
	return nil
}

// Download opens the object stored under key for reading. The caller closes it.
func (d *DiskService) Download(key string) (io.ReadCloser, error) {
	return os.Open(d.pathFor(key))
}

// DownloadChunk returns length bytes of the object under key starting at offset.
func (d *DiskService) DownloadChunk(key string, offset, length int64) ([]byte, error) {
	f, err := os.Open(d.pathFor(key))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, length)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

// Delete removes the object under key. A missing key is not an error (idempotent,
// like Rails rescuing Errno::ENOENT).
func (d *DiskService) Delete(key string) error {
	err := fsRemove(d.pathFor(key))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Exist reports whether an object is stored under key.
func (d *DiskService) Exist(key string) (bool, error) {
	_, err := fsStat(d.pathFor(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Size returns the stored byte size of the object under key.
func (d *DiskService) Size(key string) (int64, error) {
	info, err := fsStat(d.pathFor(key))
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Url returns a disk-service URL for key. Without the Rails routing/engine layer
// (deferred, see the roadmap) this is a representative, disposition-annotated
// path rather than a signed, routable URL.
func (d *DiskService) Url(key string, opts URLOptions) (string, error) {
	disposition := opts.Disposition
	if disposition == "" {
		disposition = "inline"
	}
	name := opts.Filename.String()
	if name == "" {
		name = key
	}
	v := url.Values{}
	v.Set("disposition", disposition+`; filename="`+name+`"`)
	if opts.ContentType != "" {
		v.Set("content_type", opts.ContentType)
	}
	return "/rails/active_storage/disk/" + key + "?" + v.Encode(), nil
}
