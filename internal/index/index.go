// Package index defines the file records that dedupify's hash and search
// commands exchange as JSON, and the set used to accumulate them.
package index

import (
	"encoding/json"
	"io"
	"os"
	"sort"
	"sync"
)

// File represents a single file's path, size, modification time and
// content hash, as captured by the hash command and consumed by the
// search command.
type File struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Hash     string `json:"hash"`
	ModTime  int64  `json:"mod_time"`
	HashTime int64  `json:"hash_time"`
}

// Set is a concurrency-safe collection of files used while hashing.
//
// It keeps two layers: seed, the entries carried in from a previous run
// (used only to decide whether a file can be skipped), and visited, the
// entries produced or confirmed during the current run. Keeping them
// separate lets a completed run report exactly what it found on disk
// (so deleted files naturally drop out), while an interrupted run can
// still fall back to everything that was ever known (seed ∪ visited) so
// that no previously captured work is lost.
type Set struct {
	mu      sync.Mutex
	seed    map[string]File
	visited map[string]File
}

// NewSet creates a Set seeded with previously captured files.
func NewSet(seed []File) *Set {
	s := &Set{
		seed:    make(map[string]File, len(seed)),
		visited: make(map[string]File, len(seed)),
	}
	for _, f := range seed {
		s.seed[f.Path] = f
	}
	return s
}

// Lookup returns the seeded entry for path, if any.
func (s *Set) Lookup(path string) (File, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.seed[path]
	return f, ok
}

// Put records a file as visited (hashed, or reused from the seed) during
// the current run.
func (s *Set) Put(f File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visited[f.Path] = f
}

// Visited returns the files visited during the current run, sorted by
// path. This is what a run that completed normally should emit: it
// reflects the current state of the filesystem, so files that have been
// deleted since the last run are correctly dropped.
func (s *Set) Visited() []File {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sortedValues(s.visited)
}

// All returns the union of the seed and the files visited so far, with
// visited entries taking precedence. This is what an interrupted run
// should emit, so that files not yet reached this run are not lost.
func (s *Set) All() []File {
	s.mu.Lock()
	defer s.mu.Unlock()
	merged := make(map[string]File, len(s.seed)+len(s.visited))
	for k, v := range s.seed {
		merged[k] = v
	}
	for k, v := range s.visited {
		merged[k] = v
	}
	return sortedValues(merged)
}

func sortedValues(m map[string]File) []File {
	out := make([]File, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ReadStdin reads a JSON array of File from stdin, if any was piped
// in. It returns a nil slice, with no error, when stdin is a terminal or
// empty.
func ReadStdin(stdin *os.File) ([]File, error) {
	stat, err := stdin.Stat()
	if err != nil {
		return nil, err
	}
	if stat.Mode()&os.ModeCharDevice != 0 {
		return nil, nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	var files []File
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// Write writes files as indented JSON to w, one trailing newline.
// A nil or empty slice is written as "[]" rather than "null".
func Write(w io.Writer, files []File) error {
	if files == nil {
		files = []File{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(files)
}
