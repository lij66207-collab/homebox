package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/backupschedule"
)

// BackupScheduleRepository persists the per-group recurring export schedule.
// Every group has at most one row (unique group_id), so the API surface is
// get/upsert rather than list/create.
type BackupScheduleRepository struct {
	db *ent.Client
}

type BackupScheduleOut struct {
	ID        uuid.UUID  `json:"id"`
	GroupID   uuid.UUID  `json:"groupId"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Enabled   bool       `json:"enabled"`
	Frequency string     `json:"frequency"`
	TimeOfDay string     `json:"timeOfDay"`
	Retention int        `json:"retention"`
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
}

// BackupScheduleUpsert is the write model for the schedule. NextRunAt is
// computed by the caller (service/handler) so the repo stays free of
// clock/timezone logic.
type BackupScheduleUpsert struct {
	Enabled   bool       `json:"enabled"`
	Frequency string     `json:"frequency"           validate:"required,oneof=daily weekly"`
	TimeOfDay string     `json:"timeOfDay"           validate:"required"`
	Retention int        `json:"retention"           validate:"required,min=1,max=100"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
}

func mapBackupSchedule(e *ent.BackupSchedule) BackupScheduleOut {
	return BackupScheduleOut{
		ID:        e.ID,
		GroupID:   e.GroupID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		Enabled:   e.Enabled,
		Frequency: string(e.Frequency),
		TimeOfDay: e.TimeOfDay,
		Retention: e.Retention,
		LastRunAt: e.LastRunAt,
		NextRunAt: e.NextRunAt,
	}
}

// GetByGroup returns the group's schedule, or an ent.NotFoundError when the
// group has never configured one.
func (r *BackupScheduleRepository) GetByGroup(ctx context.Context, gid uuid.UUID) (BackupScheduleOut, error) {
	e, err := r.db.BackupSchedule.Query().
		Where(backupschedule.GroupID(gid)).
		Only(ctx)
	if err != nil {
		return BackupScheduleOut{}, err
	}
	return mapBackupSchedule(e), nil
}

// Upsert creates the group's schedule on first save and replaces it
// afterwards. The unique group_id index guarantees at most one row per
// group; last_run_at is preserved across updates.
func (r *BackupScheduleRepository) Upsert(ctx context.Context, gid uuid.UUID, in BackupScheduleUpsert) (BackupScheduleOut, error) {
	existing, err := r.db.BackupSchedule.Query().
		Where(backupschedule.GroupID(gid)).
		Only(ctx)
	switch {
	case err == nil:
		upd := r.db.BackupSchedule.UpdateOneID(existing.ID).
			SetEnabled(in.Enabled).
			SetFrequency(backupschedule.Frequency(in.Frequency)).
			SetTimeOfDay(in.TimeOfDay).
			SetRetention(in.Retention)
		if in.NextRunAt != nil {
			upd.SetNextRunAt(*in.NextRunAt)
		} else {
			upd.ClearNextRunAt()
		}
		e, err := upd.Save(ctx)
		if err != nil {
			return BackupScheduleOut{}, err
		}
		return mapBackupSchedule(e), nil
	case ent.IsNotFound(err):
		create := r.db.BackupSchedule.Create().
			SetGroupID(gid).
			SetEnabled(in.Enabled).
			SetFrequency(backupschedule.Frequency(in.Frequency)).
			SetTimeOfDay(in.TimeOfDay).
			SetRetention(in.Retention)
		if in.NextRunAt != nil {
			create.SetNextRunAt(*in.NextRunAt)
		}
		e, err := create.Save(ctx)
		if err != nil {
			return BackupScheduleOut{}, err
		}
		return mapBackupSchedule(e), nil
	default:
		return BackupScheduleOut{}, err
	}
}

// ListDue returns every enabled schedule whose next_run_at has arrived. Not
// scoped to a group on purpose: the scheduler sweeps all tenants.
func (r *BackupScheduleRepository) ListDue(ctx context.Context, now time.Time) ([]BackupScheduleOut, error) {
	rows, err := r.db.BackupSchedule.Query().
		Where(
			backupschedule.Enabled(true),
			backupschedule.NextRunAtNotNil(),
			backupschedule.NextRunAtLTE(now),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BackupScheduleOut, len(rows))
	for i, e := range rows {
		out[i] = mapBackupSchedule(e)
	}
	return out, nil
}

// UpdateAfterRun stamps the run timestamps after the scheduler fires.
func (r *BackupScheduleRepository) UpdateAfterRun(ctx context.Context, id uuid.UUID, lastRun, nextRun time.Time) error {
	return r.db.BackupSchedule.UpdateOneID(id).
		SetLastRunAt(lastRun).
		SetNextRunAt(nextRun).
		Exec(ctx)
}
