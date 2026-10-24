// Command dedupify finds duplicate files by content hash.
//
// It has two subcommands, "hash" and "search", designed to be chained
// through JSON on stdin/stdout: hash walks a directory tree and emits a
// JSON description of every file and its hash, and search reads that
// JSON back in to report duplicates. Because the hash command both reads
// and writes this JSON, an interrupted hash run can be resumed by piping
// its own (possibly partial) output back in.
package main

import (
	"fmt"
	"os"

	"github.com/jondoveston/dedupify/internal/cli"
)

// Set at build time with -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	info := cli.BuildInfo{Version: version, Commit: commit, Date: date}
	if err := cli.NewRootCmd(info).Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "dedupify:", err)
		os.Exit(1)
	}
}
