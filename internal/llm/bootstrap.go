package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Manager installs, starts and queries the local Ollama runtime. It never
// does anything that touches the network or the system without being told:
// installing and pulling models are separate, explicit calls, and the
// driver is expected to ask the developer first.
type Manager struct {
	BaseURL string
	HTTP    *http.Client // short timeout; for status/list calls
	Stream  *http.Client // no overall timeout; for model downloads

	GOOS     string
	LookPath func(file string) (string, error)
	// StartServe launches `ollama serve` detached from this process.
	StartServe func(exe string) error
	// Exec runs an installer command attached to the developer's terminal.
	Exec func(ctx context.Context, name string, args ...string) error
}

// NewManager builds a Manager for baseURL (DefaultBaseURL if empty).
func NewManager(baseURL string) *Manager {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Manager{
		BaseURL:    baseURL,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
		Stream:     &http.Client{},
		GOOS:       runtime.GOOS,
		LookPath:   exec.LookPath,
		StartServe: startDetached,
		Exec:       execAttached,
	}
}

// Status describes the local Ollama runtime.
type Status struct {
	Installed  bool   `json:"installed"`
	Executable string `json:"executable,omitempty"`
	Running    bool   `json:"running"`
	Version    string `json:"version,omitempty"`
	BaseURL    string `json:"baseUrl"`
}

// Status inspects the runtime without changing anything.
func (m *Manager) Status(ctx context.Context) Status {
	st := Status{BaseURL: m.BaseURL}
	if exe, ok := m.findExecutable(); ok {
		st.Installed, st.Executable = true, exe
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := m.getJSON(ctx, "/api/version", &v); err == nil {
		st.Running, st.Version = true, v.Version
		st.Installed = true // something is serving the API
	}
	return st
}

// findExecutable looks on PATH, then at the places the official installers
// use (PATH is not refreshed in a terminal opened before the install).
func (m *Manager) findExecutable() (string, bool) {
	if p, err := m.LookPath("ollama"); err == nil {
		return p, true
	}
	var candidates []string
	switch m.GOOS {
	case "windows":
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			candidates = append(candidates, filepath.Join(dir, "Programs", "Ollama", "ollama.exe"))
		}
	case "darwin":
		candidates = append(candidates, "/Applications/Ollama.app/Contents/Resources/ollama", "/opt/homebrew/bin/ollama", "/usr/local/bin/ollama")
	default:
		candidates = append(candidates, "/usr/local/bin/ollama", "/usr/bin/ollama")
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, true
		}
	}
	return "", false
}

// InstallPlan says how Ollama would be installed on this platform. When
// Command is empty the install must be done by hand (Manual explains how).
type InstallPlan struct {
	Command []string
	Manual  string
}

// PlanInstall picks the platform's package manager. It deliberately never
// pipes a downloaded script into a shell: on Linux the developer installs
// Ollama themselves.
func (m *Manager) PlanInstall() InstallPlan {
	const manual = "Install Ollama from https://ollama.com/download, then run `docsmaker model pull`."
	switch m.GOOS {
	case "windows":
		if _, err := m.LookPath("winget"); err == nil {
			return InstallPlan{Command: []string{"winget", "install", "--id", "Ollama.Ollama", "-e"}}
		}
	case "darwin":
		if _, err := m.LookPath("brew"); err == nil {
			return InstallPlan{Command: []string{"brew", "install", "ollama"}}
		}
	}
	return InstallPlan{Manual: manual}
}

// Install runs plan.Command. The package manager runs attached to the
// terminal, so any agreement it asks the developer to accept is shown and
// answered by them, not by docsmaker.
func (m *Manager) Install(ctx context.Context, plan InstallPlan) error {
	if len(plan.Command) == 0 {
		return errors.New("llm: no automatic install available on this platform; " + plan.Manual)
	}
	if err := m.Exec(ctx, plan.Command[0], plan.Command[1:]...); err != nil {
		return fmt.Errorf("llm: installing Ollama (%s): %w", strings.Join(plan.Command, " "), err)
	}
	return nil
}

// EnsureRunning starts `ollama serve` if the API is not reachable and
// Ollama is installed. It returns true when it had to start it.
func (m *Manager) EnsureRunning(ctx context.Context) (started bool, err error) {
	st := m.Status(ctx)
	if st.Running {
		return false, nil
	}
	if !st.Installed {
		return false, errors.New("llm: Ollama is not installed; run `docsmaker model pull`")
	}
	if err := m.StartServe(st.Executable); err != nil {
		return false, fmt.Errorf("llm: starting Ollama: %w", err)
	}

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status(ctx).Running {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return true, fmt.Errorf("llm: Ollama did not become reachable at %s within 30s", m.BaseURL)
}

// ModelInfo is one locally available model.
type ModelInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// List returns the models available locally.
func (m *Manager) List(ctx context.Context) ([]ModelInfo, error) {
	var tags struct {
		Models []ModelInfo `json:"models"`
	}
	if err := m.getJSON(ctx, "/api/tags", &tags); err != nil {
		return nil, fmt.Errorf("llm: listing models: %w", err)
	}
	return tags.Models, nil
}

// HasModel reports whether name is available locally. A name without a tag
// means ":latest", as in Ollama itself.
func (m *Manager) HasModel(ctx context.Context, name string) (bool, error) {
	models, err := m.List(ctx)
	if err != nil {
		return false, err
	}
	want := withTag(name)
	for _, mi := range models {
		if withTag(mi.Name) == want {
			return true, nil
		}
	}
	return false, nil
}

func withTag(name string) string {
	if strings.Contains(name, ":") {
		return name
	}
	return name + ":latest"
}

// PullProgress is one progress update from a model download.
type PullProgress struct {
	Status    string
	Digest    string
	Total     int64
	Completed int64
}

// Pull downloads model through the local Ollama (the one network operation
// in the tool, and only the model comes down — nothing is uploaded).
// progress may be nil.
func (m *Manager) Pull(ctx context.Context, model string, progress func(PullProgress)) error {
	if err := CheckLocalModel(model); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"model": model, "stream": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.BaseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.Stream.Do(req)
	if err != nil {
		return fmt.Errorf("llm: pull request failed (is Ollama running?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("llm: ollama returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sawSuccess := false
	for sc.Scan() {
		var line struct {
			Status    string `json:"status"`
			Digest    string `json:"digest"`
			Total     int64  `json:"total"`
			Completed int64  `json:"completed"`
			Error     string `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		if line.Error != "" {
			return fmt.Errorf("llm: pulling %s: %s", model, line.Error)
		}
		if line.Status == "success" {
			sawSuccess = true
		}
		if progress != nil {
			progress(PullProgress{Status: line.Status, Digest: line.Digest, Total: line.Total, Completed: line.Completed})
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("llm: reading pull progress: %w", err)
	}
	if !sawSuccess {
		return fmt.Errorf("llm: pull of %s ended without a success status (interrupted?)", model)
	}
	return nil
}

func (m *Manager) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %s for %s", resp.Status, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func execAttached(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func startDetached(exe string) error {
	cmd := exec.Command(exe, "serve")
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
