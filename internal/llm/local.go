package llm

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// The tool's core promise is that no source code leaves the machine. Two
// things in the Ollama ecosystem could silently break it, so both are
// refused here rather than left to the user's environment:
//
//  1. OLLAMA_HOST pointing at another machine;
//  2. "cloud" models, which the local Ollama forwards to Ollama's servers.

// ResolveBaseURL returns the Ollama API address to use: DefaultBaseURL, or
// the OLLAMA_HOST environment variable if set, but only when it points at
// this machine.
func ResolveBaseURL() (string, error) {
	return resolveBaseURL(os.Getenv("OLLAMA_HOST"))
}

func resolveBaseURL(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return DefaultBaseURL, nil
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Hostname() == "" && u.Host == "" {
		return "", fmt.Errorf("llm: cannot parse OLLAMA_HOST %q", host)
	}

	name := u.Hostname()
	switch {
	case name == "" || name == "0.0.0.0" || name == "::":
		name = "127.0.0.1" // a bind-all address used as a client target means "this machine"
	case name == "localhost":
	default:
		ip := net.ParseIP(name)
		if ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("llm: OLLAMA_HOST points at %q, which is not this machine; "+
				"docsmaker only talks to a local Ollama so that no code leaves the machine", name)
		}
	}

	port := u.Port()
	if port == "" {
		port = "11434"
	}
	scheme := u.Scheme
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("llm: unsupported OLLAMA_HOST scheme %q", scheme)
	}
	return scheme + "://" + net.JoinHostPort(name, port), nil
}

// CheckLocalModel rejects Ollama "cloud" models (e.g. "gpt-oss:120b-cloud"):
// they look like local models but run on Ollama's servers.
func CheckLocalModel(name string) error {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return fmt.Errorf("llm: empty model name")
	}
	if strings.HasSuffix(n, "cloud") || strings.Contains(n, "-cloud") || strings.Contains(n, ":cloud") {
		return fmt.Errorf("llm: model %q is a cloud model that runs on remote servers; "+
			"docsmaker only uses models that run on this machine", name)
	}
	return nil
}
