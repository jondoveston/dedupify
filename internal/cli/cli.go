// Package cli defines the dedupify command line.
package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/c2h5oh/datasize"
	"github.com/spf13/cobra"

	"github.com/jondoveston/dedupify/internal/hasher"
	"github.com/jondoveston/dedupify/internal/index"
	"github.com/jondoveston/dedupify/internal/search"
)

// BuildInfo describes the running binary. It is set by main from the
// values injected at build time with -ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd builds the dedupify command tree.
func NewRootCmd(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:           "dedupify",
		Short:         "Find duplicate files by content hash",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", info.Version, info.Commit, info.Date),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newHashCmd())
	root.AddCommand(newSearchCmd())
	return root
}

func newHashCmd() *cobra.Command {
	var minSize string
	var verbose bool

	cmd := &cobra.Command{
		Use:   "hash [path]",
		Short: "Walk a directory tree, hashing files, and emit JSON on stdout",
		Long: `Hash recursively walks the given path (the current directory by default),
hashing every file at least as large as --min. Previously captured JSON
piped in on stdin is used to skip files that have not changed, so an
interrupted run can be resumed by piping its own (partial) output back in.

Interrupting with Ctrl-C stops the walk and immediately writes out
everything known so far -- both files reached this run and anything
carried over from stdin -- so no work is lost.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			minBytes, err := parseSize(minSize)
			if err != nil {
				return err
			}

			seed, err := index.ReadStdin(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			set := index.NewSet(seed)
			opts := hasher.Options{MinSize: minBytes, Verbose: verbose}
			if err := hasher.Walk(ctx, path, set, opts, cmd.ErrOrStderr()); err != nil {
				return err
			}

			result := set.Visited()
			if ctx.Err() != nil {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "interrupted: writing everything known so far")
				result = set.All()
			}

			return index.Write(cmd.OutOrStdout(), result)
		},
	}

	cmd.Flags().StringVarP(&minSize, "min", "m", "0", "minimum file size to consider, e.g. 1mb")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "log every file instead of periodic progress")
	return cmd
}

func newSearchCmd() *cobra.Command {
	var minSize string

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Find duplicate files from JSON piped in on stdin",
		Long: `Search reads the JSON produced by "hash" from stdin, groups files by
hash, and for each group of duplicates prints the file chosen to keep,
plus the files to remove, to stderr. The files to remove are also written
to stdout, one path per line, ready to pipe into something like
"xargs rm".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			minBytes, err := parseSize(minSize)
			if err != nil {
				return err
			}

			files, err := index.ReadStdin(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			if files == nil {
				return errors.New("no JSON found on stdin; pipe in output from \"dedupify hash\"")
			}

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "searching %d files...\n", len(files))
			groups := search.Find(files, search.Options{MinSize: minBytes})
			search.Print(groups, cmd.ErrOrStderr(), cmd.OutOrStdout())
			return nil
		},
	}

	cmd.Flags().StringVarP(&minSize, "min", "m", "0", "minimum file size to consider, e.g. 1mb")
	return cmd
}

func parseSize(s string) (int64, error) {
	var v datasize.ByteSize
	if err := v.UnmarshalText([]byte(s)); err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	return int64(v.Bytes()), nil
}
