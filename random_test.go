package activestorage

import (
	"errors"
	"strings"
	"testing"
)

func TestBase36Deterministic(t *testing.T) {
	// Bytes 0..27, each < 36 after mod 64, so they map straight onto the alphabet.
	raw := make([]byte, minimumKeyLength)
	for i := range raw {
		raw[i] = byte(i)
	}
	src := &fakeRandom{bytesVal: raw}
	key, err := generateKey(src)
	if err != nil {
		t.Fatal(err)
	}
	want := base36Alphabet[:minimumKeyLength]
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}
	if len(key) != minimumKeyLength {
		t.Fatalf("len(key) = %d, want %d", len(key), minimumKeyLength)
	}
}

func TestBase36Resample(t *testing.T) {
	// A byte whose residue mod 64 is >= 36 must be re-drawn via RandomNumber.
	raw := make([]byte, minimumKeyLength)
	raw[0] = 40 // 40 % 64 = 40 >= 36 -> resample
	src := &fakeRandom{bytesVal: raw, numbers: []int{7}}
	key, err := generateKey(src)
	if err != nil {
		t.Fatal(err)
	}
	if key[0] != base36Alphabet[7] {
		t.Fatalf("key[0] = %q, want %q", key[0], base36Alphabet[7])
	}
}

func TestBase36RandomBytesError(t *testing.T) {
	src := &fakeRandom{bytesErr: errInjected}
	if _, err := generateKey(src); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestBase36RandomNumberError(t *testing.T) {
	raw := make([]byte, minimumKeyLength)
	raw[0] = 40 // force a resample so RandomNumber is called
	src := &fakeRandom{bytesVal: raw, numErr: errInjected}
	if _, err := generateKey(src); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v, want errInjected", err)
	}
}

func TestDefaultRandom(t *testing.T) {
	r := DefaultRandom()
	b, err := r.RandomBytes(16)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 16 {
		t.Fatalf("len = %d, want 16", len(b))
	}
	n, err := r.RandomNumber(36)
	if err != nil {
		t.Fatal(err)
	}
	if n < 0 || n >= 36 {
		t.Fatalf("n = %d out of range", n)
	}
	key, err := generateKey(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != minimumKeyLength {
		t.Fatalf("len(key) = %d, want %d", len(key), minimumKeyLength)
	}
	for _, c := range key {
		if !strings.ContainsRune(base36Alphabet, c) {
			t.Fatalf("key has non-base36 char %q", c)
		}
	}
}
