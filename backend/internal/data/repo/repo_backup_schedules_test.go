package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
)

const (
	freqDaily = "daily"
	timeOfDay = "03:00"
)

func TestBackupSchedules_GetByGroup_NotFound(t *testing.T) {
	_, err := tRepos.BackupSchedules.GetByGroup(context.Background(), uuid.New())
	require.Error(t, err)
	assert.True(t, ent.IsNotFound(err), "expected ent.NotFoundError, got %v", err)
}

func TestBackupSchedules_Upsert_CreateThenUpdate(t *testing.T) {
	ctx := context.Background()
	gid := tGroup.ID

	next := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	created, err := tRepos.BackupSchedules.Upsert(ctx, gid, BackupScheduleUpsert{
		Enabled:   true,
		Frequency: freqDaily,
		TimeOfDay: timeOfDay,
		Retention: 7,
		NextRunAt: &next,
	})
	require.NoError(t, err)
	assert.True(t, created.Enabled)
	assert.Equal(t, freqDaily, created.Frequency)
	assert.Equal(t, timeOfDay, created.TimeOfDay)
	assert.Equal(t, 7, created.Retention)
	require.NotNil(t, created.NextRunAt)
	assert.True(t, created.NextRunAt.Equal(next))

	// Second upsert must update the same row (one schedule per group).
	updated, err := tRepos.BackupSchedules.Upsert(ctx, gid, BackupScheduleUpsert{
		Enabled:   false,
		Frequency: "weekly",
		TimeOfDay: "22:30",
		Retention: 3,
		NextRunAt: nil,
	})
	require.NoError(t, err)
	assert.Equal(t, created.ID, updated.ID, "upsert must not create a second row")
	assert.False(t, updated.Enabled)
	assert.Equal(t, "weekly", updated.Frequency)
	assert.Equal(t, "22:30", updated.TimeOfDay)
	assert.Equal(t, 3, updated.Retention)
	assert.Nil(t, updated.NextRunAt, "disabling must clear next_run_at")

	fetched, err := tRepos.BackupSchedules.GetByGroup(ctx, gid)
	require.NoError(t, err)
	assert.Equal(t, updated.ID, fetched.ID)
	assert.Equal(t, "weekly", fetched.Frequency)
}

func TestBackupSchedules_ListDue(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	dueGroup, err := tRepos.Groups.GroupCreate(ctx, "due-group", uuid.Nil)
	require.NoError(t, err)
	notDueGroup, err := tRepos.Groups.GroupCreate(ctx, "not-due-group", uuid.Nil)
	require.NoError(t, err)
	disabledGroup, err := tRepos.Groups.GroupCreate(ctx, "disabled-group", uuid.Nil)
	require.NoError(t, err)

	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	_, err = tRepos.BackupSchedules.Upsert(ctx, dueGroup.ID, BackupScheduleUpsert{
		Enabled: true, Frequency: freqDaily, TimeOfDay: timeOfDay, Retention: 7, NextRunAt: &past,
	})
	require.NoError(t, err)
	_, err = tRepos.BackupSchedules.Upsert(ctx, notDueGroup.ID, BackupScheduleUpsert{
		Enabled: true, Frequency: freqDaily, TimeOfDay: timeOfDay, Retention: 7, NextRunAt: &future,
	})
	require.NoError(t, err)
	_, err = tRepos.BackupSchedules.Upsert(ctx, disabledGroup.ID, BackupScheduleUpsert{
		Enabled: false, Frequency: freqDaily, TimeOfDay: timeOfDay, Retention: 7, NextRunAt: nil,
	})
	require.NoError(t, err)

	due, err := tRepos.BackupSchedules.ListDue(ctx, now)
	require.NoError(t, err)

	ids := make([]uuid.UUID, len(due))
	for i, s := range due {
		ids[i] = s.GroupID
	}
	assert.Contains(t, ids, dueGroup.ID)
	assert.NotContains(t, ids, notDueGroup.ID, "future next_run_at must not be due")
	assert.NotContains(t, ids, disabledGroup.ID, "disabled schedules must never be due")
}

func TestBackupSchedules_UpdateAfterRun(t *testing.T) {
	ctx := context.Background()

	grp, err := tRepos.Groups.GroupCreate(ctx, "after-run-group", uuid.Nil)
	require.NoError(t, err)

	sched, err := tRepos.BackupSchedules.Upsert(ctx, grp.ID, BackupScheduleUpsert{
		Enabled: true, Frequency: freqDaily, TimeOfDay: timeOfDay, Retention: 7,
	})
	require.NoError(t, err)

	last := time.Now().Truncate(time.Second)
	next := last.Add(24 * time.Hour)
	require.NoError(t, tRepos.BackupSchedules.UpdateAfterRun(ctx, sched.ID, last, next))

	fetched, err := tRepos.BackupSchedules.GetByGroup(ctx, grp.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched.LastRunAt)
	require.NotNil(t, fetched.NextRunAt)
	assert.True(t, fetched.LastRunAt.Equal(last))
	assert.True(t, fetched.NextRunAt.Equal(next))

	// A later upsert must not clobber last_run_at.
	_, err = tRepos.BackupSchedules.Upsert(ctx, grp.ID, BackupScheduleUpsert{
		Enabled: true, Frequency: "weekly", TimeOfDay: "04:00", Retention: 5, NextRunAt: &next,
	})
	require.NoError(t, err)
	fetched, err = tRepos.BackupSchedules.GetByGroup(ctx, grp.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched.LastRunAt)
	assert.True(t, fetched.LastRunAt.Equal(last), "upsert must preserve last_run_at")
}

func TestExports_ListScheduledCompleted(t *testing.T) {
	ctx := context.Background()

	grp, err := tRepos.Groups.GroupCreate(ctx, "scheduled-exports-group", uuid.Nil)
	require.NoError(t, err)

	// Manual export — must be excluded.
	manual, err := tRepos.Exports.Create(ctx, grp.ID)
	require.NoError(t, err)
	require.NoError(t, tRepos.Exports.SetCompleted(ctx, grp.ID, manual.ID, "a.zip", 1))

	// Two scheduled exports; only the completed one may be listed.
	sched1, err := tRepos.Exports.CreateScheduled(ctx, grp.ID)
	require.NoError(t, err)
	assert.Equal(t, "scheduled", sched1.Trigger)
	require.NoError(t, tRepos.Exports.SetCompleted(ctx, grp.ID, sched1.ID, "b.zip", 1))

	_, err = tRepos.Exports.CreateScheduled(ctx, grp.ID)
	require.NoError(t, err) // stays pending

	got, err := tRepos.Exports.ListScheduledCompleted(ctx, grp.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, sched1.ID, got[0].ID)
	assert.Equal(t, "scheduled", got[0].Trigger)
}
