package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/sysadminsmedia/homebox/backend/internal/data/repo"
)

// Schedule frequency values accepted by NextRunAt and the API.
const (
	FrequencyDaily  = "daily"
	FrequencyWeekly = "weekly"
)

// NextRunAt computes the next fire time for a schedule after `from`, in
// from's timezone (the server's local time for the scheduler).
//
//   - daily: the next occurrence of timeOfDay ("HH:MM") strictly after from —
//     today if it hasn't passed yet, tomorrow otherwise.
//   - weekly: same rule but anchored to from's weekday — if today's time has
//     passed, the next run is the same weekday next week.
func NextRunAt(frequency, timeOfDay string, from time.Time) (time.Time, error) {
	var hour, minute int
	if _, err := fmt.Sscanf(timeOfDay, "%d:%d", &hour, &minute); err != nil {
		return time.Time{}, fmt.Errorf("invalid time_of_day %q: %w", timeOfDay, err)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid time_of_day %q", timeOfDay)
	}

	loc := from.Location()
	y, m, d := from.Date()
	candidate := time.Date(y, m, d, hour, minute, 0, 0, loc)
	if candidate.After(from) {
		return candidate, nil
	}

	switch frequency {
	case FrequencyDaily:
		return candidate.AddDate(0, 0, 1), nil
	case FrequencyWeekly:
		return candidate.AddDate(0, 0, 7), nil
	default:
		return time.Time{}, fmt.Errorf("invalid frequency %q", frequency)
	}
}

// EnqueueScheduled mirrors Enqueue but flags the row trigger=scheduled so the
// retention sweep can distinguish scheduler-produced backups from
// user-initiated ones.
func (s *ExportService) EnqueueScheduled(ctx context.Context, gid uuid.UUID) (repo.ExportOut, error) {
	ctx, span := otel.Tracer("services").Start(ctx, "ExportService.EnqueueScheduled")
	defer span.End()

	out, err := s.repos.Exports.CreateScheduled(ctx, gid)
	if err != nil {
		return out, err
	}

	if err := s.publishExportJob(ctx, gid, out.ID); err != nil {
		_ = s.repos.Exports.SetFailed(ctx, gid, out.ID, "failed to enqueue: "+err.Error())
		return out, err
	}

	s.publishMutation(gid)
	return out, nil
}

// PruneScheduledExports enforces the schedule's retention: it keeps the
// newest `retention` completed scheduled exports for gid and deletes the
// rest, blob artifact first (the row holds the only pointer to the blob) and
// DB row second. Manual exports are never touched. Blob-delete failures are
// logged and left for the next run rather than aborting the sweep.
func (s *ExportService) PruneScheduledExports(ctx context.Context, gid uuid.UUID, retention int) error {
	ctx, span := otel.Tracer("services").Start(ctx, "ExportService.PruneScheduledExports")
	defer span.End()

	completed, err := s.repos.Exports.ListScheduledCompleted(ctx, gid)
	if err != nil {
		return err
	}
	if len(completed) <= retention {
		return nil
	}

	bucket, err := blob.OpenBucket(ctx, s.repos.Attachments.GetConnString())
	if err != nil {
		return fmt.Errorf("open bucket: %w", err)
	}
	defer func() { _ = bucket.Close() }()

	// completed is oldest-first; everything before the last `retention`
	// entries is pruned.
	stale := completed[:len(completed)-retention]
	for _, e := range stale {
		if e.ArtifactPath != "" {
			err := bucket.Delete(ctx, s.repos.Attachments.GetFullPath(e.ArtifactPath))
			if err != nil && gcerrors.Code(err) != gcerrors.NotFound {
				log.Warn().Err(err).
					Str("export_id", e.ID.String()).
					Str("artifact_path", e.ArtifactPath).
					Msg("backup retention: blob delete failed; leaving row for next sweep")
				continue
			}
		}
		if _, err := s.repos.Exports.Delete(ctx, gid, e.ID); err != nil {
			log.Warn().Err(err).
				Str("export_id", e.ID.String()).
				Msg("backup retention: row delete failed; leaving for next sweep")
			continue
		}
	}
	return nil
}
