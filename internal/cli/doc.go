package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/app"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
	"github.com/spf13/cobra"
)

func newDocCmd() *cobra.Command {
	var level, target, mode, secretsPolicy, format, model string
	var yes bool

	cmd := &cobra.Command{
		Use:   "doc [path]",
		Short: "Generate documentation for a project, class, or method",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			opts := app.GenerateDocsOptions{
				ProjectPath:    path,
				Level:          app.DocLevel(level),
				Target:         target,
				Mode:           app.OutputMode(mode),
				Secrets:        app.SecretsPolicy(secretsPolicy),
				NonInteractive: yes,
				Model:          model,
				Logf:           func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) },
				// Prompts go to stderr so stdout stays clean for results / --format json.
				Approver: secrets.NewTerminalApprover(cmd.InOrStdin(), cmd.ErrOrStderr()),
			}

			if opts.Secrets == app.SecretsOff {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"WARNING: --secrets off disables sensitive-information screening. "+
						"Source text goes to the model unredacted; use only on trusted or synthetic code.")
			}

			result, err := app.GenerateDocs(cmd.Context(), opts)
			if err != nil {
				return err
			}

			return renderDoc(cmd, result, format)
		},
	}

	cmd.Flags().StringVar(&level, "level", "project", "detail level: method|class|package|project")
	cmd.Flags().StringVar(&target, "target", "", "fully-qualified name, required for --level method|class|package (e.g. com.foo.Bar#baz)")
	cmd.Flags().StringVar(&mode, "mode", "console", "output mode: console | inline (write into sources; requires `docsmaker init`) | sidecar (.docsmaker/doc)")
	cmd.Flags().StringVar(&secretsPolicy, "secrets", "prompt",
		"sensitive-information policy: prompt (ask: redact/skip/allow once/abort), strict (block any finding), off (no screening)")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: never prompt; any sensitive finding blocks the run")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	cmd.Flags().StringVar(&model, "model", "", "Ollama model to use (defaults to the tool's built-in default)")

	return cmd
}

func renderDoc(cmd *cobra.Command, result *app.GenerateDocsResult, format string) error {
	w := cmd.OutOrStdout()

	switch format {
	case "json":
		return renderDocJSON(w, result)
	case "text", "":
		return renderDocText(w, result)
	default:
		return fmt.Errorf("doc: unknown --format %q, expected text|json", format)
	}
}

func renderDocJSON(w io.Writer, result *app.GenerateDocsResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func renderDocText(w io.Writer, result *app.GenerateDocsResult) error {
	for i, a := range result.Artifacts {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "=== %s: %s ===\n", a.Level, a.Target)
		fmt.Fprintln(w, a.Content)
	}
	if result.Inline != nil {
		renderInline(w, result.Inline)
	}
	return nil
}

func renderInline(w io.Writer, r *app.InlineReport) {
	counts := map[string]int{}
	for _, o := range r.Outcomes {
		counts[o.Action]++
	}
	fmt.Fprintf(w, "\n=== inline: %d inserted, %d replaced, %d unchanged, %d skipped in %d file(s) ===\n",
		counts["inserted"], counts["replaced"], counts["unchanged"], counts["skipped"], r.FilesChanged)
	for _, o := range r.Outcomes {
		if o.Action == "skipped" {
			fmt.Fprintf(w, "  skipped %s (%s:%d): %s\n", o.Target, o.File, o.Line, o.Reason)
		}
	}
}
