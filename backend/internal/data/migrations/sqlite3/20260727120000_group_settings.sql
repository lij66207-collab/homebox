-- +goose Up
-- groups: per-group JSON settings (assistant config, expiry reminder config, ...)
alter table groups add column settings text not null default '{}';

-- +goose Down
alter table groups drop column settings;
