# Dedupify

Dedupify is a stupidly simple two pass file deduplicator. File comparison is
done by content hash, using the xxHash algorithm
(https://github.com/cespare/xxhash).

It has two subcommands, `hash` and `search`, designed to be chained together
through JSON on stdin/stdout rather than through flags, config files, or a
database. That keeps the tool simple, scriptable, and -- importantly --
**interruptible and resumable**: hit Ctrl-C partway through hashing a large
tree and whatever has been discovered so far is written out as valid JSON.
Pipe that same JSON back in and dedupify picks up where it left off, skipping
files it already knows haven't changed.

## Install

Homebrew (macOS on Apple Silicon, Linux on x86-64 and arm64):

```sh
brew install jondoveston/tap/dedupify
```

Nix:

```sh
nix run github:jondoveston/dedupify -- hash .
nix profile install github:jondoveston/dedupify
```

From source (needs the Go version in `go.mod`):

```sh
go install github.com/jondoveston/dedupify@latest
```

## Usage

### `dedupify hash [path]`

Recursively walks `path` (the current directory by default), hashing every
file and writing the result as JSON to stdout. Progress and diagnostics go to
stderr, so stdout stays clean JSON suitable for piping or redirecting to a
file.

```
Flags:
  -m, --min string   minimum file size to consider, e.g. 1mb (default "0")
  -v, --verbose       log every file instead of periodic progress
```

Piping in JSON previously produced by `hash` lets it skip files that haven't
changed (same size, and a hash time at or after the file's modification
time), so re-running `hash` on a mostly-unchanged tree is fast:

```sh
$ dedupify hash --min 1mb . > files.json
scanned 500, hashed 480, reused 0, too small 20, failed 0, in 812ms

$ touch some/file
$ dedupify hash --min 1mb . < files.json > files.json
scanned 500, hashed 1, reused 479, too small 20, failed 0, in 34ms
```

**Interrupting and resuming.** Press Ctrl-C while a hash is running and
dedupify stops the walk immediately and writes out everything it knows --
both files it finished hashing this run, and anything carried over from
stdin -- so nothing already done is lost:

```sh
$ dedupify hash . > files.json
scanned 2345, hashed 2344, reused 0, too small 1, failed 0, in 81ms
^C
interrupted: writing everything known so far

$ dedupify hash . < files.json > files.json   # resume
scanned 20004, hashed 17658, reused 2344, too small 2, failed 0, in 1.2s
```

A run that completes normally (not interrupted) only ever emits files it
actually found on this walk, so files deleted since the last run correctly
drop out of the JSON.

### `dedupify search`

Reads the JSON produced by `hash` from stdin, groups files by hash, and for
each group of duplicates prints the file chosen to keep (the oldest file
whose name doesn't look like a "... copy" duplicate) plus the files to
remove, to stderr. The files to remove are also written to stdout, one path
per line, ready to pipe into something like `xargs rm`.

```
Flags:
  -m, --min string   minimum file size to consider, e.g. 1mb (default "0")
```

```sh
$ dedupify search --min 1mb < files.json > to-remove.txt
searching 2345 files...
found 3 duplicate group(s)
* /photos/holiday.jpg
/photos/holiday copy.jpg

$ xargs rm -v < to-remove.txt
```

## Development

Everything is driven by [Task](https://taskfile.dev). `nix develop` gives you
Go, task, golangci-lint and gopls at the versions CI uses. Without Nix,
`mise install` installs the versions pinned in `mise.toml`.

```sh
task            # build ./dedupify
task test       # go test ./... -race -cover
task lint       # golangci-lint run
task fmt        # gofmt -s -w .
task check      # everything CI checks: fmt:check, vet, test, lint
task --list
```

The Go version is whatever the stable nixpkgs channel in `flake.nix` provides
(currently nixos-26.05); keep the `go` line in `go.mod` at or below it.

### Layout

```
main.go             entry point; version info is injected here at build time
internal/cli        cobra commands and flag handling
internal/index      File records, the Set used while hashing, JSON read/write
internal/hasher     directory walk and content hashing
internal/search     duplicate grouping and the keep/remove report
```

### Nix builds

Nix builds everything, including the release binaries. Go cross-compiles with
`CGO_ENABLED=0`, so one machine can build every target:

```sh
task nix:build     # native build, with tests
task nix:cross     # linux x86-64/arm64 and darwin arm64
task nix:release   # tarballs + SHA256SUMS + the Homebrew formula in ./result
task nix:check     # nix flake check
```

Nix only sees files tracked by git, so `git add` new files before building.
After changing `go.mod`/`go.sum`, run `task nix:vendor-hash` to refresh
`vendorHash` in `flake.nix`.

### Releasing

1. Change `VERSION` and commit.
2. `task release:tag` creates and pushes `v<VERSION>`.
3. The `release` workflow builds `.#release` with Nix, publishes the tarballs
   and `SHA256SUMS` as a GitHub release, and pushes the generated formula to
   `jondoveston/homebrew-tap` (`Formula/dedupify.rb`).

The workflow needs a `HOMEBREW_TAP_GITHUB_TOKEN` secret: a fine-grained token
limited to the `homebrew-tap` repository with **Contents: Read and write**.

## Improvements

* Concurrent hashing -- the hashing loop is currently sequential by design,
  to keep interrupt handling simple and because it's safer on spinning disks
  and network shares. Worth revisiting with a worker pool for local NVMe.
* Better keep-file selection -- currently just "oldest file that doesn't
  look like a copy". Could take directory depth, file name similarity, or an
  explicit preference list into account.
