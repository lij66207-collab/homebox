package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/sysadminsmedia/homebox/backend/internal/data/ent/schema/mixins"
)

// BackupSchedule holds the schema definition for the BackupSchedule entity.
// Each group has at most one row describing its recurring automatic export:
// whether it runs, how often, at what local time, and how many produced
// artifacts are kept. The scheduler stamps last_run_at/next_run_at after
// every trigger.
type BackupSchedule struct {
	ent.Schema
}

func (BackupSchedule) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.BaseMixin{},
		GroupMixin{
			ref:   "backup_schedule",
			field: "group_id",
		},
	}
}

func (BackupSchedule) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("enabled").
			Default(false),
		field.Enum("frequency").
			Values("daily", "weekly").
			Default("daily"),
		// time_of_day is "HH:MM" in the server's local timezone.
		field.String("time_of_day").
			Default("03:00"),
		// retention is how many scheduled exports are kept per group; older
		// ones (and their blob artifacts) are purged. Manual exports are
		// never touched by retention.
		field.Int("retention").
			Default(7),
		field.Time("last_run_at").
			Optional().
			Nillable(),
		field.Time("next_run_at").
			Optional().
			Nillable(),
	}
}

func (BackupSchedule) Indexes() []ent.Index {
	return []ent.Index{
		// One schedule per group.
		index.Fields("group_id").Unique(),
	}
}
