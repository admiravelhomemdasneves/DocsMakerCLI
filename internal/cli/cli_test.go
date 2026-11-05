package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "github.com/admiravelhomemdasneves/DocsMakerCLI/internal/lang/java"
)

// fakeOllama is a local stand-in for the Ollama API. Pointing OLLAMA_HOST at
// it (loopback, so the tool accepts it) makes the CLI tests hermetic and lets
// them inspect exactly what a "model" would have been sent.
type fakeOllama struct {
	mu      sync.Mutex
	models  []string
	prompts []string
	reply   string
}

func startFakeOllama(t *testing.T, models ...string) *fakeOllama {
	t.Helper()
	f := &fakeOllama{models: models, reply: "A summary."}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"version":"test"}`) })
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var items []string
		for _, m := range f.models {
			items = append(items, fmt.Sprintf(`{"name":%q,"size":2048}`, m))
		}
		fmt.Fprintf(w, `{"models":[%s]}`, strings.Join(items, ","))
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Prompt string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.prompts = append(f.prompts, req.Prompt)
		reply := f.reply
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"response": reply, "done": true})
	})
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.mu.Lock()
		f.models = append(f.models, req.Model)
		f.mu.Unlock()
		fmt.Fprintln(w, `{"status":"pulling manifest"}`)
		fmt.Fprintln(w, `{"status":"pulling abc","total":100,"completed":100}`)
		fmt.Fprintln(w, `{"status":"success"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("OLLAMA_HOST", srv.URL)
	return f
}

func (f *fakeOllama) allPrompts() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.prompts, "\n---\n")
}

func (f *fakeOllama) hasModel(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, m := range f.models {
		if m == name {
			return true
		}
	}
	return false
}

// run executes the CLI in-process and returns what it wrote.
func run(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errb.String(), err
}

// sampleProject copies testdata/sample-project (minus build leftovers) to a
// temp dir, which holds a planted fake credential in DatabaseConfig.
func sampleProject(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", "sample-project")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if top := strings.Split(rel, string(filepath.Separator))[0]; top == ".docsmaker" || top == "target" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := sampleProject(t)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

const dbClass = "com.example.config.DatabaseConfig"

func TestVersionAndHelp(t *testing.T) {
	out, _, err := run(t, "", "--version")
	if err != nil || !strings.Contains(out, "docsmaker version") {
		t.Fatalf("--version: %q %v", out, err)
	}
	out, _, _ = run(t, "", "--help")
	for _, cmd := range []string{"analyze", "doc", "init", "model", "allowlist"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("--help does not list %q", cmd)
		}
	}
}

func TestAnalyze(t *testing.T) {
	dir := sampleProject(t)

	out, _, err := run(t, "", "analyze", dir)
	if err != nil || !strings.Contains(out, "package com.example.greeting") || !strings.Contains(out, "interface Greeter") {
		t.Fatalf("text output: %q %v", out, err)
	}

	out, _, err = run(t, "", "analyze", dir, "--format", "json")
	var parsed struct {
		Model struct{ Language string }
	}
	if err != nil || json.Unmarshal([]byte(out), &parsed) != nil || parsed.Model.Language != "java" {
		t.Fatalf("json output is not the expected document: %q %v", out, err)
	}

	if _, _, err := run(t, "", "analyze", dir, "--format", "xml"); err == nil {
		t.Fatal("an unknown format must be an error")
	}
	if _, _, err := run(t, "", "analyze", t.TempDir()); err == nil {
		t.Fatal("a folder without sources must be an error")
	}
}

func TestDoc_ConsoleAndInputValidation(t *testing.T) {
	fake := startFakeOllama(t, "qwen2.5-coder:7b")
	dir := sampleProject(t)

	out, _, err := run(t, "", "doc", dir, "--level", "class", "--target", "com.example.greeting.Greeter")
	if err != nil || !strings.Contains(out, "=== class: com.example.greeting.Greeter ===") || !strings.Contains(out, "A summary.") {
		t.Fatalf("console output: %q %v", out, err)
	}
	if !strings.Contains(fake.allPrompts(), "Greeter") {
		t.Fatal("the model was never sent the class")
	}

	out, _, err = run(t, "", "doc", dir, "--level", "method", "--target", "com.example.greeting.Greeter#greet", "--format", "json")
	var res struct {
		Artifacts []struct{ Level, Content string }
	}
	if err != nil || json.Unmarshal([]byte(out), &res) != nil || len(res.Artifacts) != 1 || res.Artifacts[0].Level != "method" {
		t.Fatalf("json output: %q %v", out, err)
	}

	for name, args := range map[string][]string{
		"method level needs a target": {"doc", dir, "--level", "method"},
		"unknown target":              {"doc", dir, "--level", "class", "--target", "no.such.Type"},
		"unknown mode":                {"doc", dir, "--mode", "banana"},
		"unknown level":               {"doc", dir, "--level", "galaxy"},
		"unknown secrets policy":      {"doc", dir, "--secrets", "maybe", "--level", "class", "--target", "com.example.greeting.Greeter"},
	} {
		if _, _, err := run(t, "", args...); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDoc_SidecarWritesFilesAndLeavesSourcesAlone(t *testing.T) {
	startFakeOllama(t, "qwen2.5-coder:7b")
	dir := sampleProject(t)
	before, _ := os.ReadFile(filepath.Join(dir, "src/main/java/com/example/greeting/Greeter.java"))

	if _, _, err := run(t, "", "doc", dir, "--mode", "sidecar", "--level", "class", "--target", "com.example.greeting.Greeter"); err != nil {
		t.Fatal(err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".docsmaker", "doc", "class", "*")); len(matches) == 0 {
		t.Fatal("no sidecar file written")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "src/main/java/com/example/greeting/Greeter.java"))
	if !bytes.Equal(before, after) {
		t.Fatal("sidecar mode must not touch the sources")
	}
}

func TestDoc_ModelMissingAndRefusals(t *testing.T) {
	startFakeOllama(t, "something-else:1b")
	dir := sampleProject(t)
	args := []string{"doc", dir, "--level", "class", "--target", "com.example.greeting.Greeter"}

	_, _, err := run(t, "", args...)
	if err == nil || !strings.Contains(err.Error(), "docsmaker model pull") {
		t.Fatalf("a missing model should point at `model pull`, got %v", err)
	}
	if _, _, err := run(t, "", append(args, "--model", "gpt-oss:120b-cloud")...); err == nil || !strings.Contains(err.Error(), "cloud") {
		t.Fatalf("cloud model must be refused, got %v", err)
	}

	t.Setenv("OLLAMA_HOST", "10.1.2.3:11434")
	if _, _, err := run(t, "", args...); err == nil || !strings.Contains(err.Error(), "not this machine") {
		t.Fatalf("a remote host must be refused, got %v", err)
	}
}

func TestDoc_Secrets(t *testing.T) {
	fake := startFakeOllama(t, "qwen2.5-coder:7b")
	dir := sampleProject(t)
	args := []string{"doc", dir, "--level", "class", "--target", dbClass}

	t.Run("--yes blocks and the model is never called", func(t *testing.T) {
		_, _, err := run(t, "", append(args, "--yes")...)
		if err == nil || strings.Contains(err.Error(), "hunter2") {
			t.Fatalf("want a blocking error without the secret, got %v", err)
		}
		if strings.Contains(fake.allPrompts(), "hunter2") {
			t.Fatal("the secret reached the model")
		}
	})

	t.Run("redact: the model sees a placeholder", func(t *testing.T) {
		out, errOut, err := run(t, "r\n", args...)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(errOut, "jdb***db") || strings.Contains(out+errOut, "hunter2") {
			t.Fatalf("prompt must show a masked value only\nstdout=%q\nstderr=%q", out, errOut)
		}
		if p := fake.allPrompts(); strings.Contains(p, "hunter2") || !strings.Contains(p, "[REDACTED:connection-string]") {
			t.Fatalf("model prompts should carry the placeholder, not the secret:\n%s", p)
		}
	})

	t.Run("quit aborts", func(t *testing.T) {
		if _, _, err := run(t, "q\n", args...); err == nil {
			t.Fatal("expected an abort")
		}
	})

	t.Run("closed stdin is an error, never an implicit yes", func(t *testing.T) {
		if _, _, err := run(t, "", args...); err == nil {
			t.Fatal("expected an error when nobody can answer")
		}
	})

	t.Run("allowlisting with w, then --yes passes and the list shows a hash", func(t *testing.T) {
		if _, _, err := run(t, "w\n", args...); err != nil {
			t.Fatal(err)
		}
		if _, _, err := run(t, "", append(args, "--yes")...); err != nil {
			t.Fatalf("an allowlisted value must not block: %v", err)
		}
		list, _, err := run(t, "", "allowlist", "list", "--project", dir)
		if err != nil || !strings.Contains(list, "connection-string") || strings.Contains(list, "hunter2") {
			t.Fatalf("list: %q %v", list, err)
		}
		raw, _ := os.ReadFile(filepath.Join(dir, ".docsmaker", "allowlist.json"))
		if strings.Contains(string(raw), "hunter2") {
			t.Fatal("the allowlist file holds the secret")
		}

		id := strings.Fields(strings.Split(list, "\n")[1])[0]
		if out, _, err := run(t, "", "allowlist", "remove", id, "--project", dir); err != nil || !strings.Contains(out, "removed") {
			t.Fatalf("remove: %q %v", out, err)
		}
		if _, _, err := run(t, "", append(args, "--yes")...); err == nil {
			t.Fatal("after removing the entry the value must block again")
		}
	})

	t.Run("--secrets off warns", func(t *testing.T) {
		_, errOut, err := run(t, "", append(args, "--secrets", "off")...)
		if err != nil || !strings.Contains(errOut, "WARNING") {
			t.Fatalf("want a warning, got %q %v", errOut, err)
		}
	})
}

func TestAllowlistCommands(t *testing.T) {
	dir := t.TempDir()
	out, _, err := run(t, "", "allowlist", "list", "--project", dir)
	if err != nil || !strings.Contains(out, "(empty)") {
		t.Fatalf("empty list: %q %v", out, err)
	}
	if _, _, err := run(t, "", "allowlist", "add-pattern", `example-[a-z]{4,}`, "--project", dir); err != nil {
		t.Fatal(err)
	}
	if out, _, _ := run(t, "", "allowlist", "list", "--project", dir); !strings.Contains(out, "pattern example-") {
		t.Fatalf("pattern not listed: %q", out)
	}
	if _, _, err := run(t, "", "allowlist", "add-pattern", ".*", "--project", dir); err == nil {
		t.Fatal("a pattern that allows everything must be refused")
	}
	if _, _, err := run(t, "", "allowlist", "remove", "ffffffffff", "--project", dir); err == nil {
		t.Fatal("removing an unknown id must fail")
	}
	if out, _, err := run(t, "", "allowlist", "remove", `example-[a-z]{4,}`, "--project", dir); err != nil || !strings.Contains(out, "removed pattern") {
		t.Fatalf("remove pattern: %q %v", out, err)
	}
}

func TestInitAndInline(t *testing.T) {
	startFakeOllama(t, "qwen2.5-coder:7b")
	dir := gitRepo(t)
	greeter := filepath.Join(dir, "src/main/java/com/example/greeting/Greeter.java")
	original, _ := os.ReadFile(greeter)
	inline := []string{"doc", dir, "--mode", "inline", "--level", "class", "--target", "com.example.greeting.Greeter"}

	_, _, err := run(t, "", inline...)
	if err == nil || !strings.Contains(err.Error(), "docsmaker init") {
		t.Fatalf("inline before init must be refused with a hint, got %v", err)
	}
	if now, _ := os.ReadFile(greeter); !bytes.Equal(now, original) {
		t.Fatal("refused run changed a file")
	}

	out, _, err := run(t, "", "init", dir)
	if err != nil || !strings.Contains(out, "updated") {
		t.Fatalf("init: %q %v", out, err)
	}
	if out, _, _ := run(t, "", "init", dir); strings.Contains(out, "updated") || !strings.Contains(out, "already set") {
		t.Fatalf("second init should change nothing: %q", out)
	}

	out, _, err = run(t, "", inline...)
	if err != nil || !strings.Contains(out, "=== inline:") {
		t.Fatalf("inline after init: %q %v", out, err)
	}
	written, _ := os.ReadFile(greeter)
	if !strings.Contains(string(written), "docsmaker:ai-begin") || !strings.Contains(string(written), "A summary.") {
		t.Fatalf("expected an AI block in the source:\n%s", written)
	}

	// And the hidden filter command git calls strips exactly that block.
	cleaned, _, err := run(t, string(written), "git-clean-filter", "clean")
	if err != nil || cleaned != string(original) {
		t.Fatalf("clean filter did not restore the original (err=%v):\n%s", err, cleaned)
	}
	if _, _, err := run(t, "", "git-clean-filter", "bogus"); err == nil {
		t.Fatal("unknown filter mode must fail")
	}
}

func TestInit_NotARepo(t *testing.T) {
	if _, _, err := run(t, "", "init", t.TempDir()); err == nil {
		t.Skip("temp dir is inside a git repository on this machine")
	}
}

func TestModelCommands(t *testing.T) {
	fake := startFakeOllama(t, "qwen2.5-coder:1.5b")

	out, _, err := run(t, "", "model", "status")
	if err != nil || !strings.Contains(out, "running:       true") || !strings.Contains(out, "downloaded: false") {
		t.Fatalf("status: %q %v", out, err)
	}
	var st struct{ Running bool }
	if jsonOut, _, err := run(t, "", "model", "status", "--format", "json"); err != nil || json.Unmarshal([]byte(jsonOut), &st) != nil || !st.Running {
		t.Fatalf("status json: %q %v", jsonOut, err)
	}

	if out, _, err := run(t, "", "model", "list"); err != nil || !strings.Contains(out, "qwen2.5-coder:1.5b") {
		t.Fatalf("list: %q %v", out, err)
	}

	if out, _, err := run(t, "", "model", "pull", "qwen2.5-coder:1.5b"); err != nil || !strings.Contains(out, "already available") {
		t.Fatalf("pull of a present model: %q %v", out, err)
	}

	t.Run("a download needs a yes", func(t *testing.T) {
		for _, answer := range []string{"", "n\n", "\n", "maybe\n"} {
			if _, _, err := run(t, answer, "model", "pull", "tiny:1b"); err == nil {
				t.Fatalf("answer %q must not download", answer)
			}
			if fake.hasModel("tiny:1b") {
				t.Fatalf("answer %q downloaded the model", answer)
			}
		}
		if _, _, err := run(t, "y\n", "model", "pull", "tiny:1b"); err != nil || !fake.hasModel("tiny:1b") {
			t.Fatalf("an explicit yes should download: %v", err)
		}
		if _, _, err := run(t, "", "model", "pull", "other:1b", "--yes"); err != nil || !fake.hasModel("other:1b") {
			t.Fatalf("--yes should download: %v", err)
		}
	})

	if _, _, err := run(t, "", "model", "pull", "gpt-oss:120b-cloud", "--yes"); err == nil {
		t.Fatal("cloud models must be refused even with --yes")
	}
}

func TestHelpers(t *testing.T) {
	cases := map[int64]string{0: "0 B", 1023: "1023 B", 1024: "1.0 KiB", 5 * 1024 * 1024: "5.0 MiB", 3 << 30: "3.0 GiB"}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}

	var out bytes.Buffer
	confirm := terminalConfirm(strings.NewReader("YES\n"), &out)
	if ok, _ := confirm("q?"); !ok {
		t.Error("YES should be accepted")
	}
	confirm = terminalConfirm(io.LimitReader(strings.NewReader(""), 0), &out)
	if ok, _ := confirm("q?"); ok {
		t.Error("EOF must be a no")
	}
}
