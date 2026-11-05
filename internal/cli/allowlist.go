package cli

import (
	"fmt"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/app"
	"github.com/spf13/cobra"
)

func newAllowlistCmd() *cobra.Command {
	var project string

	cmd := &cobra.Command{
		Use:   "allowlist",
		Short: "Manage values you have declared are not secrets (false positives)",
		Long: "When docsmaker flags something that is not a secret, answer [w] at the prompt\n" +
			"and it is never asked about again. Only a hash of the value is stored, in\n" +
			"<project>/.docsmaker/allowlist.json. That file is local to your machine on\n" +
			"purpose (a hash of a weak value can be guessed): do not commit it.",
	}
	cmd.PersistentFlags().StringVar(&project, "project", ".", "project directory")

	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "Show allowlisted entries and patterns",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				res, err := app.AllowlistShow(project)
				if err != nil {
					return err
				}
				w := cmd.OutOrStdout()
				fmt.Fprintf(w, "file: %s\n", res.Path)
				if len(res.Entries) == 0 && len(res.Patterns) == 0 {
					fmt.Fprintln(w, "(empty)")
				}
				for _, e := range res.Entries {
					fmt.Fprintf(w, "  %s  %-17s added %s  at %s\n", e.SHA256[:12], e.Kind, e.Added, e.Where)
				}
				for _, p := range res.Patterns {
					fmt.Fprintf(w, "  pattern %s\n", p)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "remove <id|pattern>",
			Short: "Remove an entry (first characters of its sha256, as shown by list) or a pattern",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				what, err := app.AllowlistRemove(project, args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", what)
				return nil
			},
		},
		&cobra.Command{
			Use:   "add-pattern <regex>",
			Short: "Allow every value that fully matches a regular expression",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := app.AllowlistAddPattern(project, args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "added pattern %s\n", args[0])
				return nil
			},
		},
	)
	return cmd
}
