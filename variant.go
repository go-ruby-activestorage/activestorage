package activestorage

// VariantRecord reserves the shape of ActiveStorage::VariantRecord — the row that
// tracks a materialized image variant of a blob (its variation digest and the
// variant's own blob).
//
// DEFERRED: image variants and transformations (mini_magick / ruby-vips),
// analyzers, and previewers are not implemented in v0.1. See the roadmap in the
// README. This type exists so downstream code and the eventual variant layer have
// a stable name to build on; it carries no behavior yet.
type VariantRecord struct {
	ID              int64
	BlobID          int64
	VariationDigest string
}
