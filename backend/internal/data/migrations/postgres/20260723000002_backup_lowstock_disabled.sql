-- +goose Up
-- exports: allow kind='backup' (scheduled auto-backup artifacts)
alter table exports drop constraint exports_kind_check;
alter table exports add constraint exports_kind_check check (kind in ('export', 'import', 'backup'));

-- entities: low stock threshold + notification latch
alter table entities add column low_stock_threshold double precision;
alter table entities add column low_stock_notified boolean not null default false;

-- users: admin-managed account disable flag
alter table users add column disabled boolean not null default false;

-- +goose Down
alter table users drop column disabled;
alter table entities drop column low_stock_notified;
alter table entities drop column low_stock_threshold;
alter table exports drop constraint exports_kind_check;
alter table exports add constraint exports_kind_check check (kind in ('export', 'import'));
