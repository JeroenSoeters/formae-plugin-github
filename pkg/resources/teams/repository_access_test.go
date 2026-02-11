//go:build integration

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoAccessCreate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/my-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	ra := newTestRepoAccess(t, mux)

	// Execute
	props, _ := json.Marshal(repoAccessProperties{
		Organization: "test-org",
		TeamSlug:     "platform",
		Repository:   "test-org/my-repo",
		Permission:   "push",
	})
	result, err := ra.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-org/platform/test-org/my-repo", result.ProgressResult.NativeID)

	var resultProps repoAccessProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "push", resultProps.Permission)
}

func TestRepoAccessCreateDefaultPermission(t *testing.T) {
	// Setup
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/my-repo", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "pull", body["permission"])
		w.WriteHeader(http.StatusNoContent)
	})
	ra := newTestRepoAccess(t, mux)

	// Execute — no permission specified, should default to "pull"
	props, _ := json.Marshal(repoAccessProperties{
		Organization: "test-org",
		TeamSlug:     "platform",
		Repository:   "test-org/my-repo",
	})
	result, err := ra.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestRepoAccessRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/my-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"full_name": "test-org/my-repo",
			"permissions": map[string]interface{}{
				"admin":    false,
				"maintain": false,
				"push":     true,
				"triage":   true,
				"pull":     true,
			},
		})
	})
	ra := newTestRepoAccess(t, mux)

	// Execute
	result, err := ra.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-org/platform/test-org/my-repo",
		ResourceType: RepositoryAccessResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props repoAccessProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-org", props.Organization)
	assert.Equal(t, "platform", props.TeamSlug)
	assert.Equal(t, "test-org/my-repo", props.Repository)
	assert.Equal(t, "push", props.Permission)
}

func TestRepoAccessReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/gone", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	ra := newTestRepoAccess(t, mux)

	// Execute
	result, err := ra.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-org/platform/test-org/gone",
		ResourceType: RepositoryAccessResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestRepoAccessUpdate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/my-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	ra := newTestRepoAccess(t, mux)

	// Execute
	props, _ := json.Marshal(repoAccessProperties{
		Organization: "test-org",
		TeamSlug:     "platform",
		Repository:   "test-org/my-repo",
		Permission:   "admin",
	})
	result, err := ra.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-org/platform/test-org/my-repo",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	var resultProps repoAccessProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "admin", resultProps.Permission)
}

func TestRepoAccessDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/my-repo", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	ra := newTestRepoAccess(t, mux)

	// Execute
	result, err := ra.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-org/platform/test-org/my-repo",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestRepoAccessDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/repos/test-org/gone", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	ra := newTestRepoAccess(t, mux)

	// Execute
	result, err := ra.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-org/platform/test-org/gone",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}
