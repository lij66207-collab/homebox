-- +goose Up
-- exports: allow kind='backup' (scheduled auto-backup artifacts)
create table exports_backup_kind
(
    id            uuid                       not null
        primary key,
    created_at    datetime                   not null,
    updated_at    datetime                   not null,
    kind          text     default 'export'  not null
        check (kind in ('export', 'import', 'backup')),
    status        text     default 'pending' not null
        check (status in ('pending', 'running', 'completed', 'failed')),
    progress      integer  default 0         not null,
    artifact_path text,
    size_bytes    integer  default 0         not null,
    error         text
        check (error is null or length(error) <= 1000),
    group_id      uuid                       not null
        constraint exports_groups_exports
            references groups
            on delete cascade
);

insert into exports_backup_kind (id, created_at, updated_at, kind, status, progress, artifact_path, size_bytes, error, group_id)
select id, created_at, updated_at, kind, status, progress, artifact_path, size_bytes, error, group_id from exports;

drop table exports;
alter table exports_backup_kind rename to exports;

create index if not exists export_group_id
    on exports (group_id);

create index if not exists export_group_id_status
    on exports (group_id, status);

-- entities: low stock threshold + notification latch
alter table entities add column low_stock_threshold real;
alter table entities add column low_stock_notified integer not null default 0;

-- users: admin-managed account disable flag
alter table users add column disabled integer not null default 0;

-- +goose Down
alter table users drop column disabled;
alter table entities drop column low_stock_notified;
alter table entities drop column low_stock_threshold;
