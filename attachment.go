package activestorage

import (
	"errors"
	"io"
	"time"
)

// ErrUnattachable is returned when a value passed to Attach is not something the
// library knows how to turn into a Blob.
var ErrUnattachable = errors.New("activestorage: value is not attachable")

// RecordRef identifies the owning record of an attachment by its polymorphic
// type and id — the record_type / record_id columns of
// active_storage_attachments.
type RecordRef struct {
	Type string
	ID   int64
}

// Upload is an attachable wrapping an io stream plus its filename and content
// type — the Rails { io:, filename:, content_type: } attachable form. Attaching
// one creates and uploads a new blob.
type Upload struct {
	Filename    string
	ContentType string
	Reader      io.Reader
}

// Attachment is the join record binding a blob to a named association on a record
// — ActiveStorage::Attachment (record, name, blob).
type Attachment struct {
	ID         int64
	RecordType string
	RecordID   int64
	Name       string
	BlobID     int64
	CreatedAt  time.Time

	blob *Blob
	cfg  *Config
}

// Blob loads the attached blob, caching it on the attachment.
func (a *Attachment) Blob() (*Blob, error) {
	if a.blob != nil {
		return a.blob, nil
	}
	b, err := a.cfg.Store.FindBlob(a.BlobID)
	if err != nil {
		return nil, err
	}
	a.blob = b
	return b, nil
}

// Purge deletes the attachment record and purges its blob —
// ActiveStorage::Attachment#purge.
func (a *Attachment) Purge() error {
	b, err := a.Blob()
	if err != nil {
		return err
	}
	if err := a.cfg.Store.DeleteAttachment(a.ID); err != nil {
		return err
	}
	return b.Purge()
}

// blobFor resolves an attachable into a Blob: an existing *Blob is used as-is, an
// Upload is created-and-uploaded, and a string is treated as a signed id.
func (c *Config) blobFor(attachable any) (*Blob, error) {
	switch v := attachable.(type) {
	case *Blob:
		return v, nil
	case Upload:
		return c.CreateAndUpload(v.Reader, BlobParams{Filename: v.Filename, ContentType: v.ContentType})
	case string:
		return c.FindSignedBlob(v)
	default:
		return nil, ErrUnattachable
	}
}

func (c *Config) buildAttachment(record RecordRef, name string, blob *Blob) *Attachment {
	return &Attachment{
		RecordType: record.Type,
		RecordID:   record.ID,
		Name:       name,
		BlobID:     blob.ID,
		CreatedAt:  c.now(),
		blob:       blob,
		cfg:        c,
	}
}

// OneAttached models a has_one_attached association: a single, named attachment
// on a record.
type OneAttached struct {
	cfg    *Config
	record RecordRef
	name   string
}

// One returns the has_one_attached proxy for name on record.
func (c *Config) One(record RecordRef, name string) *OneAttached {
	return &OneAttached{cfg: c, record: record, name: name}
}

// Attachment returns the current attachment, or nil when none is attached. If
// several rows exist (from prior attaches), the most recent wins.
func (o *OneAttached) Attachment() (*Attachment, error) {
	atts, err := o.cfg.Store.FindAttachments(o.record.Type, o.record.ID, o.name)
	if err != nil {
		return nil, err
	}
	if len(atts) == 0 {
		return nil, nil
	}
	return atts[len(atts)-1], nil
}

// Attached reports whether an attachment is present.
func (o *OneAttached) Attached() (bool, error) {
	a, err := o.Attachment()
	if err != nil {
		return false, err
	}
	return a != nil, nil
}

// Blob returns the attached blob, or nil when none is attached.
func (o *OneAttached) Blob() (*Blob, error) {
	a, err := o.Attachment()
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, nil
	}
	return a.Blob()
}

// Attach binds attachable to the record, replacing any existing attachment (the
// previous join record is removed). It returns the new attachment.
func (o *OneAttached) Attach(attachable any) (*Attachment, error) {
	blob, err := o.cfg.blobFor(attachable)
	if err != nil {
		return nil, err
	}
	existing, err := o.Attachment()
	if err != nil {
		return nil, err
	}
	att := o.cfg.buildAttachment(o.record, o.name, blob)
	if err := o.cfg.Store.InsertAttachment(att); err != nil {
		return nil, err
	}
	if existing != nil {
		if err := o.cfg.Store.DeleteAttachment(existing.ID); err != nil {
			return nil, err
		}
	}
	return att, nil
}

// Detach removes the attachment record, leaving its blob in place —
// ActiveStorage's detach.
func (o *OneAttached) Detach() error {
	a, err := o.Attachment()
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	return o.cfg.Store.DeleteAttachment(a.ID)
}

// Purge removes the attachment record and purges its blob.
func (o *OneAttached) Purge() error {
	a, err := o.Attachment()
	if err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	return a.Purge()
}

// ManyAttached models a has_many_attached association: an ordered set of named
// attachments on a record.
type ManyAttached struct {
	cfg    *Config
	record RecordRef
	name   string
}

// Many returns the has_many_attached proxy for name on record.
func (c *Config) Many(record RecordRef, name string) *ManyAttached {
	return &ManyAttached{cfg: c, record: record, name: name}
}

// Attachments returns the current attachments in insertion order.
func (m *ManyAttached) Attachments() ([]*Attachment, error) {
	return m.cfg.Store.FindAttachments(m.record.Type, m.record.ID, m.name)
}

// Attached reports whether at least one attachment is present.
func (m *ManyAttached) Attached() (bool, error) {
	atts, err := m.Attachments()
	if err != nil {
		return false, err
	}
	return len(atts) > 0, nil
}

// Blobs returns the attached blobs in order.
func (m *ManyAttached) Blobs() ([]*Blob, error) {
	atts, err := m.Attachments()
	if err != nil {
		return nil, err
	}
	blobs := make([]*Blob, 0, len(atts))
	for _, a := range atts {
		b, err := a.Blob()
		if err != nil {
			return nil, err
		}
		blobs = append(blobs, b)
	}
	return blobs, nil
}

// Attach appends each attachable to the record, returning the new attachments. On
// the first failure it returns the attachments created so far and the error.
func (m *ManyAttached) Attach(attachables ...any) ([]*Attachment, error) {
	out := make([]*Attachment, 0, len(attachables))
	for _, attachable := range attachables {
		blob, err := m.cfg.blobFor(attachable)
		if err != nil {
			return out, err
		}
		att := m.cfg.buildAttachment(m.record, m.name, blob)
		if err := m.cfg.Store.InsertAttachment(att); err != nil {
			return out, err
		}
		out = append(out, att)
	}
	return out, nil
}

// Detach removes all attachment records, leaving their blobs in place.
func (m *ManyAttached) Detach() error {
	atts, err := m.Attachments()
	if err != nil {
		return err
	}
	for _, a := range atts {
		if err := m.cfg.Store.DeleteAttachment(a.ID); err != nil {
			return err
		}
	}
	return nil
}

// Purge removes all attachment records and purges their blobs.
func (m *ManyAttached) Purge() error {
	atts, err := m.Attachments()
	if err != nil {
		return err
	}
	for _, a := range atts {
		if err := a.Purge(); err != nil {
			return err
		}
	}
	return nil
}
