package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/types"
)

func intPtr(v int) *int { return &v }

func uuidEntityIDs(items []EntityOut) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestDeriveExpiryFields(t *testing.T) {
	production := types.DateFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	expiry := types.DateFromTime(time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))

	t.Run("production+shelf life derives expiry", func(t *testing.T) {
		_, shelfLife, gotExpiry := deriveExpiryFields(production, intPtr(30), types.Date{})
		require.NotNil(t, shelfLife)
		assert.Equal(t, 30, *shelfLife)
		assert.Equal(t, expiry, gotExpiry)
	})

	t.Run("production+expiry derives shelf life", func(t *testing.T) {
		_, shelfLife, gotExpiry := deriveExpiryFields(production, nil, expiry)
		require.NotNil(t, shelfLife)
		assert.Equal(t, 30, *shelfLife)
		assert.Equal(t, expiry, gotExpiry)
	})

	t.Run("both set keeps user values", func(t *testing.T) {
		other := types.DateFromTime(time.Date(2027, 6, 15, 0, 0, 0, 0, time.UTC))
		_, shelfLife, gotExpiry := deriveExpiryFields(production, intPtr(10), other)
		require.NotNil(t, shelfLife)
		assert.Equal(t, 10, *shelfLife)
		assert.Equal(t, other, gotExpiry)
	})

	t.Run("no production date derives nothing", func(t *testing.T) {
		// Values pass through untouched: shelf life is kept, no expiry is
		// derived from it.
		_, shelfLife, gotExpiry := deriveExpiryFields(types.Date{}, intPtr(30), types.Date{})
		require.NotNil(t, shelfLife)
		assert.Equal(t, 30, *shelfLife)
		assert.True(t, gotExpiry.Time().IsZero())

		// Shelf life is not derived from an expiry date either.
		_, shelfLife, gotExpiry = deriveExpiryFields(types.Date{}, nil, expiry)
		assert.Nil(t, shelfLife)
		assert.Equal(t, expiry, gotExpiry)
	})
}

func TestEntityRepository_ExpiryFields_RoundTrip(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	production := types.DateFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	expiry := types.DateFromTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))

	// Create with all three set explicitly → user values are kept.
	e, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:           "expiry-" + fk.Str(6),
		Quantity:       1,
		ProductionDate: production,
		ShelfLifeDays:  intPtr(7),
		ExpiryDate:     expiry,
	})
	require.NoError(t, err)

	assert.Equal(t, production, e.ProductionDate)
	require.NotNil(t, e.ShelfLifeDays)
	assert.Equal(t, 7, *e.ShelfLifeDays)
	assert.Equal(t, expiry, e.ExpiryDate)

	// Same values come back on read (also via the embedded summary).
	got, err := tRepos.Entities.GetOne(ctx, e.ID)
	require.NoError(t, err)
	assert.Equal(t, production, got.ProductionDate)
	require.NotNil(t, got.ShelfLifeDays)
	assert.Equal(t, 7, *got.ShelfLifeDays)
	assert.Equal(t, expiry, got.ExpiryDate)

	// Update with new explicit values → round-trips too.
	newExpiry := types.DateFromTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	updated, err := tRepos.Entities.UpdateByGroup(ctx, gid, EntityUpdate{
		ID:             e.ID,
		Name:           e.Name,
		Quantity:       1,
		ProductionDate: production,
		ShelfLifeDays:  intPtr(14),
		ExpiryDate:     newExpiry,
	})
	require.NoError(t, err)
	assert.Equal(t, production, updated.ProductionDate)
	require.NotNil(t, updated.ShelfLifeDays)
	assert.Equal(t, 14, *updated.ShelfLifeDays)
	assert.Equal(t, newExpiry, updated.ExpiryDate)
}

func TestEntityRepository_ExpiryFields_Optional(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	e, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:     "expiry-empty-" + fk.Str(6),
		Quantity: 1,
	})
	require.NoError(t, err)

	assert.True(t, e.ProductionDate.Time().IsZero())
	assert.Nil(t, e.ShelfLifeDays)
	assert.True(t, e.ExpiryDate.Time().IsZero())
}

func TestEntityRepository_ExpiryFields_ClearOnUpdate(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	e, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:           "expiry-clear-" + fk.Str(6),
		Quantity:       1,
		ProductionDate: types.DateFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		ShelfLifeDays:  intPtr(30),
	})
	require.NoError(t, err)
	require.NotNil(t, e.ShelfLifeDays)

	// Zero dates / nil shelf life clear the columns.
	updated, err := tRepos.Entities.UpdateByGroup(ctx, gid, EntityUpdate{
		ID:       e.ID,
		Name:     e.Name,
		Quantity: 1,
	})
	require.NoError(t, err)
	assert.True(t, updated.ProductionDate.Time().IsZero())
	assert.Nil(t, updated.ShelfLifeDays)
	assert.True(t, updated.ExpiryDate.Time().IsZero())
}

func TestEntityRepository_ExpiryFields_Derivation(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	production := types.DateFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// production + shelf life → expiry derived on create.
	e, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:           "expiry-derive-fwd-" + fk.Str(6),
		Quantity:       1,
		ProductionDate: production,
		ShelfLifeDays:  intPtr(30),
	})
	require.NoError(t, err)
	assert.Equal(t, types.DateFromTime(production.Time().AddDate(0, 0, 30)), e.ExpiryDate)

	// production + expiry → shelf life derived on update.
	expiry := types.DateFromTime(time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC))
	updated, err := tRepos.Entities.UpdateByGroup(ctx, gid, EntityUpdate{
		ID:             e.ID,
		Name:           e.Name,
		Quantity:       1,
		ProductionDate: production,
		ExpiryDate:     expiry,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.ShelfLifeDays)
	assert.Equal(t, int(expiry.Time().Sub(production.Time()).Hours()/24), *updated.ShelfLifeDays)
	assert.Equal(t, expiry, updated.ExpiryDate)

	// No production date → no derivation.
	e2, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:          "expiry-derive-none-" + fk.Str(6),
		Quantity:      1,
		ShelfLifeDays: intPtr(30),
	})
	require.NoError(t, err)
	assert.True(t, e2.ProductionDate.Time().IsZero())
	require.NotNil(t, e2.ShelfLifeDays)
	assert.Equal(t, 30, *e2.ShelfLifeDays)
	assert.True(t, e2.ExpiryDate.Time().IsZero())
}

func TestEntityRepository_ExpiryFields_Duplicate(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	production := types.DateFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	expiry := types.DateFromTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))

	e, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:           "expiry-dup-" + fk.Str(6),
		Quantity:       1,
		ProductionDate: production,
		ShelfLifeDays:  intPtr(7),
		ExpiryDate:     expiry,
	})
	require.NoError(t, err)

	dup, err := tRepos.Entities.Duplicate(ctx, gid, e.ID, DuplicateOptions{})
	require.NoError(t, err)

	assert.Equal(t, production, dup.ProductionDate)
	require.NotNil(t, dup.ShelfLifeDays)
	assert.Equal(t, 7, *dup.ShelfLifeDays)
	assert.Equal(t, expiry, dup.ExpiryDate)
}

func TestEntityRepository_Query_ExpiringWithinDays(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	soon, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:       "expiring-soon-" + fk.Str(6),
		Quantity:   1,
		ExpiryDate: types.DateFromTime(time.Now().AddDate(0, 0, 5)),
	})
	require.NoError(t, err)

	later, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:       "expiring-later-" + fk.Str(6),
		Quantity:   1,
		ExpiryDate: types.DateFromTime(time.Now().AddDate(0, 0, 60)),
	})
	require.NoError(t, err)

	noExpiry, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:     "expiring-never-" + fk.Str(6),
		Quantity: 1,
	})
	require.NoError(t, err)

	results, err := tRepos.Entities.QueryByGroup(ctx, gid, EntityQuery{
		Page:               1,
		PageSize:           100,
		ExpiringWithinDays: 30,
	})
	require.NoError(t, err)

	ids := make([]string, 0, len(results.Items))
	for _, item := range results.Items {
		ids = append(ids, item.ID.String())
	}
	assert.Contains(t, ids, soon.ID.String())
	assert.NotContains(t, ids, later.ID.String())
	assert.NotContains(t, ids, noExpiry.ID.String())
}

func TestEntityRepository_GetExpiringBetween(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	e, err := tRepos.Entities.Create(ctx, gid, EntityCreate{
		Name:       "expiring-between-" + fk.Str(6),
		Quantity:   1,
		ExpiryDate: types.DateFromTime(time.Now().AddDate(0, 0, 10)),
	})
	require.NoError(t, err)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Within 31-day window → found.
	items, err := tRepos.Entities.GetExpiringBetween(ctx, gid, today, today.AddDate(0, 0, 31))
	require.NoError(t, err)
	assert.Contains(t, uuidEntityIDs(items), e.ID)

	// Outside window → not found.
	items, err = tRepos.Entities.GetExpiringBetween(ctx, gid, today, today.AddDate(0, 0, 5))
	require.NoError(t, err)
	assert.NotContains(t, uuidEntityIDs(items), e.ID)
}
