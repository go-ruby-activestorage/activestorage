package activestorage

import (
	"errors"
	"strings"
	"testing"
)

func mkBlob(t *testing.T, cfg *Config, content string) *Blob {
	t.Helper()
	b, err := cfg.CreateAndUpload(strings.NewReader(content), BlobParams{Filename: "f.txt"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBlobForVariants(t *testing.T) {
	cfg, _ := newTestConfig(t)
	existing := mkBlob(t, cfg, "x")

	// *Blob passes through.
	if b, err := cfg.blobFor(existing); err != nil || b != existing {
		t.Fatalf("blobFor(*Blob) = %v, %v", b, err)
	}
	// Upload creates and uploads a new blob.
	up, err := cfg.blobFor(Upload{Filename: "u.txt", ContentType: "text/plain", Reader: strings.NewReader("hi")})
	if err != nil || up.ID == 0 {
		t.Fatalf("blobFor(Upload) = %v, %v", up, err)
	}
	// string is a signed id.
	sid, _ := existing.SignedID()
	if b, err := cfg.blobFor(sid); err != nil || b.ID != existing.ID {
		t.Fatalf("blobFor(signed id) = %v, %v", b, err)
	}
	// Unsupported type.
	if _, err := cfg.blobFor(42); !errors.Is(err, ErrUnattachable) {
		t.Fatalf("blobFor(int) = %v, want ErrUnattachable", err)
	}
}

func TestOneAttached(t *testing.T) {
	cfg, disk := newTestConfig(t)
	rec := RecordRef{Type: "User", ID: 1}
	one := cfg.One(rec, "avatar")

	// Initially unattached.
	if ok, err := one.Attached(); ok || err != nil {
		t.Fatalf("Attached = %v, %v", ok, err)
	}
	if b, err := one.Blob(); b != nil || err != nil {
		t.Fatalf("Blob = %v, %v", b, err)
	}
	if err := one.Detach(); err != nil {
		t.Fatalf("Detach empty = %v", err)
	}
	if err := one.Purge(); err != nil {
		t.Fatalf("Purge empty = %v", err)
	}

	// Attach.
	first := mkBlob(t, cfg, "one")
	att, err := one.Attach(first)
	if err != nil {
		t.Fatal(err)
	}
	if att.BlobID != first.ID {
		t.Fatalf("att.BlobID = %d", att.BlobID)
	}
	if ok, _ := one.Attached(); !ok {
		t.Fatal("should be attached")
	}
	gotBlob, err := one.Blob()
	if err != nil || gotBlob.ID != first.ID {
		t.Fatalf("Blob = %v, %v", gotBlob, err)
	}

	// Re-attach replaces the previous join record.
	second := mkBlob(t, cfg, "two")
	if _, err := one.Attach(second); err != nil {
		t.Fatal(err)
	}
	atts, _ := cfg.Store.FindAttachments("User", 1, "avatar")
	if len(atts) != 1 || atts[0].BlobID != second.ID {
		t.Fatalf("after re-attach: %+v", atts)
	}

	// Purge removes the attachment and its blob.
	if err := one.Purge(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := disk.Exist(second.Key); ok {
		t.Fatal("blob object not purged")
	}
	if ok, _ := one.Attached(); ok {
		t.Fatal("still attached after purge")
	}
}

func TestOneAttachedDetach(t *testing.T) {
	cfg, disk := newTestConfig(t)
	one := cfg.One(RecordRef{Type: "User", ID: 2}, "avatar")
	blob := mkBlob(t, cfg, "keepme")
	if _, err := one.Attach(blob); err != nil {
		t.Fatal(err)
	}
	if err := one.Detach(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := one.Attached(); ok {
		t.Fatal("still attached")
	}
	// Detach leaves the blob object in place.
	if ok, _ := disk.Exist(blob.Key); !ok {
		t.Fatal("detach must not delete the blob")
	}
}

func TestOneAttachedErrors(t *testing.T) {
	// blobFor error.
	cfg, _ := newTestConfig(t)
	one := cfg.One(RecordRef{Type: "U", ID: 1}, "a")
	if _, err := one.Attach(42); !errors.Is(err, ErrUnattachable) {
		t.Fatalf("Attach blobFor err = %v", err)
	}

	// Attachment() lookup error during Attach.
	fs := newFailStore()
	cfg2 := configWith(fs, NewDiskService("test", t.TempDir()))
	blob := mkBlob(t, cfg2, "x")
	fs.findAttErr = errInjected
	one2 := cfg2.One(RecordRef{Type: "U", ID: 1}, "a")
	if _, err := one2.Attach(blob); !errors.Is(err, errInjected) {
		t.Fatalf("Attach find err = %v", err)
	}
	// The same lookup error surfaces through the read helpers.
	if _, err := one2.Attached(); !errors.Is(err, errInjected) {
		t.Fatalf("Attached err = %v", err)
	}
	if _, err := one2.Blob(); !errors.Is(err, errInjected) {
		t.Fatalf("Blob err = %v", err)
	}
	if err := one2.Detach(); !errors.Is(err, errInjected) {
		t.Fatalf("Detach err = %v", err)
	}
	if err := one2.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge err = %v", err)
	}

	// InsertAttachment error.
	fs3 := newFailStore()
	cfg3 := configWith(fs3, NewDiskService("test", t.TempDir()))
	blob3 := mkBlob(t, cfg3, "x")
	fs3.insertAttErr = errInjected
	if _, err := cfg3.One(RecordRef{Type: "U", ID: 1}, "a").Attach(blob3); !errors.Is(err, errInjected) {
		t.Fatalf("Attach insert err = %v", err)
	}

	// Delete-existing error on re-attach.
	fs4 := newFailStore()
	cfg4 := configWith(fs4, NewDiskService("test", t.TempDir()))
	b4 := mkBlob(t, cfg4, "x")
	one4 := cfg4.One(RecordRef{Type: "U", ID: 1}, "a")
	if _, err := one4.Attach(b4); err != nil {
		t.Fatal(err)
	}
	fs4.deleteAttErr = errInjected
	if _, err := one4.Attach(mkBlob(t, cfg4, "y")); !errors.Is(err, errInjected) {
		t.Fatalf("re-attach delete err = %v", err)
	}
}

func TestAttachmentBlobAndPurge(t *testing.T) {
	// Uncached Blob load.
	fs := newFailStore()
	cfg := configWith(fs, NewDiskService("test", t.TempDir()))
	blob := mkBlob(t, cfg, "content")
	a := &Attachment{BlobID: blob.ID, cfg: cfg}
	got, err := a.Blob()
	if err != nil || got.ID != blob.ID {
		t.Fatalf("Blob = %v, %v", got, err)
	}
	// Second call returns the cached blob.
	if got2, _ := a.Blob(); got2 != got {
		t.Fatal("expected cached blob")
	}

	// FindBlob error (uncached).
	fs.findBlobErr = errInjected
	a2 := &Attachment{BlobID: 999, cfg: cfg}
	if _, err := a2.Blob(); !errors.Is(err, errInjected) {
		t.Fatalf("Blob err = %v", err)
	}
	fs.findBlobErr = nil

	// Attachment.Purge: DeleteAttachment error.
	one := cfg.One(RecordRef{Type: "R", ID: 1}, "a")
	att, err := one.Attach(mkBlob(t, cfg, "z"))
	if err != nil {
		t.Fatal(err)
	}
	fs.deleteAttErr = errInjected
	if err := att.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge delete-att err = %v", err)
	}
	fs.deleteAttErr = nil

	// Attachment.Purge: Blob() error.
	fs.findBlobErr = errInjected
	att3 := &Attachment{BlobID: 5, cfg: cfg}
	if err := att3.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge blob err = %v", err)
	}
	fs.findBlobErr = nil

	// Attachment.Purge: blob.Purge() (store delete) error.
	att4, err := cfg.One(RecordRef{Type: "R", ID: 2}, "a").Attach(mkBlob(t, cfg, "w"))
	if err != nil {
		t.Fatal(err)
	}
	fs.deleteBlobErr = errInjected
	if err := att4.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge blob-purge err = %v", err)
	}
}

func TestManyAttached(t *testing.T) {
	cfg, disk := newTestConfig(t)
	rec := RecordRef{Type: "Post", ID: 1}
	many := cfg.Many(rec, "images")

	if ok, err := many.Attached(); ok || err != nil {
		t.Fatalf("Attached empty = %v, %v", ok, err)
	}

	a1 := mkBlob(t, cfg, "img1")
	atts, err := many.Attach(a1, Upload{Filename: "img2.txt", Reader: strings.NewReader("img2")})
	if err != nil {
		t.Fatal(err)
	}
	if len(atts) != 2 {
		t.Fatalf("attached %d, want 2", len(atts))
	}
	if ok, _ := many.Attached(); !ok {
		t.Fatal("should be attached")
	}
	blobs, err := many.Blobs()
	if err != nil || len(blobs) != 2 {
		t.Fatalf("Blobs = %v, %v", blobs, err)
	}

	// Purge removes all records and their blobs.
	keys := []string{blobs[0].Key, blobs[1].Key}
	if err := many.Purge(); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if ok, _ := disk.Exist(k); ok {
			t.Fatalf("blob %s not purged", k)
		}
	}
	if ok, _ := many.Attached(); ok {
		t.Fatal("still attached after purge")
	}
}

func TestManyAttachedDetach(t *testing.T) {
	cfg, disk := newTestConfig(t)
	many := cfg.Many(RecordRef{Type: "Post", ID: 2}, "images")
	b := mkBlob(t, cfg, "keep")
	if _, err := many.Attach(b); err != nil {
		t.Fatal(err)
	}
	if err := many.Detach(); err != nil {
		t.Fatal(err)
	}
	if ok, _ := many.Attached(); ok {
		t.Fatal("still attached")
	}
	if ok, _ := disk.Exist(b.Key); !ok {
		t.Fatal("detach must not delete blobs")
	}
}

func TestManyAttachedErrors(t *testing.T) {
	// blobFor error mid-batch: the first attachment is returned, then the error.
	cfg, _ := newTestConfig(t)
	many := cfg.Many(RecordRef{Type: "P", ID: 1}, "imgs")
	good := mkBlob(t, cfg, "ok")
	out, err := many.Attach(good, 42)
	if !errors.Is(err, ErrUnattachable) || len(out) != 1 {
		t.Fatalf("out = %d, err = %v", len(out), err)
	}

	// InsertAttachment error.
	fs := newFailStore()
	cfg2 := configWith(fs, NewDiskService("test", t.TempDir()))
	b2 := mkBlob(t, cfg2, "x")
	fs.insertAttErr = errInjected
	if _, err := cfg2.Many(RecordRef{Type: "P", ID: 1}, "i").Attach(b2); !errors.Is(err, errInjected) {
		t.Fatalf("Attach insert err = %v", err)
	}

	// FindAttachments error surfaces everywhere.
	fs3 := newFailStore()
	cfg3 := configWith(fs3, NewDiskService("test", t.TempDir()))
	fs3.findAttErr = errInjected
	m3 := cfg3.Many(RecordRef{Type: "P", ID: 1}, "i")
	if _, err := m3.Attached(); !errors.Is(err, errInjected) {
		t.Fatalf("Attached err = %v", err)
	}
	if _, err := m3.Blobs(); !errors.Is(err, errInjected) {
		t.Fatalf("Blobs err = %v", err)
	}
	if err := m3.Detach(); !errors.Is(err, errInjected) {
		t.Fatalf("Detach err = %v", err)
	}
	if err := m3.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge err = %v", err)
	}

	// Blobs: per-attachment Blob() error.
	fs4 := newFailStore()
	cfg4 := configWith(fs4, NewDiskService("test", t.TempDir()))
	b4 := mkBlob(t, cfg4, "x")
	m4 := cfg4.Many(RecordRef{Type: "P", ID: 3}, "i")
	if _, err := m4.Attach(b4); err != nil {
		t.Fatal(err)
	}
	// Force the attachment to reload its blob from the store, then fail that load.
	atts, _ := m4.Attachments()
	atts[0].blob = nil
	fs4.findBlobErr = errInjected
	if _, err := m4.Blobs(); !errors.Is(err, errInjected) {
		t.Fatalf("Blobs per-item err = %v", err)
	}

	// Detach: DeleteAttachment error.
	fs5 := newFailStore()
	cfg5 := configWith(fs5, NewDiskService("test", t.TempDir()))
	b5 := mkBlob(t, cfg5, "x")
	m5 := cfg5.Many(RecordRef{Type: "P", ID: 4}, "i")
	if _, err := m5.Attach(b5); err != nil {
		t.Fatal(err)
	}
	fs5.deleteAttErr = errInjected
	if err := m5.Detach(); !errors.Is(err, errInjected) {
		t.Fatalf("Detach delete err = %v", err)
	}

	// Purge: per-attachment Purge error.
	fs6 := newFailStore()
	cfg6 := configWith(fs6, NewDiskService("test", t.TempDir()))
	b6 := mkBlob(t, cfg6, "x")
	m6 := cfg6.Many(RecordRef{Type: "P", ID: 5}, "i")
	if _, err := m6.Attach(b6); err != nil {
		t.Fatal(err)
	}
	fs6.deleteBlobErr = errInjected
	if err := m6.Purge(); !errors.Is(err, errInjected) {
		t.Fatalf("Purge item err = %v", err)
	}
}
