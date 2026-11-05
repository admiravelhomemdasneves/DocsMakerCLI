package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/andresantos/docsmakercli/internal/app"
	"github.com/andresantos/docsmakercli/internal/llm"
	"github.com/spf13/cobra"
)

func newModelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Manage the local Ollama runtime and models",
		Long: "Everything docsmaker generates runs on a local Ollama. `model pull` is the one\n" +
			"place that may use the network, to install Ollama and download a model; it asks\n" +
			"first, and nothing from your code is ever uploaded.",
	}

	var yes bool
	pull := &cobra.Command{
		Use:   "pull [model]",
		Short: "Install/start Ollama if needed and download a model (default: " + llm.DefaultModel + ")",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := app.ModelPullOptions{
				Log:      func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) },
				Progress: newPullRenderer(cmd.ErrOrStderr()),
			}
			if len(args) == 1 {
				opts.Model = args[0]
			}
			if yes {
				opts.Confirm = func(string) (bool, error) { return true, nil }
			} else {
				opts.Confirm = terminalConfirm(cmd.InOrStdin(), cmd.ErrOrStderr())
			}

			res, err := app.ModelPull(cmd.Context(), opts)
			if errors.Is(err, app.ErrDeclined) {
				fmt.Fprintln(cmd.ErrOrStderr(), "Nothing was installed or downloaded.")
				return err
			}
			if err != nil {
				return err
			}
			switch {
			case res.DownloadedModel:
				fmt.Fprintf(cmd.OutOrStdout(), "model %s is ready\n", res.Model)
			case res.AlreadyAvailable:
				fmt.Fprintf(cmd.OutOrStdout(), "model %s is already available\n", res.Model)
			}
			return nil
		},
	}
	pull.Flags().BoolVar(&yes, "yes", false, "answer yes to the install/download confirmations")

	var format string
	list := &cobra.Command{
		Use:   "list",
		Short: "List models available locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			models, err := app.ModelList(cmd.Context())
			if err != nil {
				return err
			}
			if format == "json" {
				return writeJSON(cmd.OutOrStdout(), models)
			}
			if len(models) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no local models; run `docsmaker model pull`")
			}
			for _, m := range models {
				fmt.Fprintf(cmd.OutOrStdout(), "%-40s %s\n", m.Name, humanBytes(m.Size))
			}
			return nil
		},
	}
	list.Flags().StringVar(&format, "format", "text", "output format: text|json")

	status := &cobra.Command{
		Use:   "status",
		Short: "Report whether Ollama is installed and reachable",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := app.ModelStatus(cmd.Context())
			if err != nil {
				return err
			}
			if format == "json" {
				return writeJSON(cmd.OutOrStdout(), st)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "installed:     %v", st.Installed)
			if st.Executable != "" {
				fmt.Fprintf(w, " (%s)", st.Executable)
			}
			fmt.Fprintf(w, "\nrunning:       %v", st.Running)
			if st.Version != "" {
				fmt.Fprintf(w, " (version %s)", st.Version)
			}
			fmt.Fprintf(w, "\naddress:       %s\ndefault model: %s (downloaded: %v)\n", st.BaseURL, st.DefaultModel, st.DefaultModelReady)
			if st.Hint != "" {
				fmt.Fprintf(w, "\n%s\n", st.Hint)
			}
			return nil
		},
	}
	status.Flags().StringVar(&format, "format", "text", "output format: text|json")

	cmd.AddCommand(pull, list, status)
	return cmd
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// terminalConfirm asks a yes/no question on out and reads the answer from
// in. Anything but an explicit yes (including EOF) is a no.
func terminalConfirm(in io.Reader, out io.Writer) func(string) (bool, error) {
	r := bufio.NewReader(in)
	return func(question string) (bool, error) {
		fmt.Fprintf(out, "\n%s\nProceed? [y/N]: ", question)
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			fmt.Fprintln(out)
			return false, nil
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		return answer == "y" || answer == "yes", nil
	}
}

// newPullRenderer prints download progress on one updating line per layer.
func newPullRenderer(out io.Writer) func(llm.PullProgress) {
	lastStatus := ""
	midLine := false
	return func(p llm.PullProgress) {
		if p.Total > 0 {
			fmt.Fprintf(out, "\r%-30s %3d%%  %s / %s   ", truncate(p.Status, 30),
				p.Completed*100/p.Total, humanBytes(p.Completed), humanBytes(p.Total))
			midLine = true
		} else if p.Status != lastStatus {
			if midLine {
				fmt.Fprintln(out)
				midLine = false
			}
			fmt.Fprintln(out, p.Status)
		}
		lastStatus = p.Status
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
