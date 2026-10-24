package hasher

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"

	"github.com/jondoveston/dedupify/internal/index"
)

func xxhashHex(t *testing.T, data []byte) string {
	t.Helper()
	h := xxhash.New()
	if _, err := h.Write(data); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

func TestWalkHashesFilesAboveMinSize(t *testing.T) {
	dir := t.TempDir()
	big := []byte("this file is definitely more than ten bytes long")
	small := []byte("tiny")

	mustWriteFile(t, filepath.Join(dir, "big.txt"), big)
	mustWriteFile(t, filepath.Join(dir, "small.txt"), small)

	set := index.NewSet(nil)
	var stderr bytes.Buffer
	err := Walk(context.Background(), dir, set, Options{MinSize: 10}, &stderr)
	if err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	files := set.Visited()
	if len(files) != 1 {
		t.Fatalf("Visited() = %+v, want 1 file (small.txt excluded by MinSize)", files)
	}
	if got, want := files[0].Hash, xxhashHex(t, big); got != want {
		t.Errorf("hash = %q, want %q", got, want)
	}
	if files[0].Size != int64(len(big)) {
		t.Errorf("size = %d, want %d", files[0].Size, len(big))
	}
}

func TestWalkReusesUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	mustWriteFile(t, path, []byte("hello world"))

	set := index.NewSet(nil)
	var stderr bytes.Buffer
	if err := Walk(context.Background(), dir, set, Options{}, &stderr); err != nil {
		t.Fatal(err)
	}
	first := set.Visited()
	if len(first) != 1 {
		t.Fatalf("first run Visited() = %+v, want 1 file", first)
	}

	// Resume: seed a fresh Set with the first run's output and hash again
	// without touching the file.
	resumed := index.NewSet(first)
	if err := Walk(context.Background(), dir, resumed, Options{Verbose: true}, &stderr); err != nil {
		t.Fatal(err)
	}
	second := resumed.Visited()
	if len(second) != 1 {
		t.Fatalf("second run Visited() = %+v, want 1 file", second)
	}

	if second[0].Hash != first[0].Hash {
		t.Errorf("hash changed across an unmodified rerun: %q -> %q", first[0].Hash, second[0].Hash)
	}
	if second[0].HashTime != first[0].HashTime {
		t.Errorf("HashTime changed, want it preserved from reuse: %d -> %d", first[0].HashTime, second[0].HashTime)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("already hashed")) {
		t.Errorf("stderr = %q, want it to mention the file was reused", stderr.String())
	}
}

func TestWalkRehashesModifiedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	mustWriteFile(t, path, []byte("hello world"))

	set := index.NewSet(nil)
	var stderr bytes.Buffer
	if err := Walk(context.Background(), dir, set, Options{}, &stderr); err != nil {
		t.Fatal(err)
	}
	first := set.Visited()

	// Simulate the file having been hashed in the past, then modified.
	stale := first[0]
	stale.HashTime = time.Now().Add(-time.Hour).Unix()
	mustWriteFile(t, path, []byte("hello world, modified"))

	resumed := index.NewSet([]index.File{stale})
	if err := Walk(context.Background(), dir, resumed, Options{}, &stderr); err != nil {
		t.Fatal(err)
	}
	second := resumed.Visited()
	if len(second) != 1 {
		t.Fatalf("Visited() = %+v, want 1 file", second)
	}
	if second[0].Hash == stale.Hash {
		t.Error("hash did not change after modifying the file's contents")
	}
}

func TestWalkStopsOnCancelledContext(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("aaaaaaaaaa"))
	mustWriteFile(t, filepath.Join(dir, "b.txt"), []byte("bbbbbbbbbb"))

	seed := []index.File{{Path: "carried-over", Hash: "zzz"}}
	set := index.NewSet(seed)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the walk even starts

	var stderr bytes.Buffer
	if err := Walk(ctx, dir, set, Options{}, &stderr); err != nil {
		t.Fatalf("Walk() error = %v", err)
	}

	if visited := set.Visited(); len(visited) != 0 {
		t.Errorf("Visited() = %+v, want empty: cancelled before any file was reached", visited)
	}
	all := set.All()
	if len(all) != 1 || all[0].Path != "carried-over" {
		t.Errorf("All() = %+v, want only the carried-over seed entry preserved", all)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
