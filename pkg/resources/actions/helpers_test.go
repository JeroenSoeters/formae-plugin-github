//go:build integration

package actions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
)

func jsonResponse(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func newTestVariable(t *testing.T, handler http.Handler) *Variable {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &Variable{cfg: &config.Config{ApiUrl: server.URL}}
}

func newTestSecret(t *testing.T, handler http.Handler) *Secret {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &Secret{cfg: &config.Config{ApiUrl: server.URL}}
}

func newTestWorkflowRun(t *testing.T, handler http.Handler) *WorkflowRun {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &WorkflowRun{cfg: &config.Config{ApiUrl: server.URL}}
}
