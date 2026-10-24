// Package hasher walks a directory tree and records a content hash for each
// file, reusing hashes from a previous run where files are unchanged.
package hasher

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/cespare/xxhash/v2"

	"github.com/jondoveston/dedupify/internal/index"
)

// Options configures a hash walk.
type Options struct {
	// MinSize is the minimum file size, in bytes, to consider. Files at
	// or below this size are skipped entirely.
	MinSize int64
	// Verbose logs every file visited; otherwise only a periodic
	// progress summary and a final summary are logged.
	Verbose bool
}

// progressInterval is how many files are scanned between progress lines
// when not running verbosely.
const progressInterval = 500

// Walk recursively walks root, hashing every file larger than
// opts.MinSize and recording it in set. A file already present in set's
// seed with a matching size and a hash time at or after its current
// modification time is assumed unchanged and is not re-read.
//
// If ctx is cancelled the walk stops early without error; whatever has
// been recorded in set so far is preserved. Callers can check ctx.Err()
// after Walk returns to tell a completed walk from an interrupted one.
func Walk(ctx context.Context, root string, set *index.Set, opts Options, stderr io.Writer) error {
	var scanned, hashed, reused, tooSmall, failed int
	start := time.Now()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(stderr, "skip %s: %v\n", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			failed++
			_, _ = fmt.Fprintf(stderr, "skip %s: %v\n", path, err)
			return nil
		}

		scanned++
		if info.Size() <= opts.MinSize {
			tooSmall++
			if opts.Verbose {
				_, _ = fmt.Fprintf(stderr, "too small: %s\n", path)
			}
			return nil
		}

		wasReused, err := hashFile(path, info, set, opts.Verbose, stderr)
		switch {
		case err != nil:
			failed++
			_, _ = fmt.Fprintf(stderr, "skip %s: %v\n", path, err)
		case wasReused:
			reused++
		default:
			hashed++
		}

		if !opts.Verbose && scanned%progressInterval == 0 {
			_, _ = fmt.Fprintf(stderr, "scanned %d, hashed %d, reused %d, too small %d, failed %d\n",
				scanned, hashed, reused, tooSmall, failed)
		}

		return nil
	})

	_, _ = fmt.Fprintf(stderr, "scanned %d, hashed %d, reused %d, too small %d, failed %d, in %s\n",
		scanned, hashed, reused, tooSmall, failed, time.Since(start).Round(time.Millisecond))

	return err
}

// hashFile hashes a single file, reusing a previously computed hash from
// set's seed when the file's size and modification time indicate it has
// not changed since it was last hashed. It reports whether the hash was
// reused rather than recomputed.
func hashFile(path string, info fs.FileInfo, set *index.Set, verbose bool, stderr io.Writer) (reused bool, err error) {
	if prev, ok := set.Lookup(path); ok {
		if prev.Size == info.Size() && prev.HashTime >= info.ModTime().Unix() {
			set.Put(index.File{
				Path:     path,
				Size:     info.Size(),
				Hash:     prev.Hash,
				ModTime:  info.ModTime().Unix(),
				HashTime: prev.HashTime,
			})
			if verbose {
				_, _ = fmt.Fprintf(stderr, "already hashed: %s\n", path)
			}
			return true, nil
		}
	}

	sum, err := hashFileContents(path)
	if err != nil {
		return false, err
	}

	set.Put(index.File{
		Path:     path,
		Size:     info.Size(),
		Hash:     sum,
		ModTime:  info.ModTime().Unix(),
		HashTime: time.Now().Unix(),
	})
	if verbose {
		_, _ = fmt.Fprintf(stderr, "hashed: %s\n", path)
	}
	return false, nil
}

// hashFileContents returns the xxhash of a file's contents as a fixed
// width hex string.
func hashFileContents(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	h := xxhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%016x", h.Sum64()), nil
}
