//go:build integration

package teams

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

func newTestTeam(t *testing.T, handler http.Handler) *Team {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &Team{cfg: &config.Config{ApiUrl: server.URL}}
}

func newTestMembership(t *testing.T, handler http.Handler) *Membership {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &Membership{cfg: &config.Config{ApiUrl: server.URL}}
}

func newTestRepoAccess(t *testing.T, handler http.Handler) *RepositoryAccess {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &RepositoryAccess{cfg: &config.Config{ApiUrl: server.URL}}
}

func githubTeamResponse(id int64, name, slug, org string) map[string]interface{} {
	return map[string]interface{}{
		"id":                   id,
		"name":                 name,
		"slug":                 slug,
		"description":          "A test team",
		"privacy":              "closed",
		"notification_setting": "notifications_enabled",
		"permission":           "pull",
		"html_url":             "https://github.com/orgs/" + org + "/teams/" + slug,
		"organization":         map[string]interface{}{"login": org},
	}
}
