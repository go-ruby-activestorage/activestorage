package activestorage

import (
	"errors"
	"sync"
)

// ErrBlobNotFound and ErrAttachmentNotFound are the sentinel errors a ModelStore
// returns when a lookup or mutation targets a row that does not exist.
var (
	ErrBlobNotFound       = errors.New("activestorage: blob not found")
	ErrAttachmentNotFound = errors.New("activestorage: attachment not found")
)

// ModelStore is the persistence seam. In Rails, Blob and Attachment are
// ActiveRecord models backed by the active_storage_blobs and
// active_storage_attachments tables; this interface abstracts that away so the
// model logic here has no database dependency. A host binds it to ActiveRecord
// (or any datastore); NewMemStore provides an in-memory reference implementation.
//
// Insert* assigns the row's ID field. Find/Update/Delete operate by ID.
type ModelStore interface {
	InsertBlob(b *Blob) error
	FindBlob(id int64) (*Blob, error)
	UpdateBlob(b *Blob) error
	DeleteBlob(id int64) error

	InsertAttachment(a *Attachment) error
	FindAttachments(recordType string, recordID int64, name string) ([]*Attachment, error)
	DeleteAttachment(id int64) error
}

// MemStore is a goroutine-safe, in-memory ModelStore. It is the reference
// implementation used in tests and suitable for the rbgo binding's scratch use;
// it is not durable.
type MemStore struct {
	mu          sync.Mutex
	blobs       map[int64]*Blob
	attachments map[int64]*Attachment
	nextBlobID  int64
	nextAttID   int64
}

// NewMemStore returns an empty in-memory ModelStore.
func NewMemStore() *MemStore {
	return &MemStore{
		blobs:       map[int64]*Blob{},
		attachments: map[int64]*Attachment{},
	}
}

// InsertBlob assigns b.ID and stores the blob.
func (m *MemStore) InsertBlob(b *Blob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextBlobID++
	b.ID = m.nextBlobID
	m.blobs[b.ID] = b
	return nil
}

// FindBlob returns the blob with the given id, or ErrBlobNotFound.
func (m *MemStore) FindBlob(id int64) (*Blob, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blobs[id]
	if !ok {
		return nil, ErrBlobNotFound
	}
	return b, nil
}

// UpdateBlob replaces the stored blob with the given id, or returns
// ErrBlobNotFound.
func (m *MemStore) UpdateBlob(b *Blob) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.blobs[b.ID]; !ok {
		return ErrBlobNotFound
	}
	m.blobs[b.ID] = b
	return nil
}

// DeleteBlob removes the blob with the given id, or returns ErrBlobNotFound.
func (m *MemStore) DeleteBlob(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.blobs[id]; !ok {
		return ErrBlobNotFound
	}
	delete(m.blobs, id)
	return nil
}

// InsertAttachment assigns a.ID and stores the attachment.
func (m *MemStore) InsertAttachment(a *Attachment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextAttID++
	a.ID = m.nextAttID
	m.attachments[a.ID] = a
	return nil
}

// FindAttachments returns the attachments for a record and name, in insertion
// order.
func (m *MemStore) FindAttachments(recordType string, recordID int64, name string) ([]*Attachment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Attachment
	// Iterate in ID order so results are deterministic.
	for id := int64(1); id <= m.nextAttID; id++ {
		a, ok := m.attachments[id]
		if !ok {
			continue
		}
		if a.RecordType == recordType && a.RecordID == recordID && a.Name == name {
			out = append(out, a)
		}
	}
	return out, nil
}

// DeleteAttachment removes the attachment with the given id, or returns
// ErrAttachmentNotFound.
func (m *MemStore) DeleteAttachment(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.attachments[id]; !ok {
		return ErrAttachmentNotFound
	}
	delete(m.attachments, id)
	return nil
}
