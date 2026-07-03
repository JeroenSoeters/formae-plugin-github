//go:build integration

package teams

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/platform-engineering-labs/formae-plugin-github/pkg/config"
	"github.com/platform-engineering-labs/formae/pkg/plugin/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamCreate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPost, r.Method)

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "platform", body["name"])

		jsonResponse(w, http.StatusCreated, githubTeamResponse(42, "platform", "platform", "test-org"))
	})
	team := newTestTeam(t, mux)

	// Execute
	props, _ := json.Marshal(teamProperties{
		Organization: "test-org",
		Name:         "platform",
		Description:  "A test team",
		Privacy:      "closed",
	})
	result, err := team.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-org/platform", result.ProgressResult.NativeID)

	var resultProps teamProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "platform", resultProps.Name)
	assert.Equal(t, "platform", resultProps.Slug)
	assert.Equal(t, "test-org", resultProps.Organization)
	assert.Equal(t, int64(42), resultProps.ID)
}

func TestTeamRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, githubTeamResponse(42, "platform", "platform", "test-org"))
	})
	team := newTestTeam(t, mux)

	// Execute
	result, err := team.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-org/platform",
		ResourceType: TeamResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props teamProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "platform", props.Name)
	assert.Equal(t, "test-org", props.Organization)
	assert.Equal(t, int64(42), props.ID)
}

func TestTeamReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/missing", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	team := newTestTeam(t, mux)

	// Execute
	result, err := team.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-org/missing",
		ResourceType: TeamResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestTeamUpdate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPatch, r.Method)
		jsonResponse(w, http.StatusOK, githubTeamResponse(42, "platform-eng", "platform-eng", "test-org"))
	})
	team := newTestTeam(t, mux)

	// Execute
	props, _ := json.Marshal(teamProperties{
		Organization: "test-org",
		Name:         "platform-eng",
		Slug:         "platform",
		Privacy:      "closed",
	})
	result, err := team.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-org/platform",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-org/platform-eng", result.ProgressResult.NativeID)

	var resultProps teamProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "platform-eng", resultProps.Name)
}

func TestTeamDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	team := newTestTeam(t, mux)

	// Execute
	result, err := team.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-org/platform",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestTeamDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/missing", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	team := newTestTeam(t, mux)

	// Execute
	result, err := team.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-org/missing",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestTeamList(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		teams := []interface{}{
			githubTeamResponse(10, "alpha", "alpha", "test-org"),
			githubTeamResponse(20, "beta", "beta", "test-org"),
		}
		jsonResponse(w, http.StatusOK, teams)
	})
	team := newTestTeam(t, mux)

	// Execute
	targetCfg, _ := json.Marshal(config.Config{Organization: "test-org"})
	result, err := team.List(context.Background(), &resource.ListRequest{
		TargetConfig: targetCfg,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "list handler was not called")
	assert.Equal(t, []string{"test-org/alpha", "test-org/beta"}, result.NativeIDs)
	assert.Nil(t, result.NextPageToken)
}
