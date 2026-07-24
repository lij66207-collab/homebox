package v1

import (
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/hay-kot/httpkit/errchain"

	"github.com/sysadminsmedia/homebox/backend/internal/core/services"
	"github.com/sysadminsmedia/homebox/backend/internal/data/ent"
	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
	"github.com/sysadminsmedia/homebox/backend/internal/sys/validate"
	"github.com/sysadminsmedia/homebox/backend/internal/web/adapters"
)

var timeOfDayRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// BackupScheduleUpdateRequest is the write payload for the group's backup
// schedule. It is handler-local (not repo.BackupScheduleUpsert) because the
// server computes next_run_at itself — clients never send it.
type BackupScheduleUpdateRequest struct {
	Enabled   bool   `json:"enabled"`
	Frequency string `json:"frequency" validate:"required,oneof=daily weekly"`
	TimeOfDay string `json:"timeOfDay" validate:"required"`
	Retention int    `json:"retention" validate:"required,min=1,max=100"`
}

// Validate enforces the HH:MM shape that the scheduler relies on when
// computing next_run_at. Runs after the struct-tag checks (see
// adapters/decoders.go).
func (b BackupScheduleUpdateRequest) Validate() error {
	if !timeOfDayRegex.MatchString(b.TimeOfDay) {
		return validate.NewRequestError(errors.New("timeOfDay must be in HH:MM format (00:00-23:59)"), http.StatusBadRequest)
	}
	return nil
}

// defaultBackupSchedule is returned when the group has never saved a
// schedule so the UI can render a sane form without special-casing 404.
func defaultBackupSchedule() repo.BackupScheduleOut {
	return repo.BackupScheduleOut{
		Enabled:   false,
		Frequency: "daily",
		TimeOfDay: "03:00",
		Retention: 7,
	}
}

// HandleBackupScheduleGet godoc
//
//	@Summary		Get the Backup Schedule
//	@Description	Returns the caller group's recurring backup schedule. Groups that never configured one get the default (disabled) schedule instead of a 404.
//	@Tags			Group
//	@Produce		json
//	@Success		200	{object}	repo.BackupScheduleOut
//	@Router			/v1/group/backup-schedule [GET]
//	@Security		Bearer
func (ctrl *V1Controller) HandleBackupScheduleGet() errchain.HandlerFunc {
	fn := func(r *http.Request) (repo.BackupScheduleOut, error) {
		ctx := services.NewContext(r.Context())
		out, err := ctrl.repo.BackupSchedules.GetByGroup(ctx, ctx.GID)
		if err != nil {
			if ent.IsNotFound(err) {
				return defaultBackupSchedule(), nil
			}
			return repo.BackupScheduleOut{}, err
		}
		return out, nil
	}

	return adapters.Command(fn, http.StatusOK)
}

// HandleBackupScheduleUpdate godoc
//
//	@Summary		Upsert the Backup Schedule
//	@Description	Creates or replaces the caller group's recurring backup schedule and recomputes next_run_at. Disabled schedules get next_run_at cleared.
//	@Tags			Group
//	@Produce		json
//	@Param			payload	body		BackupScheduleUpdateRequest	true	"Backup schedule"
//	@Success		200		{object}	repo.BackupScheduleOut
//	@Router			/v1/group/backup-schedule [PUT]
//	@Security		Bearer
func (ctrl *V1Controller) HandleBackupScheduleUpdate() errchain.HandlerFunc {
	fn := func(r *http.Request, body BackupScheduleUpdateRequest) (repo.BackupScheduleOut, error) {
		ctx := services.NewContext(r.Context())

		in := repo.BackupScheduleUpsert{
			Enabled:   body.Enabled,
			Frequency: body.Frequency,
			TimeOfDay: body.TimeOfDay,
			Retention: body.Retention,
		}
		if body.Enabled {
			next, err := services.NextRunAt(body.Frequency, body.TimeOfDay, time.Now())
			if err != nil {
				return repo.BackupScheduleOut{}, validate.NewRequestError(err, http.StatusBadRequest)
			}
			in.NextRunAt = &next
		}

		out, err := ctrl.repo.BackupSchedules.Upsert(ctx, ctx.GID, in)
		if err != nil {
			return repo.BackupScheduleOut{}, err
		}
		return out, nil
	}

	return adapters.Action(fn, http.StatusOK)
}
