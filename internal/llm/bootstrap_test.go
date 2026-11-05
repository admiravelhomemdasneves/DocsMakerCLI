package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestResolveBaseURL(t *testing.T) {
	ok := map[string]string{
		"":                       DefaultBaseURL,
		"localhost":              "http://localhost:11434",
		"127.0.0.1:9999":         "http://127.0.0.1:9999",
		"http://localhost:8080":  "http://localhost:8080",
		":11500":                 "http://127.0.0.1:11500",
		"0.0.0.0":                "http://127.0.0.1:11434",
		"[::1]:11434":            "http://[::1]:11434",
		"https://127.0.0.1:1234": "https://127.0.0.1:1234",
	}
	for in, want := range ok {
		got, err := resolveBaseURL(in)
		if err != nil || got != want {
			t.Errorf("resolveBaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"example.com", "10.0.0.5:11434", "http://my-gpu-box:11434", "192.168.1.20", "ftp://localhost"} {
		if got, err := resolveBaseURL(in); err == nil {
			t.Errorf("resolveBaseURL(%q) = %q, want an error (not this machine)", in, got)
		}
	}
}

func TestCheckLocalModel(t *testing.T) {
	for _, n := range []string{"qwen2.5-coder:7b", "llama3", "qwen2.5-coder:1.5b"} {
		if err := CheckLocalModel(n); err != nil {
			t.Errorf("%q should be allowed: %v", n, err)
		}
	}
	for _, n := range []string{"gpt-oss:120b-cloud", "qwen3-coder:480b-cloud", "foo:cloud", "", "FOO:120B-CLOUD"} {
		if err := CheckLocalModel(n); err == nil {
			t.Errorf("%q should be rejected", n)
		}
	}
}

func fakeOllama(t *testing.T, pull string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"version":"0.9.9"}`) })
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"models":[{"name":"qwen2.5-coder:1.5b","size":986062089},{"name":"llama3:latest","size":1}]}`)
	})
	mux.HandleFunc("/api/pull", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, pull) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestManager(url string) *Manager {
	m := NewManager(url)
	m.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	m.GOOS = "linux"
	return m
}

func TestStatusAndModels(t *testing.T) {
	m := newTestManager(fakeOllama(t, "").URL)
	st := m.Status(context.Background())
	if !st.Running || st.Version != "0.9.9" || !st.Installed {
		t.Fatalf("status = %+v", st)
	}
	for name, want := range map[string]bool{
		"qwen2.5-coder:1.5b": true, "qwen2.5-coder:7b": false, "llama3": true, "llama3:latest": true,
	} {
		got, err := m.HasModel(context.Background(), name)
		if err != nil || got != want {
			t.Errorf("HasModel(%q) = %v, %v; want %v", name, got, err, want)
		}
	}
}

func TestStatus_NotRunningNotInstalled(t *testing.T) {
	m := newTestManager("http://127.0.0.1:1") // nothing listens there
	if st := m.Status(context.Background()); st.Running || st.Installed {
		t.Fatalf("status = %+v", st)
	}
	if _, err := m.EnsureRunning(context.Background()); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("err = %v", err)
	}
}

func TestPull_ReportsProgressAndRequiresSuccess(t *testing.T) {
	good := `{"status":"pulling manifest"}` + "\n" +
		`{"status":"pulling abc","digest":"abc","total":100,"completed":40}` + "\n" +
		`{"status":"pulling abc","digest":"abc","total":100,"completed":100}` + "\n" +
		`{"status":"success"}` + "\n"
	var updates []PullProgress
	err := newTestManager(fakeOllama(t, good).URL).Pull(context.Background(), "x:1b", func(p PullProgress) { updates = append(updates, p) })
	if err != nil || len(updates) != 4 || updates[2].Completed != 100 {
		t.Fatalf("err=%v updates=%+v", err, updates)
	}

	failed := `{"status":"pulling manifest"}` + "\n" + `{"error":"pull model manifest: file does not exist"}` + "\n"
	if err := newTestManager(fakeOllama(t, failed).URL).Pull(context.Background(), "nope", nil); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v", err)
	}

	cut := `{"status":"pulling abc","total":100,"completed":10}` + "\n"
	if err := newTestManager(fakeOllama(t, cut).URL).Pull(context.Background(), "x", nil); err == nil {
		t.Fatal("a stream that ends without success must be an error")
	}
}

func TestPull_RefusesCloudModelsWithoutContactingOllama(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	if err := newTestManager(srv.URL).Pull(context.Background(), "gpt-oss:120b-cloud", nil); err == nil {
		t.Fatal("expected a refusal")
	}
	if hits.Load() != 0 {
		t.Fatal("must not contact Ollama for a cloud model")
	}
}

func TestEnsureRunning_StartsInstalledServer(t *testing.T) {
	var up atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"version":"1"}`)
	}))
	defer srv.Close()

	m := newTestManager(srv.URL)
	m.LookPath = func(string) (string, error) { return "/bin/ollama", nil }
	var started string
	m.StartServe = func(exe string) error { started = exe; up.Store(true); return nil }

	did, err := m.EnsureRunning(context.Background())
	if err != nil || !did || started != "/bin/ollama" {
		t.Fatalf("did=%v err=%v started=%q", did, err, started)
	}
	if did, err := m.EnsureRunning(context.Background()); err != nil || did {
		t.Fatalf("already running: did=%v err=%v", did, err)
	}
}

func TestPlanInstall(t *testing.T) {
	has := func(names ...string) func(string) (string, error) {
		return func(f string) (string, error) {
			for _, n := range names {
				if n == f {
					return "/x/" + f, nil
				}
			}
			return "", errors.New("missing")
		}
	}
	cases := []struct {
		goos string
		have []string
		want string // joined command, "" for manual
	}{
		{"windows", []string{"winget"}, "winget install --id Ollama.Ollama -e"},
		{"windows", nil, ""},
		{"darwin", []string{"brew"}, "brew install ollama"},
		{"linux", []string{"curl", "sh"}, ""}, // never pipes a script into a shell
	}
	for _, c := range cases {
		m := newTestManager("http://x")
		m.GOOS, m.LookPath = c.goos, has(c.have...)
		plan := m.PlanInstall()
		if got := strings.Join(plan.Command, " "); got != c.want {
			t.Errorf("%s %v: command %q, want %q", c.goos, c.have, got, c.want)
		}
		if c.want == "" && plan.Manual == "" {
			t.Errorf("%s: manual instructions missing", c.goos)
		}
	}
}
