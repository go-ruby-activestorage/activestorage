<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-activestorage/brand/main/social/go-ruby-activestorage-activestorage.png" alt="go-ruby-activestorage/activestorage" width="720"></p>

# activestorage — go-ruby-activestorage

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-activestorage.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Rails'
[Active Storage](https://guides.rubyonrails.org/active_storage_overview.html)** —
the framework for attaching files to records and storing them on a pluggable
backend. It reproduces Active Storage's observable behaviour **byte-for-byte** —
sharded storage keys, base64 MD5 checksums, `has_one_attached` /
`has_many_attached` semantics, message-verifier `signed_id`s, `Marcel`-style
content-type detection, variation digests / variant keys, and the direct-upload
token shape — **without any Ruby runtime**. The on-disk key layout, checksum
encoding, signed-id / variation-key format, and Marcel detection are pinned by a
**live differential test against the `activestorage` gem** (skipped when the gem
is absent).

It is the Active Storage backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby), but is a
**standalone, reusable** module — a sibling of
[go-ruby-set](https://github.com/go-ruby-set/set) and the other go-ruby-* stdlib
ports.

> **Persistence is a seam.** Active Storage's `Blob` and `Attachment` are
> ActiveRecord models. This library keeps the *model logic* and abstracts the
> database behind the [`ModelStore`](store.go) interface, so a host binds it to
> ActiveRecord (or anything else). A goroutine-safe in-memory `MemStore` ships as
> the reference implementation.

## Seams

Everything host-specific is injected through a small set of interfaces, gathered
in a `Config`:

| Seam | Interface | Rails analogue | Default provided |
|------|-----------|----------------|------------------|
| **Persistence** | `ModelStore` | ActiveRecord (`active_storage_blobs`, `active_storage_attachments`) | `MemStore` (in-memory) |
| **Storage** | `Service` | `ActiveStorage::Service` | `DiskService` (local, sharded) |
| **Signing** | `Signer` | `ActiveSupport::MessageVerifier` (`signed_id`) | `HMACSigner` (HMAC-SHA256) |
| **Randomness** | `RandomSource` | `SecureRandom.base36` (storage keys) | crypto/rand-backed |
| **Clock** | `func() time.Time` | `Time.current` | `time.Now().UTC` |

## Install

```sh
go get github.com/go-ruby-activestorage/activestorage
```

## Usage

```go
package main

import (
	"fmt"
	"strings"

	as "github.com/go-ruby-activestorage/activestorage"
)

func main() {
	cfg := &as.Config{
		Store:    as.NewMemStore(),
		Services: as.NewRegistry().Register(as.NewDiskService("local", "/var/storage")),
		Signer:   as.NewHMACSigner([]byte("secret-key-base")),
	}

	// Create a blob and stream its bytes to the service (create_and_upload!).
	blob, _ := cfg.CreateAndUpload(strings.NewReader("hello world"), as.BlobParams{
		Filename: "greeting.txt",
	})
	fmt.Println(blob.Key)        // 28-char base36 storage key
	fmt.Println(blob.Checksum)   // XrY7u+Ae7tCTyyK7j1rNww==  (Digest::MD5.base64digest)
	fmt.Println(blob.ContentType) // inferred from the filename

	// Attach it to a record: has_one_attached :avatar on User#1.
	user := as.RecordRef{Type: "User", ID: 1}
	cfg.One(user, "avatar").Attach(blob)

	// has_many_attached :photos — attach an existing blob and a fresh upload.
	cfg.Many(user, "photos").Attach(
		blob,
		as.Upload{Filename: "sunset.txt", Reader: strings.NewReader("...")},
	)

	// A tamper-evident id to embed in a URL, then resolve it back.
	sid, _ := blob.SignedID()
	again, _ := cfg.FindSignedBlob(sid)
	fmt.Println(again.ID == blob.ID) // true

	data, _ := blob.Download()
	fmt.Println(string(data)) // hello world
}
```

## What ships

- **`Service` abstraction** — a key-addressed, streaming interface
  (`Upload` / `Download` / `DownloadChunk` / `Delete` / `Exist` / `Url` / `Size`),
  shaped so S3/GCS/Azure services can implement the same contract, plus a
  `Registry` of configured services with a default.
- **`DiskService`** — stores each object at `root/<key[0:2]>/<key[2:4]>/<key>`
  (Rails' two-level sharding), verifies the base64 MD5 checksum on upload, and
  supports ranged reads. All temp/handle cleanup **closes files before removing
  them, so it is safe on Windows**.
- **`Blob`** — `create_and_upload!`, `build_after_upload`,
  `create_before_direct_upload!`, `upload`, `download`, `download_chunk`, `open`
  (checksum-verified, Windows-safe temp file), `purge`, `signed_id` / signed-id
  lookup, content-type inference, and 28-char base36 key generation with an
  injectable RNG.
- **`Attachment`** — `has_one_attached` (`OneAttached`) and `has_many_attached`
  (`ManyAttached`) semantics: `Attach` (of a `*Blob`, an `Upload{io,…}`, or a
  signed id), `Attached`, `Blob(s)`, `Detach`, `Purge`, and the join record.
- **`RailsVerifier`** — a `Signer`/`Verifier` that reproduces
  `ActiveSupport::MessageVerifier` **byte-for-byte** (JSON serializer, SHA-1 HMAC,
  url-safe payload, the `{"_rails":{"message","exp","pur"}}` metadata envelope), so
  a blob's `signed_id`, a variation key, and the disk service's direct-upload token
  are identical to a Rails app configured with the same secret. `HMACSigner`
  remains as a self-contained default.
- **Content-type detection** — `DetectContentType(data, name)`, a faithful subset
  of `Marcel::MimeType.for`: leading magic bytes first (PNG/JPEG/GIF/PDF/ZIP/…),
  the xml→svg / zip→docx "more specific" upgrades, then extension fallback, wired
  into the upload path (`identify:` honoured).
- **Variants / representations** — `Variation` (transformations descriptor,
  `Digest` = `SHA1.base64digest(Marshal.dump(transformations))` via an internal
  MRI-exact Ruby-Marshal encoder, `Key` = the signed variation key), `Variant`
  (combined `variants/<key>/<sha256>` service key, filename, content type,
  `Processed`/`Process`), and `VariantRecord`. Actual pixel work is delegated to an
  injectable **`Transformer`** seam (e.g. go-images) — **no libvips/CGO required**.
- **Direct uploads** — `CreateForDirectUpload` / `Blob.DirectUpload` return the
  `{signed_id, url, headers}` shape of `DirectUploadsController#create`; the
  `DiskService` mints the signed `:blob_token` URL (`DirectUploadService`).
- **`Filename`** — base / extension (Ruby `File.extname` semantics, incl. dotfiles)
  and `sanitized` matching the gem's exact `tr` character set.

## Roadmap (deferred)

The following Active Storage surface is intentionally **not** implemented yet and
is tracked for later releases:

- **Actual image transformation** — the `Variant`/`Variation` *descriptors* (key,
  digest, filename, content type) ship and match the gem; producing the transformed
  bytes needs an injected `Transformer` (this repo ships none, to stay CGO-free —
  a go-images-backed transformer is the intended companion).
- **Analyzers** (image/video/audio metadata) and **previewers** (PDF/video
  poster frames).
- **Cloud services** — S3, Google Cloud Storage, Azure, and the **mirror**
  service — layered on the existing `Service` interface.
- **The Rails engine** — routes, controllers, the disk/blob/representation
  redirect and proxy controllers, and signed *download* URLs backed by the Rails
  routing layer (the direct-upload token itself ships).
- **Streaming uploads** — an upload is buffered in memory to compute its checksum
  and size; a streaming/rewindable path is future work.

## Tests & coverage

The suite is deterministic and Ruby-free. Hard-to-provoke syscall error paths
(a failing `Create`/`Close`, an injected `Stat`/`Remove`, temp-file failures) are
reached through small filesystem seams, and the persistence/service/signer seams
are exercised with in-memory fakes and a temp-dir `DiskService`, holding line
coverage at **100%** including every error branch. The base64 MD5 checksum is
asserted against Rails' `Digest::MD5.base64digest` format.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

CGO-free, dependency-free, `gofmt` + `go vet` clean, and green across the six
64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le, s390x — including the
big-endian s390x) and three OSes (Linux, macOS, Windows).

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the
go-ruby-activestorage/activestorage authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
