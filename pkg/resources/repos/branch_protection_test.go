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

func newTestBP(t *testing.T, handler http.Handler) *BranchProtection {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	t.Setenv("GITHUB_TOKEN", "test-token")
	return &BranchProtection{cfg: &config.Config{ApiUrl: server.URL}}
}

func githubProtectionResponse(enforceAdmins bool) map[string]interface{} {
	return map[string]interface{}{
		"enforce_admins": map[string]interface{}{
			"enabled": enforceAdmins,
		},
		"required_linear_history": map[string]interface{}{
			"enabled": false,
		},
		"allow_force_pushes": map[string]interface{}{
			"enabled": false,
		},
		"allow_deletions": map[string]interface{}{
			"enabled": false,
		},
		"required_status_checks": map[string]interface{}{
			"strict":   true,
			"contexts": []string{"ci/test"},
			"checks": []interface{}{
				map[string]interface{}{"context": "ci/test"},
			},
		},
		"required_pull_request_reviews": map[string]interface{}{
			"required_approving_review_count": 2,
			"dismiss_stale_reviews":           true,
			"require_code_owner_reviews":      false,
			"require_last_push_approval":      false,
		},
	}
}

func TestBPCreate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, true, body["enforce_admins"])

		jsonResponse(w, http.StatusOK, githubProtectionResponse(true))
	})
	bp := newTestBP(t, mux)

	// Execute
	props, _ := json.Marshal(branchProtectionProperties{
		Owner:         "test-owner",
		Repo:          "test-repo",
		Branch:        "main",
		EnforceAdmins: true,
		RequiredStatusChecks: &statusChecksProps{
			Strict:   true,
			Contexts: []string{"ci/test"},
		},
		RequiredPullRequestReviews: &pullRequestReviewProps{
			RequiredApprovingReviewCount: 2,
			DismissStaleReviews:         true,
		},
	})
	result, err := bp.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo/main", result.ProgressResult.NativeID)

	var resultProps branchProtectionProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, true, resultProps.EnforceAdmins)
	assert.NotNil(t, resultProps.RequiredStatusChecks)
	assert.Equal(t, true, resultProps.RequiredStatusChecks.Strict)
	assert.Equal(t, []string{"ci/test"}, resultProps.RequiredStatusChecks.Contexts)
	assert.NotNil(t, resultProps.RequiredPullRequestReviews)
	assert.Equal(t, 2, resultProps.RequiredPullRequestReviews.RequiredApprovingReviewCount)
}

func TestBPRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, githubProtectionResponse(true))
	})
	bp := newTestBP(t, mux)

	// Execute
	result, err := bp.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/main",
		ResourceType: BranchProtectionResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props branchProtectionProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-owner", props.Owner)
	assert.Equal(t, "test-repo", props.Repo)
	assert.Equal(t, "main", props.Branch)
	assert.Equal(t, true, props.EnforceAdmins)
}

func TestBPReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	bp := newTestBP(t, mux)

	// Execute
	result, err := bp.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/main",
		ResourceType: BranchProtectionResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestBPUpdate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)
		resp := githubProtectionResponse(false)
		resp["enforce_admins"] = map[string]interface{}{"enabled": false}
		jsonResponse(w, http.StatusOK, resp)
	})
	bp := newTestBP(t, mux)

	// Execute
	props, _ := json.Marshal(branchProtectionProperties{
		Owner:         "test-owner",
		Repo:          "test-repo",
		Branch:        "main",
		EnforceAdmins: false,
	})
	result, err := bp.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-owner/test-repo/main",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	var resultProps branchProtectionProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, false, resultProps.EnforceAdmins)
}

func TestBPDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	bp := newTestBP(t, mux)

	// Execute
	result, err := bp.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/main",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestBPDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/branches/main/protection", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	bp := newTestBP(t, mux)

	// Execute
	result, err := bp.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/main",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}
