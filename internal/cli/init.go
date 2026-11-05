package cli

import (
	"fmt"
	"io"

	"github.com/andresantos/docsmakercli/internal/app"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var force, shared bool

	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Wire up the git filter that keeps AI-generated doc lines out of commits",
		Long: "init configures this clone so that docs inserted with `doc --mode inline` are\n" +
			"stripped from everything git stages or commits, even inside a file that is\n" +
			"otherwise pushed. It is local to your clone (.git/info and .git/config) and\n" +
			"idempotent. Every developer who uses --mode inline must run it once per clone.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			result, err := app.Init(cmd.Context(), app.InitOptions{RepoPath: path, Force: force, Shared: shared})
			if err != nil {
				return err
			}
			renderInit(cmd.OutOrStdout(), result)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "replace a conflicting filter already configured for these files")
	cmd.Flags().BoolVar(&shared, "shared", false, "record the attributes in the tracked .gitattributes (shared with the team) instead of the local-only .git/info/attributes")

	return cmd
}

func renderInit(w io.Writer, r *app.InitResult) {
	status := func(changed bool) string {
		if changed {
			return "updated"
		}
		return "already set"
	}
	fmt.Fprintf(w, "repository: %s\n", r.RepoRoot)
	fmt.Fprintf(w, "  attributes %v in %s: %s\n", r.Patterns, r.AttributesFile, status(r.AttributesChanged))
	fmt.Fprintf(w, "  git filter %s (clean): %s\n", r.Executable, status(r.ConfigChanged))
	fmt.Fprintf(w, "  .docsmaker/ excluded from git: %s\n", status(r.ExcludeChanged))
	fmt.Fprintln(w, "\nAI-generated inline docs will now be stripped on commit. They live in your working tree only:")
	fmt.Fprintln(w, "a git operation that rewrites a file (e.g. a pull that changes it, or a stash) drops them; re-run `docsmaker doc --mode inline` to restore them (cached, so it is fast).")
}
