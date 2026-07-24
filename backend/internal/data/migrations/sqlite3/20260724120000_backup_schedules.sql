-- +goose Up
create table if not exists backup_schedules
(
    id          uuid                     not null
        primary key,
    created_at  datetime                 not null,
    updated_at  datetime                 not null,
    enabled     boolean  default false   not null,
    frequency   text     default 'daily' not null
        check (frequency in ('daily', 'weekly')),
    time_of_day text     default '03:00' not null,
    retention   integer  default 7       not null,
    last_run_at datetime,
    next_run_at datetime,
    group_id    uuid                     not null
        constraint backup_schedules_groups_backup_schedule
            references groups
            on delete cascade
);

create unique index if not exists backupschedule_group_id
    on backup_schedules (group_id);

alter table exports
    add column export_trigger text default 'manual' not null
        check (export_trigger in ('manual', 'scheduled'));

-- +goose Down
alter table exports
    drop column export_trigger;

drop table if exists backup_schedules;
