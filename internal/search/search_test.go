package search

import (
	"bytes"
	"testing"

	"github.com/jondoveston/dedupify/internal/index"
)

func TestFindGroupsByHash(t *testing.T) {
	files := []index.File{
		{Path: "/a", Hash: "same", Size: 100, ModTime: 1},
		{Path: "/b", Hash: "same", Size: 100, ModTime: 2},
		{Path: "/c", Hash: "different", Size: 100, ModTime: 1},
		{Path: "/tiny", Hash: "same", Size: 1, ModTime: 1},
	}

	groups := Find(files, Options{MinSize: 10})
	if len(groups) != 1 {
		t.Fatalf("Find() = %+v, want 1 group", groups)
	}

	g := groups[0]
	if g.Keep.Path != "/a" {
		t.Errorf("Keep.Path = %q, want %q (oldest ModTime)", g.Keep.Path, "/a")
	}
	if len(g.Remove) != 1 || g.Remove[0].Path != "/b" {
		t.Errorf("Remove = %+v, want [{Path: /b}]", g.Remove)
	}
}

func TestSelectKeeperPrefersNonCopyAndOldest(t *testing.T) {
	files := []index.File{
		{Path: "/dir/report copy.pdf", ModTime: 1},
		{Path: "/dir/report.pdf", ModTime: 2},
	}
	keep := selectKeeper(files)
	if keep.Path != "/dir/report.pdf" {
		t.Errorf("selectKeeper() = %q, want %q", keep.Path, "/dir/report.pdf")
	}
}

func TestSelectKeeperFallsBackWhenAllLookLikeCopies(t *testing.T) {
	files := []index.File{
		{Path: "/dir/report copy.pdf", ModTime: 2},
		{Path: "/dir/report copy 2.pdf", ModTime: 1},
	}
	keep := selectKeeper(files)
	if keep.Path != "/dir/report copy 2.pdf" {
		t.Errorf("selectKeeper() = %q, want the oldest of the copies, %q", keep.Path, "/dir/report copy 2.pdf")
	}
}

func TestPrint(t *testing.T) {
	groups := []Group{
		{
			Keep:   index.File{Path: "/keep"},
			Remove: []index.File{{Path: "/dup1"}, {Path: "/dup2"}},
		},
	}

	var report, remove bytes.Buffer
	Print(groups, &report, &remove)

	wantRemove := "/dup1\n/dup2\n"
	if got := remove.String(); got != wantRemove {
		t.Errorf("remove output = %q, want %q", got, wantRemove)
	}
	if !bytes.Contains(report.Bytes(), []byte("/keep")) {
		t.Errorf("report output = %q, want it to mention the kept file", report.String())
	}
}
