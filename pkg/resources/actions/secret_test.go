//go:build integration

package actions

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
)

// testPublicKey generates a valid NaCl public key for mocking the GitHub secrets API.
func testPublicKey(t *testing.T) string {
	t.Helper()
	pub, _, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(pub[:])
}

func TestSecretCreate(t *testing.T) {
	// Setup
	pubKey := testPublicKey(t)
	var secretCreated atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/public-key", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"key_id": "test-key-id",
			"key":    pubKey,
		})
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/MY_SECRET", func(w http.ResponseWriter, r *http.Request) {
		secretCreated.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "test-key-id", body["key_id"])
		assert.NotEmpty(t, body["encrypted_value"])

		w.WriteHeader(http.StatusCreated)
	})
	s := newTestSecret(t, mux)

	// Execute
	props, _ := json.Marshal(secretProperties{
		Owner: "test-owner",
		Repo:  "test-repo",
		Name:  "MY_SECRET",
		Value: "super-secret-value",
	})
	result, err := s.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, secretCreated.Load(), "secret create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-owner/test-repo/MY_SECRET", result.ProgressResult.NativeID)

	// Value should NOT appear in result properties (write-only)
	var resultProps secretProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "MY_SECRET", resultProps.Name)
	assert.Empty(t, resultProps.Value)
}

func TestSecretRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/MY_SECRET", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"name":       "MY_SECRET",
			"created_at": "2024-01-01T00:00:00Z",
			"updated_at": "2024-06-15T12:00:00Z",
		})
	})
	s := newTestSecret(t, mux)

	// Execute
	result, err := s.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/MY_SECRET",
		ResourceType: SecretResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props secretProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-owner", props.Owner)
	assert.Equal(t, "test-repo", props.Repo)
	assert.Equal(t, "MY_SECRET", props.Name)
	assert.Empty(t, props.Value, "secret value should not be returned on read")
	assert.NotEmpty(t, props.CreatedAt)
}

func TestSecretReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/GONE", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	s := newTestSecret(t, mux)

	// Execute
	result, err := s.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-owner/test-repo/GONE",
		ResourceType: SecretResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestSecretUpdate(t *testing.T) {
	// Setup
	pubKey := testPublicKey(t)
	var secretUpdated atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/public-key", func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"key_id": "test-key-id",
			"key":    pubKey,
		})
	})
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/MY_SECRET", func(w http.ResponseWriter, r *http.Request) {
		secretUpdated.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	s := newTestSecret(t, mux)

	// Execute
	props, _ := json.Marshal(secretProperties{
		Owner: "test-owner",
		Repo:  "test-repo",
		Name:  "MY_SECRET",
		Value: "new-secret-value",
	})
	result, err := s.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-owner/test-repo/MY_SECRET",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, secretUpdated.Load(), "secret update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestSecretDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/MY_SECRET", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	s := newTestSecret(t, mux)

	// Execute
	result, err := s.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/MY_SECRET",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestSecretDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/test-owner/test-repo/actions/secrets/GONE", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	s := newTestSecret(t, mux)

	// Execute
	result, err := s.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-owner/test-repo/GONE",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}
