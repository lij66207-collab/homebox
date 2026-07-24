package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

func TestLowStockReminder_Lifecycle(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	threshold := 3.0
	e, err := tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
		Name:     "low-stock-" + fk.Str(6),
		Quantity: 5,
	})
	require.NoError(t, err)

	// Set a threshold; quantity still above it → not low stock.
	_, err = tRepos.Entities.UpdateByGroup(ctx, gid, repo.EntityUpdate{
		ID:                e.ID,
		Name:              e.Name,
		Quantity:          5,
		LowStockThreshold: &threshold,
	})
	require.NoError(t, err)

	low, err := tRepos.Entities.GetLowStock(ctx, gid)
	require.NoError(t, err)
	assert.NotContains(t, uuidSlice(low), e.ID)

	// Drop below the threshold → shows up.
	_, err = tRepos.Entities.UpdateByGroup(ctx, gid, repo.EntityUpdate{
		ID:                e.ID,
		Name:              e.Name,
		Quantity:          2,
		LowStockThreshold: &threshold,
	})
	require.NoError(t, err)

	low, err = tRepos.Entities.GetLowStock(ctx, gid)
	require.NoError(t, err)
	assert.Contains(t, uuidSlice(low), e.ID)

	// Latch after notification → hidden again.
	require.NoError(t, tRepos.Entities.MarkLowStockNotified(ctx, gid, []uuid.UUID{e.ID}))
	low, err = tRepos.Entities.GetLowStock(ctx, gid)
	require.NoError(t, err)
	assert.NotContains(t, uuidSlice(low), e.ID)

	// Restock above the threshold → latch resets.
	require.NoError(t, tRepos.Entities.Patch(ctx, gid, e.ID, repo.EntityPatch{
		ID:       e.ID,
		Quantity: ptr(10.0),
	}))
	// Drop below again → reported again (latch was reset by the restock).
	require.NoError(t, tRepos.Entities.Patch(ctx, gid, e.ID, repo.EntityPatch{
		ID:       e.ID,
		Quantity: ptr(1.0),
	}))
	low, err = tRepos.Entities.GetLowStock(ctx, gid)
	require.NoError(t, err)
	assert.Contains(t, uuidSlice(low), e.ID)
}

func uuidSlice(items []repo.EntityOut) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func ptr(v float64) *float64 { return &v }

func TestWarrantyExpiringBetween(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	expires := types.DateFromTime(time.Now().AddDate(0, 0, 10))
	e, err := tRepos.Entities.Create(ctx, gid, repo.EntityCreate{
		Name:     "warranty-" + fk.Str(6),
		Quantity: 1,
	})
	require.NoError(t, err)

	_, err = tRepos.Entities.UpdateByGroup(ctx, gid, repo.EntityUpdate{
		ID:              e.ID,
		Name:            e.Name,
		Quantity:        1,
		WarrantyExpires: expires,
	})
	require.NoError(t, err)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Within 31-day window → found.
	items, err := tRepos.Entities.GetWarrantyExpiringBetween(ctx, gid, today, today.AddDate(0, 0, 31))
	require.NoError(t, err)
	assert.Contains(t, uuidSlice(items), e.ID)

	// Outside window → not found.
	items, err = tRepos.Entities.GetWarrantyExpiringBetween(ctx, gid, today, today.AddDate(0, 0, 5))
	require.NoError(t, err)
	assert.NotContains(t, uuidSlice(items), e.ID)
}
