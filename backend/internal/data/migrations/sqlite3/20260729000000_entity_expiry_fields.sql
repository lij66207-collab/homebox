-- +goose Up
-- entities: production date (生产日期), shelf life in days (保质期) and
-- expiry date (截止日期). All optional; expiry reminders read expiry_date.
-- Date-only semantics are enforced by the Go layer (internal/data/types.Date),
-- matching purchase_date/sold_date (see 20260425000000_rename_date_columns.sql).
ALTER TABLE entities ADD COLUMN production_date datetime;
ALTER TABLE entities ADD COLUMN shelf_life_days integer;
ALTER TABLE entities ADD COLUMN expiry_date datetime;

-- +goose Down
ALTER TABLE entities DROP COLUMN production_date;
ALTER TABLE entities DROP COLUMN shelf_life_days;
ALTER TABLE entities DROP COLUMN expiry_date;
