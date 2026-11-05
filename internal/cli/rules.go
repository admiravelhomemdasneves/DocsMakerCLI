package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/app"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/rules"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/secrets"
	"github.com/spf13/cobra"
)

func newRulesCmd() *cobra.Command {
	var target, mode, secretsPolicy, format, model, minConfidence, outputLang string
	var minScore, maxMethods int
	var yes bool

	cmd := &cobra.Command{
		Use:   "rules [path]",
		Short: "Explain, in plain language, the business rules the code implements",
		Long: "rules reads the methods that look like they hold business logic (decisions,\n" +
			"thresholds, validations) and asks the local model to state the rule each one\n" +
			"implements, with the code lines it rests on and how far to trust it.\n\n" +
			"The model only sees the code, so the \"why\" of a rule is a hypothesis, always\n" +
			"marked as such. Nothing is written into your sources, and the same secret\n" +
			"screening as `doc` applies.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			if format != "text" && format != "json" && format != "" {
				return fmt.Errorf("rules: unknown --format %q, expected text|json", format)
			}

			opts := app.ExtractRulesOptions{
				ProjectPath:    path,
				Target:         target,
				Mode:           app.OutputMode(mode),
				MinScore:       minScore,
				MaxMethods:     maxMethods,
				MinConfidence:  rules.Confidence(minConfidence),
				OutputLang:     outputLang,
				Secrets:        app.SecretsPolicy(secretsPolicy),
				NonInteractive: yes,
				Model:          model,
				Logf:           func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) },
				// Prompts and progress go to stderr so stdout stays clean for the report / --format json.
				Approver: secrets.NewTerminalApprover(cmd.InOrStdin(), cmd.ErrOrStderr()),
			}

			if opts.Secrets == app.SecretsOff {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"WARNING: --secrets off disables sensitive-information screening. "+
						"Source text goes to the model unredacted; use only on trusted or synthetic code.")
			}

			result, err := app.ExtractRules(cmd.Context(), opts)
			if err != nil {
				return err
			}
			return renderRules(cmd.OutOrStdout(), cmd.ErrOrStderr(), result, format)
		},
	}

	cmd.Flags().StringVar(&target, "target", "", "limit to a package, class or method (pkg, pkg.Type or pkg.Type#method)")
	cmd.Flags().StringVar(&mode, "mode", "console", "output mode: console | sidecar (.docsmaker/rules/RULES.md and one file per class)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text (Markdown report) | json")
	cmd.Flags().IntVar(&minScore, "min-score", rules.DefaultMinScore, "lowest structural score a method needs to be analysed (higher = fewer, more promising methods)")
	cmd.Flags().IntVar(&maxMethods, "max-methods", 0, "analyse at most N methods, the highest-scoring first (0 = no limit)")
	cmd.Flags().StringVar(&minConfidence, "min-confidence", "", "hide rules less trustworthy than low|medium|high (default: show all)")
	cmd.Flags().StringVar(&outputLang, "output-lang", "pt", "language of the rules and the report: pt | en | any language the model writes")
	cmd.Flags().StringVar(&secretsPolicy, "secrets", "prompt",
		"sensitive-information policy: prompt (ask: redact/skip/allow once/abort), strict (block any finding), off (no screening)")
	cmd.Flags().BoolVar(&yes, "yes", false, "non-interactive: never prompt; any sensitive finding blocks the run")
	cmd.Flags().StringVar(&model, "model", "", "Ollama model to use (defaults to the tool's built-in default)")

	return cmd
}

func renderRules(out, errOut io.Writer, result *app.ExtractRulesResult, format string) error {
	if format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if result.Hidden > 0 {
		fmt.Fprintf(errOut, "%d rule(s) hidden by --min-confidence\n", result.Hidden)
	}
	if len(result.Results) == 0 {
		fmt.Fprintln(errOut, "no candidate methods: nothing in scope scores high enough; try a lower --min-score")
	}
	if result.OutputDir != "" {
		fmt.Fprintf(errOut, "report written to %s\n", result.OutputDir)
	}
	_, err := io.WriteString(out, result.Markdown())
	return err
}
