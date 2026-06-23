package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	spec := &AgentSpec{
		Name:        "test-agent",
		Description: "A test agent",
	}

	version, err := store.Create(ctx, "tenant1", spec)
	require.NoError(t, err)
	assert.True(t, len(version) > 0)

	_, err = store.Get(ctx, "tenant1", "test-agent")
	require.Error(t, err)

	err = store.Activate(ctx, "tenant1", "test-agent", version)
	require.NoError(t, err)

	got, err := store.Get(ctx, "tenant1", "test-agent")
	require.NoError(t, err)
	assert.Equal(t, spec.Name, got.Name)
}

func TestMemoryStore_CreateIdempotence(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	spec := &AgentSpec{Name: "idempotent-test"}

	v1, err := store.Create(ctx, "tenant1", spec)
	require.NoError(t, err)

	v2, err := store.Create(ctx, "tenant1", spec)
	require.NoError(t, err)

	assert.Equal(t, v1, v2)
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	spec := &AgentSpec{Name: "delete-test"}

	version, err := store.Create(ctx, "tenant1", spec)
	require.NoError(t, err)

	err = store.Activate(ctx, "tenant1", "delete-test", version)
	require.NoError(t, err)

	err = store.Delete(ctx, "tenant1", "delete-test")
	require.NoError(t, err)

	_, err = store.Get(ctx, "tenant1", "delete-test")
	require.Error(t, err)

	names, err := store.List(ctx, "tenant1")
	require.NoError(t, err)
	assert.Equal(t, 0, len(names))
}

func TestMemoryStore_TenantIsolation(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	spec := &AgentSpec{Name: "shared"}

	v1, err := store.Create(ctx, "tenant1", spec)
	require.NoError(t, err)

	v2, err := store.Create(ctx, "tenant2", spec)
	require.NoError(t, err)

	assert.Equal(t, v1, v2)

	err = store.Activate(ctx, "tenant1", "shared", v1)
	require.NoError(t, err)

	got, err := store.Get(ctx, "tenant1", "shared")
	require.NoError(t, err)
	assert.Equal(t, spec.Name, got.Name)

	_, err = store.Get(ctx, "tenant2", "shared")
	require.Error(t, err)
}

func TestMemoryStore_ListVersions(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	spec1 := &AgentSpec{Name: "list-test", Description: "v1"}
	spec2 := &AgentSpec{Name: "list-test", Description: "v2"}

	v1, err := store.Create(ctx, "tenant1", spec1)
	require.NoError(t, err)

	v2, err := store.Create(ctx, "tenant1", spec2)
	require.NoError(t, err)

	versions, err := store.ListVersions(ctx, "tenant1", "list-test")
	require.NoError(t, err)
	require.Equal(t, 2, len(versions))

	assert.Equal(t, v2, versions[0].Version)
	assert.Equal(t, v1, versions[1].Version)
}
