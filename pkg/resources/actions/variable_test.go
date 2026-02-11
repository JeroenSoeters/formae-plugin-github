//go:build integration

package actions

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

func TestVariableCreate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/variables", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "MY_VAR", body["name"])
		assert.Equal(t, "hello", body["value"])

		w.WriteHeader(http.StatusCreated)
	})
	v := newTestVariable(t, mux)

	// Execute
	props, _ := json.Marshal(variableProperties{
		Owner: "test-owner",
		Repo:  "test-repo",
		Name:  "MY_VAR",
		Value: "hello",
	})
	result, err := v.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo/MY_VAR", result.ProgressResult.NativeID)

	var resultProps variableProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "MY_VAR", resultProps.Name)
	assert.Equal(t, "hello", resultProps.Value)
}

func TestVariableRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/variables/MY_VAR", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"name":       "MY_VAR",
			"value":      "hello",
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-06-15T12:00:00Z",
		})
	})
	v := newTestVariable(t, mux)

	// Execute
	result, err := v.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/MY_VAR",
		ResourceType: VariableResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props variableProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-owner", props.Owner)
	assert.Equal(t, "test-repo", props.Repo)
	assert.Equal(t, "MY_VAR", props.Name)
	assert.Equal(t, "hello", props.Value)
	assert.NotEmpty(t, props.CreatedAt)
	assert.NotEmpty(t, props.UpdatedAt)
}

func TestVariableReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/variables/GONE", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	v := newTestVariable(t, mux)

	// Execute
	result, err := v.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/GONE",
		ResourceType: VariableResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestVariableUpdate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/variables/MY_VAR", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPatch, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "world", body["value"])

		w.WriteHeader(http.StatusNoContent)
	})
	v := newTestVariable(t, mux)

	// Execute
	props, _ := json.Marshal(variableProperties{
		Owner: "test-owner",
		Repo:  "test-repo",
		Name:  "MY_VAR",
		Value: "world",
	})
	result, err := v.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-owner/test-repo/MY_VAR",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	var resultProps variableProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "world", resultProps.Value)
}

func TestVariableDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/variables/MY_VAR", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	v := newTestVariable(t, mux)

	// Execute
	result, err := v.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/MY_VAR",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestVariableDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/variables/GONE", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	v := newTestVariable(t, mux)

	// Execute
	result, err := v.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/GONE",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}
