package activestorage

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func railsConfig(t *testing.T) (*Config, *DiskService) {
	t.Helper()
	disk := NewDiskService("test", t.TempDir())
	disk.Verifier = NewRailsVerifier(oracleSecret())
	cfg := &Config{
		Store:    NewMemStore(),
		Services: NewRegistry().Register(disk),
		Signer:   NewRailsVerifier(oracleSecret()),
	}
	return cfg, disk
}

func TestVariationDigestGolden(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	cases := []struct {
		in     Hash
		digest string
	}{
		{Hash{{Key: "resize_to_limit", Value: []any{100, 100}}}, "MrRQt6g5ESnwLN2rWK172mwnte0="},
		{Hash{{Key: "resize_to_limit", Value: []any{800, 800}}, {Key: "colourspace", Value: "b-w"}, {Key: "rotate", Value: "-90"}}, "O0J4tjGv8NTepSg8SPrSh7TGUyQ="},
		{Hash{{Key: "resize", Value: "100x100"}, {Key: "format", Value: Symbol("jpg")}}, "L8po9ijOyQqUboPtLKNwvtYTSmU="},
		{Hash{{Key: "crop", Value: true}}, "1CYYc9ucBzVkLZ1o8/inX+CcN0k="},
	}
	for _, c := range cases {
		va := NewVariation(v, c.in)
		got, err := va.Digest()
		if err != nil || got != c.digest {
			t.Fatalf("digest(%v) = %q, %v; want %q", c.in, got, err, c.digest)
		}
	}
}

func TestVariationKeyGolden(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	va := NewVariation(v, Hash{{Key: "resize_to_limit", Value: []any{100, 100}}})
	key, err := va.Key()
	if err != nil {
		t.Fatal(err)
	}
	if key != "eyJfcmFpbHMiOnsibWVzc2FnZSI6ImV5SnlaWE5wZW1WZmRHOWZiR2x0YVhRaU9sc3hNREFzTVRBd1hYMD0iLCJleHAiOm51bGwsInB1ciI6InZhcmlhdGlvbiJ9fQ--7008411a478ef8c26e89ca84ed90ce294ddb7d4d" {
		t.Fatalf("variation key = %s", key)
	}
}

func TestVariationFormatAndContentType(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	// default png
	if f := NewVariation(v, Hash{}).Format(); f != "png" {
		t.Fatalf("default format = %q", f)
	}
	if ct := NewVariation(v, Hash{}).ContentType(); ct != "image/png" {
		t.Fatalf("default content type = %q", ct)
	}
	// symbol format
	if f := NewVariation(v, Hash{{Key: "format", Value: Symbol("jpg")}}).Format(); f != "jpg" {
		t.Fatalf("symbol format = %q", f)
	}
	// string format
	if f := NewVariation(v, Hash{{Key: "format", Value: "gif"}}).Format(); f != "gif" {
		t.Fatalf("string format = %q", f)
	}
	// non-string/symbol format falls back to default
	if f := NewVariation(v, Hash{{Key: "format", Value: 7}}).Format(); f != "png" {
		t.Fatalf("odd format = %q", f)
	}
}

func TestVariationErrors(t *testing.T) {
	// Digest of an unmarshalable value.
	if _, err := NewVariation(nil, Hash{{Key: "x", Value: notEncodable{}}}).Digest(); err == nil {
		t.Fatal("expected digest error")
	}
	// Key with no verifier.
	if _, err := NewVariation(nil, Hash{}).Key(); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("nil-verifier key err = %v", err)
	}
}

func TestVariantKeyGolden(t *testing.T) {
	cfg, _ := railsConfig(t)
	b, err := cfg.CreateAndUpload(strings.NewReader("img"), BlobParams{Filename: "a.png"})
	if err != nil {
		t.Fatal(err)
	}
	b.Key = "abcdefghijklmnopqrstuvwxyz01" // fixed so the variant key is deterministic
	variant := b.Variant(Hash{{Key: "resize_to_limit", Value: []any{100, 100}}})
	key, err := variant.Key()
	if err != nil {
		t.Fatal(err)
	}
	want := "variants/abcdefghijklmnopqrstuvwxyz01/ddc564eb41d60cbc3e51b14c1c0165ab613154092565d17cd3ec1b1a6eac3e48"
	if key != want {
		t.Fatalf("variant key = %s", key)
	}
	// Filename and content type.
	if fn := variant.Filename().String(); fn != "a.png" {
		t.Fatalf("variant filename = %s", fn)
	}
	if variant.ContentType() != "image/png" {
		t.Fatalf("variant content type = %s", variant.ContentType())
	}
}

func TestVariantKeyErrorNoVerifier(t *testing.T) {
	cfg, _ := newTestConfig(t) // HMACSigner, not a Verifier
	b, _ := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	if _, err := b.Variant(Hash{}).Key(); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v", err)
	}
}

// stubTransformer records its inputs and emits fixed bytes.
type stubTransformer struct {
	err    error
	format string
}

func (s *stubTransformer) Transform(dst io.Writer, src io.Reader, _ Hash, format string) error {
	if s.err != nil {
		return s.err
	}
	s.format = format
	_, _ = io.Copy(io.Discard, src)
	_, err := dst.Write([]byte("transformed"))
	return err
}

func TestVariantProcess(t *testing.T) {
	cfg, disk := railsConfig(t)
	tr := &stubTransformer{}
	cfg.Transformer = tr
	b, _ := cfg.CreateAndUpload(strings.NewReader("original"), BlobParams{Filename: "a.jpg"})
	variant := b.Variant(Hash{{Key: "resize_to_limit", Value: []any{10, 10}}, {Key: "format", Value: Symbol("png")}})

	if ok, _ := variant.Processed(); ok {
		t.Fatal("variant should not exist before processing")
	}
	if err := variant.Process(); err != nil {
		t.Fatal(err)
	}
	if tr.format != "png" {
		t.Fatalf("transformer format = %q", tr.format)
	}
	if ok, _ := variant.Processed(); !ok {
		t.Fatal("variant should exist after processing")
	}
	// The stored bytes are the transformer's output.
	key, _ := variant.Key()
	got, err := disk.Download(key)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(got)
	got.Close()
	if !bytes.Equal(data, []byte("transformed")) {
		t.Fatalf("stored = %q", data)
	}
}

func TestVariantProcessErrors(t *testing.T) {
	// No transformer configured.
	cfg, _ := railsConfig(t)
	b, _ := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	if err := b.Variant(Hash{}).Process(); !errors.Is(err, ErrNotTransformable) {
		t.Fatalf("no-transformer err = %v", err)
	}

	// Transformer returns an error.
	cfg2, _ := railsConfig(t)
	cfg2.Transformer = &stubTransformer{err: errInjected}
	b2, _ := cfg2.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	if err := b2.Variant(Hash{}).Process(); !errors.Is(err, errInjected) {
		t.Fatalf("transform err = %v", err)
	}

	// Key error (verifier missing) propagates.
	cfg3, _ := newTestConfig(t)
	cfg3.Transformer = &stubTransformer{}
	b3, _ := cfg3.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	if _, err := b3.Variant(Hash{}).Processed(); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("processed key err = %v", err)
	}
	if err := b3.Variant(Hash{}).Process(); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("process key err = %v", err)
	}
}

func TestVariantServiceAndDownloadErrors(t *testing.T) {
	// Download error: point the blob at a non-existent key with a real transformer.
	cfg, _ := railsConfig(t)
	cfg.Transformer = &stubTransformer{}
	b, _ := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	b.Key = "zz00000000000000000000000000" // nothing stored here
	if err := b.Variant(Hash{}).Process(); err == nil {
		t.Fatal("expected download error")
	}

	// Service resolution error in Processed/Process.
	cfg2, _ := railsConfig(t)
	cfg2.Transformer = &stubTransformer{}
	b2, _ := cfg2.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	b2.ServiceName = "missing"
	if _, err := b2.Variant(Hash{}).Processed(); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("processed svc err = %v", err)
	}
	if err := b2.Variant(Hash{}).Process(); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("process svc err = %v", err)
	}
}

func TestNewVariantRecord(t *testing.T) {
	v := NewRailsVerifier(oracleSecret())
	rec, err := NewVariantRecord(5, NewVariation(v, Hash{{Key: "crop", Value: true}}))
	if err != nil {
		t.Fatal(err)
	}
	if rec.BlobID != 5 || rec.VariationDigest != "1CYYc9ucBzVkLZ1o8/inX+CcN0k=" {
		t.Fatalf("record = %+v", rec)
	}
	// Digest error propagates.
	if _, err := NewVariantRecord(1, NewVariation(v, Hash{{Key: "x", Value: notEncodable{}}})); err == nil {
		t.Fatal("expected digest error")
	}
}

func TestVariationAccessor(t *testing.T) {
	cfg, _ := railsConfig(t)
	b, _ := cfg.CreateAndUpload(strings.NewReader("x"), BlobParams{Filename: "a.png"})
	variant := b.Variant(Hash{{Key: "crop", Value: true}})
	if variant.Variation() == nil {
		t.Fatal("Variation() returned nil")
	}
}
