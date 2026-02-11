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

func TestMembershipCreate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/alice", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"role":  "maintainer",
			"state": "active",
		})
	})
	m := newTestMembership(t, mux)

	// Execute
	props, _ := json.Marshal(membershipProperties{
		Organization: "test-org",
		TeamSlug:     "platform",
		Username:     "alice",
		Role:         "maintainer",
	})
	result, err := m.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "create handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
	assert.Equal(t, "test-org/platform/alice", result.ProgressResult.NativeID)

	var resultProps membershipProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "maintainer", resultProps.Role)
	assert.Equal(t, "active", resultProps.State)
}

func TestMembershipCreateDefaultRole(t *testing.T) {
	// Setup
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/bob", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		assert.Equal(t, "member", body["role"])
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"role":  "member",
			"state": "pending",
		})
	})
	m := newTestMembership(t, mux)

	// Execute — no role specified, should default to "member"
	props, _ := json.Marshal(membershipProperties{
		Organization: "test-org",
		TeamSlug:     "platform",
		Username:     "bob",
	})
	result, err := m.Create(context.Background(), &resource.CreateRequest{
		Properties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestMembershipRead(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/alice", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodGet, r.Method)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"role":  "maintainer",
			"state": "active",
		})
	})
	m := newTestMembership(t, mux)

	// Execute
	result, err := m.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-org/platform/alice",
		ResourceType: MembershipResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Empty(t, result.ErrorCode)

	var props membershipProperties
	require.NoError(t, json.Unmarshal([]byte(result.Properties), &props))
	assert.Equal(t, "test-org", props.Organization)
	assert.Equal(t, "platform", props.TeamSlug)
	assert.Equal(t, "alice", props.Username)
	assert.Equal(t, "maintainer", props.Role)
}

func TestMembershipReadNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/gone", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	m := newTestMembership(t, mux)

	// Execute
	result, err := m.Read(context.Background(), &resource.ReadRequest{
		NativeID:     "test-org/platform/gone",
		ResourceType: MembershipResourceType,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "read handler was not called")
	assert.Equal(t, resource.OperationErrorCodeNotFound, result.ErrorCode)
}

func TestMembershipUpdate(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/alice", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodPut, r.Method)
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"role":  "member",
			"state": "active",
		})
	})
	m := newTestMembership(t, mux)

	// Execute
	props, _ := json.Marshal(membershipProperties{
		Organization: "test-org",
		TeamSlug:     "platform",
		Username:     "alice",
		Role:         "member",
	})
	result, err := m.Update(context.Background(), &resource.UpdateRequest{
		NativeID:          "test-org/platform/alice",
		DesiredProperties: props,
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "update handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)

	var resultProps membershipProperties
	require.NoError(t, json.Unmarshal(result.ProgressResult.ResourceProperties, &resultProps))
	assert.Equal(t, "member", resultProps.Role)
}

func TestMembershipDelete(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/alice", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		assert.Equal(t, http.MethodDelete, r.Method)
		w.WriteHeader(http.StatusNoContent)
	})
	m := newTestMembership(t, mux)

	// Execute
	result, err := m.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-org/platform/alice",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}

func TestMembershipDeleteNotFound(t *testing.T) {
	// Setup
	var called atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/orgs/test-org/teams/platform/memberships/gone", func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		jsonResponse(w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
	})
	m := newTestMembership(t, mux)

	// Execute
	result, err := m.Delete(context.Background(), &resource.DeleteRequest{
		NativeID: "test-org/platform/gone",
	})

	// Verify
	require.NoError(t, err)
	assert.True(t, called.Load(), "delete handler was not called")
	assert.Equal(t, resource.OperationStatusSuccess, result.ProgressResult.OperationStatus)
}
