package activestorage

import (
	"errors"
	"testing"
)

func TestMemStoreBlobLifecycle(t *testing.T) {
	m := NewMemStore()
	b := &Blob{Key: "k"}
	if err := m.InsertBlob(b); err != nil {
		t.Fatal(err)
	}
	if b.ID != 1 {
		t.Fatalf("ID = %d, want 1", b.ID)
	}
	got, err := m.FindBlob(1)
	if err != nil || got != b {
		t.Fatalf("FindBlob = %v, %v", got, err)
	}
	b.Filename = "updated"
	if err := m.UpdateBlob(b); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteBlob(1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FindBlob(1); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("FindBlob after delete = %v, want ErrBlobNotFound", err)
	}
}

func TestMemStoreBlobNotFound(t *testing.T) {
	m := NewMemStore()
	if _, err := m.FindBlob(99); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("FindBlob = %v", err)
	}
	if err := m.UpdateBlob(&Blob{ID: 99}); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("UpdateBlob = %v", err)
	}
	if err := m.DeleteBlob(99); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("DeleteBlob = %v", err)
	}
}

func TestMemStoreAttachmentLifecycle(t *testing.T) {
	m := NewMemStore()
	a := &Attachment{RecordType: "User", RecordID: 1, Name: "avatar", BlobID: 5}
	other := &Attachment{RecordType: "User", RecordID: 2, Name: "avatar", BlobID: 6}
	if err := m.InsertAttachment(a); err != nil {
		t.Fatal(err)
	}
	if err := m.InsertAttachment(other); err != nil {
		t.Fatal(err)
	}
	if a.ID != 1 {
		t.Fatalf("ID = %d, want 1", a.ID)
	}
	found, err := m.FindAttachments("User", 1, "avatar")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != a {
		t.Fatalf("FindAttachments = %v", found)
	}
	if err := m.DeleteAttachment(1); err != nil {
		t.Fatal(err)
	}
	// After deleting id 1, the range loop must skip the missing id.
	found, err = m.FindAttachments("User", 2, "avatar")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != other {
		t.Fatalf("FindAttachments = %v", found)
	}
}

func TestMemStoreAttachmentNotFound(t *testing.T) {
	m := NewMemStore()
	if err := m.DeleteAttachment(99); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("DeleteAttachment = %v", err)
	}
}
