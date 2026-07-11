package activestorage

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"time"
)

// variationPurpose scopes a variation key, matching ActiveStorage::Variation's
// :variation purpose.
const variationPurpose = "variation"

// ErrNotTransformable is returned when a variant's bytes are requested on a
// Config whose Transformer seam is unset. Producing variant bytes needs a real
// image processor (go-images, libvips, ImageMagick); the descriptor records — a
// variant's key, digest, filename, and content type — never do.
var ErrNotTransformable = errors.New("activestorage: no image transformer configured")

// Transformer is the image-processing seam. Active Storage delegates the actual
// pixel work to ImageProcessing (libvips / ImageMagick); this library keeps that
// out of the CGO-free core and lets a host inject a transformer (e.g. one backed
// by go-images). A variation's descriptor — its key, digest, filename, and
// content type — is computed without ever calling this.
type Transformer interface {
	// Transform applies transformations to the image in src, encoding the result
	// as format (e.g. "png", "jpg"), and writes it to dst.
	Transform(dst io.Writer, src io.Reader, transformations Hash, format string) error
}

// Variation is a set of image transformations that, applied to a blob, yields a
// variant — ActiveStorage::Variation. The transformations are an ordered Hash
// (Active Storage deep-symbolizes them); order is preserved because it fixes the
// bytes of both the signed key and the Marshal-based digest.
type Variation struct {
	Transformations Hash
	verifier        Verifier
}

// NewVariation builds a Variation from transformations, signed by v (needed for
// Key; Digest does not use it).
func NewVariation(v Verifier, transformations Hash) *Variation {
	return &Variation{Transformations: transformations, verifier: v}
}

// Format returns the output format symbol, defaulting to "png" as Active Storage
// does when the transformations carry no :format.
func (v *Variation) Format() string {
	if raw, ok := v.Transformations.Get("format"); ok {
		switch f := raw.(type) {
		case Symbol:
			return string(f)
		case string:
			return f
		}
	}
	return "png"
}

// ContentType returns the MIME type of the variation's output format, matching
// ActiveStorage::Variation#content_type (Marcel::MimeType.for(extension: format)).
func (v *Variation) ContentType() string {
	return ContentTypeForFilename("x." + v.Format())
}

// Digest returns the variation's digest — the value stored in
// active_storage_variant_records.variation_digest to look a variant up. It is
// OpenSSL::Digest::SHA1.base64digest(Marshal.dump(transformations)), reproduced
// byte-for-byte via the internal Ruby-Marshal encoder.
func (v *Variation) Digest() (string, error) {
	dump, err := rubyMarshal(v.Transformations)
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(dump)
	return base64.StdEncoding.EncodeToString(sum[:]), nil
}

// Key returns the signed key that identifies the variation in a URL or in a
// variant's combined key — ActiveStorage::Variation#key, i.e.
// verifier.generate(transformations, purpose: :variation).
func (v *Variation) Key() (string, error) {
	if v.verifier == nil {
		return "", ErrInvalidSignature
	}
	return v.verifier.Generate(v.Transformations, variationPurpose, time.Time{})
}

// Variant is a specific transformation of an image blob —
// ActiveStorage::Variant. Its Key locates the transformed object on the service;
// producing the bytes needs a Transformer.
type Variant struct {
	blob      *Blob
	variation *Variation
}

// Variant returns the variant of b described by transformations. The blob's
// Config supplies the verifier (for the key) and the Transformer (for Process).
func (b *Blob) Variant(transformations Hash) *Variant {
	return &Variant{blob: b, variation: NewVariation(asVerifier(b.cfg.Signer), transformations)}
}

// asVerifier returns s as a Verifier when it implements one, else nil.
func asVerifier(s Signer) Verifier {
	if v, ok := s.(Verifier); ok {
		return v
	}
	return nil
}

// Variation returns the variant's variation descriptor.
func (v *Variant) Variation() *Variation { return v.variation }

// Key returns the variant's combined service key —
// "variants/<blob.key>/<SHA-256 hexdigest of the variation key>", matching
// ActiveStorage::Variant#key.
func (v *Variant) Key() (string, error) {
	vk, err := v.variation.Key()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(vk))
	return "variants/" + v.blob.Key + "/" + hex.EncodeToString(sum[:]), nil
}

// Filename returns the variant's filename — the blob's base name with the
// variation's (downcased) format as extension, per ActiveStorage::Variant#filename.
func (v *Variant) Filename() Filename {
	base := NewFilename(v.blob.Filename).Base()
	return NewFilename(base + "." + v.variation.Format())
}

// ContentType returns the variant's content type (its variation's output type).
func (v *Variant) ContentType() string { return v.variation.ContentType() }

// Processed reports whether the variant's transformed object already exists on
// the service (ActiveStorage::Variant#processed?).
func (v *Variant) Processed() (bool, error) {
	key, err := v.Key()
	if err != nil {
		return false, err
	}
	svc, err := v.blob.Service()
	if err != nil {
		return false, err
	}
	return svc.Exist(key)
}

// Process downloads the blob, runs the configured Transformer over it, and
// uploads the transformed bytes under the variant's key — the work
// ActiveStorage::Variant#process performs. It returns ErrNotTransformable when no
// Transformer is configured. The digest/key descriptor records never require this.
func (v *Variant) Process() error {
	if v.blob.cfg.Transformer == nil {
		return ErrNotTransformable
	}
	key, err := v.Key()
	if err != nil {
		return err
	}
	svc, err := v.blob.Service()
	if err != nil {
		return err
	}
	data, err := v.blob.Download()
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := v.blob.cfg.Transformer.Transform(&out, bytes.NewReader(data), v.variation.Transformations, v.variation.Format()); err != nil {
		return err
	}
	return svc.Upload(key, bytes.NewReader(out.Bytes()), "")
}

// VariantRecord mirrors ActiveStorage::VariantRecord — the row that tracks a
// materialized variant of a blob by its variation digest, pointing (through an
// attachment named "image") at the variant's own stored blob.
type VariantRecord struct {
	ID              int64
	BlobID          int64
	VariationDigest string
}

// NewVariantRecord builds the tracking record for variation on blobID, computing
// the variation digest the way Active Storage does.
func NewVariantRecord(blobID int64, variation *Variation) (*VariantRecord, error) {
	digest, err := variation.Digest()
	if err != nil {
		return nil, err
	}
	return &VariantRecord{BlobID: blobID, VariationDigest: digest}, nil
}
