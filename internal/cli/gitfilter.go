package cli

import (
	"fmt"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/gitfilter"
	"github.com/spf13/cobra"
)

// newGitCleanFilterCmd wires the stdin -> stdout contract git expects from a
// clean/smudge filter driver. This is invoked by git itself (configured via
// `docsmaker init`), never directly by a developer — hidden from --help.
func newGitCleanFilterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "git-clean-filter <clean|smudge>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "clean":
				return gitfilter.Clean(cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			case "smudge":
				return gitfilter.Smudge(cmd.InOrStdin(), cmd.OutOrStdout())
			default:
				return fmt.Errorf("git-clean-filter: unknown mode %q, expected clean|smudge", args[0])
			}
		},
	}
	return cmd
}
