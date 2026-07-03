//go:build integration

package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func workflowRunResponse(id int64, status, conclusion, branch, workflow string) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"status":      status,
		"conclusion":  conclusion,
		"html_url":    fmt.Sprintf("https://github.com/test-owner/test-repo/actions/runs/%d", id),
		"head_branch": branch,
		"path":        workflow,
		"event":       "workflow_dispatch",
		"created_at":  time.Now().UTC().Format(time.RFC3339),
	}
}

func TestWorkflowRunCreate(t *testing.T) {
	// Setup: no existing successful run → dispatch → return InProgress without
	// blocking. The run is discovered later by Status, so Create must not poll.
	var dispatched atomic.Bool
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/workflows/deploy.yml/runs", func(w http.ResponseWriter, r *http.Request) {
		// Idempotency check only: no existing successful run.
		assert.Equal(t, "success", r.URL.Query().Get("status"))
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count":   0,
			"workflow_runs": []interface{}{},
		})
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/workflows/deploy.yml/dispatches", func(w http.ResponseWriter, r *http.Request) {
		dispatched.Store(true)
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	props, _ := json.Marshal(workflowRunProperties{
		Repository: "test-owner/test-repo",
		Workflow:   "deploy.yml",
		Ref:        "main",
	})
	result, err := wr.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify: dispatched, InProgress, and the ID is a dispatch token (no run yet).
	require.NoError(t, err)
	assert.True(t, dispatched.Load(), "dispatch handler was not called")
	assert.Equal(t, resource.OperationStatusInProgress, result.ProgressResult.OperationStatus)
	assert.Equal(t, result.ProgressResult.NativeID, result.ProgressResult.RequestID)

	_, _, workflow, ref, _, ok := parseDispatchToken(result.ProgressResult.NativeID)
	require.True(t, ok, "expected a dispatch token native ID, got %q", result.ProgressResult.NativeID)
	assert.Equal(t, "deploy.yml", workflow)
	assert.Equal(t, "main", ref)
}

func TestWorkflowRunStatusDiscoversDispatchedRun(t *testing.T) {
	// A dispatch token resolves to the concrete run via a single list call,
	// then reports that run's status.
	var listed atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/workflows/deploy.yml/runs", func(w http.ResponseWriter, r *http.Request) {
		listed.Store(true)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count": 1,
			"workflow_runs": []interface{}{
				workflowRunResponse(12345, "in_progress", "", "main", ".github/workflows/deploy.yml"),
			},
		})
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, workflowRunResponse(12345, "in_progress", "", "main", ".github/workflows/deploy.yml"))
	})
	wr := newTestWorkflowRun(t, mux)

	token := dispatchToken("test-owner", "test-repo", "deploy.yml", "main", time.Now().Add(-2*time.Second))
	result, err := wr.Status(context.Background(), &resource.StatusRequest{RequestID: token})

	require.NoError(t, err)
	assert.True(t, listed.Load(), "list handler was not called")
	assert.Equal(t, resource.OperationStatusInProgress, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo/12345", result.ProgressResult.NativeID)
}

func TestWorkflowRunStatusPendingDispatch(t *testing.T) {
	// No run has appeared yet → stay InProgress with the same dispatch token.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/workflows/deploy.yml/runs", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count":   0,
			"workflow_runs": []interface{}{},
		})
	})
	wr := newTestWorkflowRun(t, mux)

	token := dispatchToken("test-owner", "test-repo", "deploy.yml", "main", time.Now())
	result, err := wr.Status(context.Background(), &resource.StatusRequest{RequestID: token})

	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusInProgress, result.ProgressResult.OperationStatus)
	assert.Equal(t, token, result.ProgressResult.RequestID)
}

func TestWorkflowRunCreateIdempotent(t *testing.T) {
	// Setup: existing successful run found → return it without dispatching
	var dispatched atomic.Bool
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/workflows/deploy.yml/runs", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count": 1,
			"workflow_runs": []interface{}{
				workflowRunResponse(99999, "completed", "success", "main", ".github/workflows/deploy.yml"),
			},
		})
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/workflows/deploy.yml/dispatches", func(w http.ResponseWriter, r *http.Request) {
		dispatched.Store(true)
		t.Fatal("dispatch should not be called for idempotent create")
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/99999/artifacts", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count": 0,
			"artifacts":   []interface{}{},
		})
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	props, _ := json.Marshal(workflowRunProperties{
		Repository: "test-owner/test-repo",
		Workflow:   "deploy.yml",
		Ref:        "main",
	})
	result, err := wr.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.False(t, dispatched.Load(), "dispatch should not be called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo/99999", result.ProgressResult.NativeID)

	var resultProps workflowRunProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "success", resultProps.Conclusion)
}

func TestWorkflowRunRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, workflowRunResponse(12345, "completed", "success", "main", ".github/workflows/deploy.yml"))
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345/artifacts", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count": 1,
			"artifacts": []interface{}{
				map[string]interface{}{
					"id":                   42,
					"name":                 "build-output",
					"archive_download_url": "https://api.github.com/download/42",
					"size_in_bytes":        2048,
				},
			},
		})
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/12345",
		ResourceType: WorkflowRunResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props workflowRunProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-owner/test-repo", props.Repository)
	assert.Equal(t, int64(12345), props.RunID)
	assert.Equal(t, "completed", props.Status)
	assert.Equal(t, "success", props.Conclusion)
	assert.Equal(t, int64(42), props.ArtifactID)
	assert.Equal(t, int64(2048), props.ArtifactSizeInBytes)
}

func TestWorkflowRunReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/99999", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/99999",
		ResourceType: WorkflowRunResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestWorkflowRunStatusInProgress(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusOK, workflowRunResponse(12345, "in_progress", "", "main", ".github/workflows/deploy.yml"))
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Status(context.Background(), &resource.StatusRequest{
		RequestID: "test-owner/test-repo/12345",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "status handler was not called")
	assert.Equal(t, resource.OperationStatusInProgress, result.ProgressResult.OperationStatus)
	assert.Contains(t, result.ProgressResult.StatusMessage, "in_progress")
}

func TestWorkflowRunStatusCompleted(t *testing.T) {
	// Setup
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, workflowRunResponse(12345, "completed", "success", "main", ".github/workflows/deploy.yml"))
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345/artifacts", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count": 1,
			"artifacts": []interface{}{
				map[string]interface{}{
					"id":                   42,
					"name":                 "build-output",
					"archive_download_url": "https://api.github.com/download/42",
					"size_in_bytes":        1024,
				},
			},
		})
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Status(context.Background(), &resource.StatusRequest{
		RequestID: "test-owner/test-repo/12345",
	})

	// Verify
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	var props workflowRunProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &props))
	assert.Equal(t, int64(42), props.ArtifactID)
	assert.Equal(t, int64(1024), props.ArtifactSizeInBytes)
}

func TestWorkflowRunStatusFailed(t *testing.T) {
	// Setup
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, workflowRunResponse(12345, "completed", "failure", "main", ".github/workflows/deploy.yml"))
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345/artifacts", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"total_count": 0,
			"artifacts":   []interface{}{},
		})
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Status(context.Background(), &resource.StatusRequest{
		RequestID: "test-owner/test-repo/12345",
	})

	// Verify
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusFailure, result.ProgressResult.OperationStatus)
	assert.Contains(t, result.ProgressResult.StatusMessage, "failure")
}

func TestWorkflowRunDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/12345", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/12345",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestWorkflowRunDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/runs/99999", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	wr := newTestWorkflowRun(t, mux)

	// Execute
	result, err := wr.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/99999",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}
