// Package cli is a thin driver: it parses flags, calls internal/app, and
// renders the structured result to the console. It must not contain any
// analysis, LLM, secret-detection, or git-filter logic itself — that all
// lives behind internal/app so a future driver (e.g. `docsmaker serve`) can
// reuse it unchanged.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time: -ldflags "-X github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cli.Version=v1.2.3".
var Version = "dev"

// Execute runs the root command. Called from cmd/docsmaker/main.go.
func Execute() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		// Execute prints errors itself; without this cobra prints them twice
		// and dumps usage after runtime failures (e.g. a blocked secret).
		SilenceUsage:  true,
		SilenceErrors: true,
		Use:           "docsmaker",
		Version:       Version,
		Short:         "Generate code documentation locally, without sending code anywhere",
		Long: "docsmaker analyzes a codebase locally (Java first) and generates class/method\n" +
			"docs and a project README using a local LLM (Ollama). No source code ever\n" +
			"leaves the machine, and potentially sensitive content is flagged before it\n" +
			"can appear in any output.",
	}

	root.AddCommand(
		newAnalyzeCmd(),
		newDocCmd(),
		newInitCmd(),
		newModelCmd(),
		newAllowlistCmd(),
		newGitCleanFilterCmd(),
	)

	return root
}
