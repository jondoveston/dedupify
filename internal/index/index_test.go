package index

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetVisitedVsAll(t *testing.T) {
	seed := []File{
		{Path: "a", Hash: "aaa"},
		{Path: "b", Hash: "bbb"},
	}
	s := NewSet(seed)

	// Nothing visited yet: Visited is empty, All still has the seed.
	if got := s.Visited(); len(got) != 0 {
		t.Fatalf("Visited() before any Put = %v, want empty", got)
	}
	if got := s.All(); len(got) != 2 {
		t.Fatalf("All() before any Put = %v, want 2 entries", got)
	}

	// Visit "a" with an updated hash.
	s.Put(File{Path: "a", Hash: "updated"})

	visited := s.Visited()
	if len(visited) != 1 || visited[0].Hash != "updated" {
		t.Fatalf("Visited() = %+v, want [{a updated}]", visited)
	}

	all := s.All()
	if len(all) != 2 {
		t.Fatalf("All() = %+v, want 2 entries", all)
	}
	byPath := map[string]File{}
	for _, f := range all {
		byPath[f.Path] = f
	}
	if byPath["a"].Hash != "updated" {
		t.Errorf("All()[a].Hash = %q, want %q (visited should win)", byPath["a"].Hash, "updated")
	}
	if byPath["b"].Hash != "bbb" {
		t.Errorf("All()[b].Hash = %q, want %q (carried from seed)", byPath["b"].Hash, "bbb")
	}
}

func TestSetLookup(t *testing.T) {
	s := NewSet([]File{{Path: "a", Hash: "aaa"}})

	f, ok := s.Lookup("a")
	if !ok || f.Hash != "aaa" {
		t.Fatalf("Lookup(a) = %+v, %v, want {a aaa}, true", f, ok)
	}

	if _, ok := s.Lookup("missing"); ok {
		t.Fatal("Lookup(missing) = true, want false")
	}
}

func TestReadStdinEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()

	files, err := ReadStdin(f)
	if err != nil {
		t.Fatalf("ReadStdin() error = %v", err)
	}
	if files != nil {
		t.Fatalf("ReadStdin() = %v, want nil", files)
	}
}

func TestReadStdinJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.json")
	want := []File{{Path: "a", Size: 1, Hash: "x", ModTime: 1, HashTime: 2}}
	data, _ := json.Marshal(want)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = f.Close()
	}()

	got, err := ReadStdin(f)
	if err != nil {
		t.Fatalf("ReadStdin() error = %v", err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ReadStdin() = %+v, want %+v", got, want)
	}
}

func TestWriteEmptyIsBracket(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, nil); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "[]\n" {
		t.Fatalf("Write(nil) = %q, want %q", got, "[]\n")
	}
}

func TestWriteRoundTrip(t *testing.T) {
	want := []File{{Path: "a", Size: 1, Hash: "x", ModTime: 1, HashTime: 2}}
	var buf bytes.Buffer
	if err := Write(&buf, want); err != nil {
		t.Fatal(err)
	}

	var got []File
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}
