<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-activestorage/brand/main/social/go-ruby-activestorage-activestorage.png" alt="go-ruby-activestorage/activestorage" width="720"></p>

# activestorage — go-ruby-activestorage

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-activestorage.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of Rails'
[Active Storage](https://guides.rubyonrails.org/active_storage_overview.html)** —
the framework for attaching files to records and storing them on a pluggable
backend. This is the **v0.1 foundation**: the `Blob` and `Attachment` model logic
and the `Service` storage abstraction (with a local `DiskService`), faithful to
Active Storage's observable behaviour — sharded storage keys, base64 MD5
checksums, `has_one_attached` / `has_many_attached` semantics, `create_and_upload!`
/ `create_before_direct_upload!` / `signed_id` — **without any Ruby runtime**.

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

## What v0.1 ships

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
- **`Filename`** (base / extension / sanitized), content-type helpers, and a
  `VariantRecord` shape stub.

## Roadmap (deferred)

The following Active Storage surface is intentionally **not** in v0.1 and is
tracked for later releases:

- **Image variants & transformations** (`variant`, `ImageProcessing`,
  mini_magick / ruby-vips) and the `VariantRecord` behaviour.
- **Analyzers** (image/video/audio metadata) and **previewers** (PDF/video
  poster frames).
- **Cloud services** — S3, Google Cloud Storage, Azure, and the **mirror**
  service — layered on the existing `Service` interface.
- **Direct uploads** — the direct-upload controller, `blob.service_url_for_direct_upload`,
  and signed disk URLs backed by the Rails routing/engine layer.
- **The Rails engine** — routes, controllers, and the representation redirect.
- **Streaming uploads** — v0.1 buffers an upload in memory to compute its
  checksum and size; a streaming/rewindable path is future work.

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
