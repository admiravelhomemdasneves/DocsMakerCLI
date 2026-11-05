package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/andresantos/docsmakercli/internal/llm"
)

// ErrDeclined is returned when the developer says no to an install or
// download. It is a normal outcome, not a failure of the tool.
var ErrDeclined = errors.New("declined by the user")

func newManager() (*llm.Manager, error) {
	base, err := llm.ResolveBaseURL()
	if err != nil {
		return nil, err
	}
	return llm.NewManager(base), nil
}

// ModelStatusResult is the structured outcome of ModelStatus.
type ModelStatusResult struct {
	llm.Status
	DefaultModel      string `json:"defaultModel"`
	DefaultModelReady bool   `json:"defaultModelReady"`
	Hint              string `json:"hint,omitempty"`
}

// ModelStatus reports on the local Ollama runtime without changing anything.
func ModelStatus(ctx context.Context) (*ModelStatusResult, error) {
	mgr, err := newManager()
	if err != nil {
		return nil, err
	}
	res := &ModelStatusResult{Status: mgr.Status(ctx), DefaultModel: llm.DefaultModel}
	switch {
	case !res.Installed:
		res.Hint = "Ollama is not installed: run `docsmaker model pull`."
	case !res.Running:
		res.Hint = "Ollama is installed but not running: `docsmaker doc` starts it, or run `ollama serve`."
	default:
		if res.DefaultModelReady, err = mgr.HasModel(ctx, llm.DefaultModel); err != nil {
			return nil, err
		}
		if !res.DefaultModelReady {
			res.Hint = "The default model is not downloaded: run `docsmaker model pull`."
		}
	}
	return res, nil
}

// ModelList lists the models available locally. It starts a stopped Ollama.
func ModelList(ctx context.Context) ([]llm.ModelInfo, error) {
	mgr, err := newManager()
	if err != nil {
		return nil, err
	}
	if _, err := mgr.EnsureRunning(ctx); err != nil {
		return nil, err
	}
	return mgr.List(ctx)
}

// ModelPullOptions configures ModelPull.
type ModelPullOptions struct {
	Model string // empty = llm.DefaultModel
	// Confirm is asked before anything is installed or downloaded. It must
	// return true only on an explicit yes. nil means "never confirmed", so
	// a nil Confirm can only reuse what is already on the machine.
	Confirm func(question string) (bool, error)
	// Progress receives human-readable steps and download updates; may be nil.
	Log      func(format string, args ...any)
	Progress func(llm.PullProgress)
}

// ModelPullResult says what ModelPull did.
type ModelPullResult struct {
	Model            string `json:"model"`
	InstalledOllama  bool   `json:"installedOllama"`
	StartedOllama    bool   `json:"startedOllama"`
	DownloadedModel  bool   `json:"downloadedModel"`
	AlreadyAvailable bool   `json:"alreadyAvailable"`
}

// ModelPull makes sure a local model is ready to use: installing Ollama if
// missing, starting it if stopped, and downloading the model if absent. Each
// step that installs software or downloads data is confirmed first.
func ModelPull(ctx context.Context, opts ModelPullOptions) (*ModelPullResult, error) {
	model := opts.Model
	if model == "" {
		model = llm.DefaultModel
	}
	if err := llm.CheckLocalModel(model); err != nil {
		return nil, err
	}
	mgr, err := newManager()
	if err != nil {
		return nil, err
	}
	logf := opts.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	confirm := func(q string) error {
		if opts.Confirm == nil {
			return fmt.Errorf("%w: confirmation required but none available (use --yes to allow)", ErrDeclined)
		}
		ok, err := opts.Confirm(q)
		if err != nil {
			return err
		}
		if !ok {
			return ErrDeclined
		}
		return nil
	}

	res := &ModelPullResult{Model: model}

	if st := mgr.Status(ctx); !st.Installed {
		plan := mgr.PlanInstall()
		if len(plan.Command) == 0 {
			return nil, fmt.Errorf("app: Ollama is not installed. %s", plan.Manual)
		}
		if err := confirm(fmt.Sprintf(
			"Ollama is not installed. Install it by running `%s`? This downloads the Ollama runtime from the internet; no source code is involved.",
			strings.Join(plan.Command, " "))); err != nil {
			return nil, err
		}
		if err := mgr.Install(ctx, plan); err != nil {
			return nil, err
		}
		res.InstalledOllama = true
		if st := mgr.Status(ctx); !st.Installed {
			return nil, errors.New("app: the installer finished but Ollama was not found; open a new terminal and re-run `docsmaker model pull`")
		}
	}

	started, err := mgr.EnsureRunning(ctx)
	if err != nil {
		return nil, err
	}
	res.StartedOllama = started
	if started {
		logf("started the local Ollama server")
	}

	has, err := mgr.HasModel(ctx, model)
	if err != nil {
		return nil, err
	}
	if has {
		res.AlreadyAvailable = true
		return res, nil
	}

	if err := confirm(fmt.Sprintf(
		"Download the model %q from the Ollama registry? It can be several GB. Only the model comes down; nothing from your code is uploaded.", model)); err != nil {
		return nil, err
	}
	logf("downloading %s", model)
	if err := mgr.Pull(ctx, model, opts.Progress); err != nil {
		return nil, err
	}
	res.DownloadedModel = true
	return res, nil
}

// ensureLocalModel is the pre-flight check for doc generation: it starts a
// stopped Ollama (harmless and local) but never installs or downloads
// anything unprompted, and fails early with the command that fixes it.
func ensureLocalModel(ctx context.Context, model string, logf func(string, ...any)) (baseURL string, err error) {
	if err := llm.CheckLocalModel(model); err != nil {
		return "", err
	}
	mgr, err := newManager()
	if err != nil {
		return "", err
	}
	started, err := mgr.EnsureRunning(ctx)
	if err != nil {
		return "", fmt.Errorf("app: local Ollama is not available: %w", err)
	}
	if started && logf != nil {
		logf("started the local Ollama server")
	}
	has, err := mgr.HasModel(ctx, model)
	if err != nil {
		return "", err
	}
	if !has {
		return "", fmt.Errorf("app: model %q is not available locally; run `docsmaker model pull %s`", model, model)
	}
	return mgr.BaseURL, nil
}
