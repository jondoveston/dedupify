// Package search finds duplicate files in a set of hashed file records.
package search

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jondoveston/dedupify/internal/index"
)

// Options configures a duplicate search.
type Options struct {
	// MinSize is the minimum file size, in bytes, to consider.
	MinSize int64
}

// Group is a set of files sharing the same content hash, along
// with which one has been selected to keep.
type Group struct {
	Keep   index.File
	Remove []index.File
}

// Find groups files by hash and returns one Group for
// every hash shared by more than one file above the minimum size,
// sorted by the path of the file to keep.
func Find(files []index.File, opts Options) []Group {
	byHash := make(map[string][]index.File)
	for _, f := range files {
		if f.Size <= opts.MinSize {
			continue
		}
		byHash[f.Hash] = append(byHash[f.Hash], f)
	}

	var groups []Group
	for _, fs := range byHash {
		if len(fs) < 2 {
			continue
		}
		keep := selectKeeper(fs)
		var remove []index.File
		for _, f := range fs {
			if f.Path != keep.Path {
				remove = append(remove, f)
			}
		}
		sort.Slice(remove, func(i, j int) bool { return remove[i].Path < remove[j].Path })
		groups = append(groups, Group{Keep: keep, Remove: remove})
	}

	sort.Slice(groups, func(i, j int) bool { return groups[i].Keep.Path < groups[j].Keep.Path })
	return groups
}

// selectKeeper picks which file in a duplicate group to keep: the oldest
// file whose path doesn't look like a copy, falling back to the oldest
// file overall if every candidate looks like a copy.
func selectKeeper(files []index.File) index.File {
	candidates := make([]index.File, 0, len(files))
	for _, f := range files {
		if strings.Contains(strings.ToLower(f.Path), " copy") {
			continue
		}
		candidates = append(candidates, f)
	}
	if len(candidates) == 0 {
		candidates = files
	}

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ModTime < candidates[j].ModTime })
	return candidates[0]
}

// Print writes a human-readable report (the file kept, followed
// by the files to remove, per group) to report, and the paths to remove,
// one per line, to remove -- ready to pipe into something like
// `xargs rm`.
func Print(groups []Group, report, remove io.Writer) {
	_, _ = fmt.Fprintf(report, "found %d duplicate group(s)\n", len(groups))
	for _, g := range groups {
		_, _ = fmt.Fprintf(report, "* %s\n", g.Keep.Path)
		for _, f := range g.Remove {
			_, _ = fmt.Fprintf(report, "%s\n", f.Path)
			_, _ = fmt.Fprintf(remove, "%s\n", f.Path)
		}
		_, _ = fmt.Fprintln(report)
	}
}
