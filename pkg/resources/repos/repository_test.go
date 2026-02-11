//go:build integration

package repos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRepo creates a Repository provisioner backed by a mock GitHub API server.
// WithEnterpriseURLs prepends /api/v3/ to all paths, so handlers must be registered
// under that prefix.
func newTestRepo(t *testing.T, handler http.Handler) *Repository {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &Repository{cfg: &config.Config{ApiUrl: server.URL}}
}

// githubRepoResponse returns a map mimicking the GitHub API response for a repository.
func githubRepoResponse(owner, name, description, visibility string) map[string]interface{} {
	return map[string]interface{}{
		"id":               1,
		"name":             name,
		"full_name":        owner + "/" + name,
		"html_url":         "https://github.com/" + owner + "/" + name,
		"clone_url":        "https://github.com/" + owner + "/" + name + ".git",
		"ssh_url":          "git@github.com:" + owner + "/" + name + ".git",
		"description":      description,
		"visibility":       visibility,
		"default_branch":   "main",
		"has_issues":       true,
		"has_wiki":         true,
		"has_projects":     true,
		"owner":            map[string]interface{}{"login": owner},
	}
}

func jsonResponse(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func TestCreate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-owner/repos", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "test-repo", body["name"])
		assert.Equal(t, "A test repo", body["description"])

		jsonResponse(w, http.StatusCreated, githubRepoResponse("test-owner", "test-repo", "A test repo", "private"))
	})
	repo := newTestRepo(t, mux)

	// Execute
	props, _ := json.Marshal(repoProperties{
		Owner:       "test-owner",
		Name:        "test-repo",
		Description: "A test repo",
		Visibility:  "private",
	})
	result, err := repo.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo", result.ProgressResult.NativeID)

	var resultProps repoProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "test-repo", resultProps.Name)
	assert.Equal(t, "test-owner", resultProps.Owner)
	assert.Equal(t, "private", resultProps.Visibility)
	assert.Equal(t, "https://github.com/test-owner/test-repo", resultProps.HtmlUrl)
}

func TestCreateWithTopics(t *testing.T) {
	// Setup
	var topicsSet []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-owner/repos", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusCreated, githubRepoResponse("test-owner", "test-repo", "", "private"))
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/topics", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		names := body["names"].([]interface{})
		for _, n := range names {
			topicsSet = append(topicsSet, n.(string))
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{"names": body["names"]})
	})
	repo := newTestRepo(t, mux)

	// Execute
	props, _ := json.Marshal(repoProperties{
		Owner:      "test-owner",
		Name:       "test-repo",
		Visibility: "private",
		Topics:     []string{"go", "formae"},
	})
	result, err := repo.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, []string{"go", "formae"}, topicsSet)
}

func TestRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, githubRepoResponse("test-owner", "test-repo", "A repo", "public"))
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/topics", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"names": []string{"go", "test"},
		})
	})
	repo := newTestRepo(t, mux)

	// Execute
	result, err := repo.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo",
		ResourceType: ResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props repoProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-repo", props.Name)
	assert.Equal(t, "test-owner", props.Owner)
	assert.Equal(t, "public", props.Visibility)
	assert.Equal(t, "main", props.DefaultBranch)
	assert.Equal(t, []string{"go", "test"}, props.Topics)
}

func TestReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/gone", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	repo := newTestRepo(t, mux)

	// Execute
	result, err := repo.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/gone",
		ResourceType: ResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestUpdate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPatch, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "Updated description", body["description"])

		jsonResponse(w, http.StatusOK, githubRepoResponse("test-owner", "test-repo", "Updated description", "private"))
	})
	repo := newTestRepo(t, mux)

	// Execute
	props, _ := json.Marshal(repoProperties{
		Owner:       "test-owner",
		Name:        "test-repo",
		Description: "Updated description",
		Visibility:  "private",
	})
	result, err := repo.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-owner/test-repo",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo", result.ProgressResult.NativeID)

	var resultProps repoProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "Updated description", resultProps.Description)
}

func TestDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	repo := newTestRepo(t, mux)

	// Execute
	result, err := repo.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/gone", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	repo := newTestRepo(t, mux)

	// Execute
	result, err := repo.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/gone",
	})

	// Verify — 404 is treated as success (idempotent delete)
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestList(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/repos", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		repos := []interface{}{
			githubRepoResponse("test-org", "repo-a", "First", "public"),
			githubRepoResponse("test-org", "repo-b", "Second", "private"),
		}
		jsonResponse(w, http.StatusOK, repos)
	})
	repo := newTestRepo(t, mux)

	// Execute
	targetCfg, _ := json.Marshal(config.Config{Organization: "test-org"})
	result, err := repo.List(context.Background(), &resource.ListRequest{
		TargetConfig: targetCfg,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "list handler was not called")
	assert.Equal(t, []string{"test-org/repo-a", "test-org/repo-b"}, result.NativeIDs)
	assert.Nil(t, result.NextPageToken)
}
