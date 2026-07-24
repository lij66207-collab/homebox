-- +goose Up
-- Create "backup_schedules" table
CREATE TABLE IF NOT EXISTS "backup_schedules" (
    "id" uuid NOT NULL,
    "created_at" timestamptz NOT NULL,
    "updated_at" timestamptz NOT NULL,
    "enabled" boolean NOT NULL DEFAULT false,
    "frequency" character varying NOT NULL DEFAULT 'daily'
        CHECK ("frequency" IN ('daily', 'weekly')),
    "time_of_day" character varying NOT NULL DEFAULT '03:00',
    "retention" bigint NOT NULL DEFAULT 7,
    "last_run_at" timestamptz NULL,
    "next_run_at" timestamptz NULL,
    "group_id" uuid NOT NULL,
    PRIMARY KEY ("id"),
    CONSTRAINT "backup_schedules_groups_backup_schedule" FOREIGN KEY ("group_id") REFERENCES "groups" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "backupschedule_group_id" to table: "backup_schedules"
CREATE UNIQUE INDEX IF NOT EXISTS "backupschedule_group_id" ON "backup_schedules" ("group_id");

-- Add "export_trigger" column to "exports" table
ALTER TABLE "exports"
    ADD COLUMN "export_trigger" character varying NOT NULL DEFAULT 'manual'
        CHECK ("export_trigger" IN ('manual', 'scheduled'));

-- +goose Down
ALTER TABLE "exports" DROP COLUMN "export_trigger";
DROP TABLE IF EXISTS "backup_schedules";
