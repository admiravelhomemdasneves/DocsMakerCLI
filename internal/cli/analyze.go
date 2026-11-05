package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/app"
	"github.com/admiravelhomemdasneves/DocsMakerCLI/internal/langmodel"
	"github.com/spf13/cobra"
)

func newAnalyzeCmd() *cobra.Command {
	var format string
	var out string

	cmd := &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze project structure without calling the local model",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) == 1 {
				path = args[0]
			}

			result, err := app.Analyze(cmd.Context(), app.AnalyzeOptions{ProjectPath: path})
			if err != nil {
				return err
			}

			return renderAnalyze(cmd, result, format, out)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	cmd.Flags().StringVar(&out, "out", "", "write output to this file instead of stdout")

	return cmd
}

func renderAnalyze(cmd *cobra.Command, result *app.AnalyzeResult, format, out string) error {
	w := cmd.OutOrStdout()
	if out != "" {
		f, err := os.Create(out)
		if err != nil {
			return fmt.Errorf("analyze: opening --out file: %w", err)
		}
		defer f.Close()
		w = f
	}

	switch format {
	case "json":
		return renderAnalyzeJSON(w, result)
	case "text", "":
		return renderAnalyzeText(w, result)
	default:
		return fmt.Errorf("analyze: unknown --format %q, expected text|json", format)
	}
}

func renderAnalyzeJSON(w io.Writer, result *app.AnalyzeResult) error {
	payload := struct {
		Model       *langmodel.CodeModel   `json:"model"`
		Diagnostics []langmodel.Diagnostic `json:"diagnostics,omitempty"`
	}{
		Model:       result.Model,
		Diagnostics: result.Diagnostics,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func renderAnalyzeText(w io.Writer, result *app.AnalyzeResult) error {
	model := result.Model

	fmt.Fprintf(w, "language: %s\n", model.Language)

	for _, pkg := range model.Packages {
		name := pkg.Name
		if name == "" {
			name = "(default package)"
		}
		fmt.Fprintf(w, "\npackage %s\n", name)

		for _, t := range pkg.Types {
			fmt.Fprintf(w, "  %s %s%s\n", t.Kind, t.Name, extendsImplementsSuffix(t))
			if t.ExistingDoc != "" {
				fmt.Fprintf(w, "    (has existing doc comment)\n")
			}
			for _, f := range t.Fields {
				fmt.Fprintf(w, "    field %s: %s\n", f.Name, formatTypeRef(f.Type))
			}
			for _, m := range t.Methods {
				fmt.Fprintf(w, "    method %s(%s)%s\n", m.Name, formatParams(m.Params), formatReturn(m.ReturnType))
				for _, c := range m.CallsOut {
					status := "unresolved"
					if c.Resolved {
						status = "resolved"
					}
					fmt.Fprintf(w, "      calls %s.%s [%s]\n", formatTypeRef(c.Receiver), c.MethodName, status)
				}
			}
		}
	}

	if len(model.Relationships) > 0 {
		fmt.Fprintf(w, "\nrelationships:\n")
		for _, r := range model.Relationships {
			fmt.Fprintf(w, "  %s -%s-> %s\n", formatTypeRef(r.From), r.Kind, formatTypeRef(r.To))
		}
	}

	if len(result.Diagnostics) > 0 {
		fmt.Fprintf(w, "\ndiagnostics:\n")
		for _, d := range result.Diagnostics {
			level := "error"
			if d.Warning {
				level = "warning"
			}
			fmt.Fprintf(w, "  [%s] %s: %s\n", level, d.File, d.Message)
		}
	}

	return nil
}

func extendsImplementsSuffix(t langmodel.TypeDecl) string {
	var parts []string
	if len(t.Extends) > 0 {
		parts = append(parts, "extends "+joinTypeRefs(t.Extends))
	}
	if len(t.Implements) > 0 {
		parts = append(parts, "implements "+joinTypeRefs(t.Implements))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func joinTypeRefs(refs []langmodel.TypeRef) string {
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = formatTypeRef(r)
	}
	return strings.Join(names, ", ")
}

func formatTypeRef(r langmodel.TypeRef) string {
	if r.External {
		return r.Name + " (external)"
	}
	return r.Name
}

func formatParams(params []langmodel.Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.Name + ": " + formatTypeRef(p.Type)
	}
	return strings.Join(parts, ", ")
}

func formatReturn(t langmodel.TypeRef) string {
	if t.Name == "" {
		return ""
	}
	return ": " + formatTypeRef(t)
}
