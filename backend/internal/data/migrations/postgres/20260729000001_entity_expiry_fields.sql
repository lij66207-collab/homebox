-- +goose Up
-- entities: production date (生产日期), shelf life in days (保质期) and
-- expiry date (截止日期). All optional; expiry reminders read expiry_date.
-- timestamptz matches the existing date columns (warranty_expires etc.).
ALTER TABLE entities ADD COLUMN production_date timestamptz NULL;
ALTER TABLE entities ADD COLUMN shelf_life_days integer NULL;
ALTER TABLE entities ADD COLUMN expiry_date timestamptz NULL;

-- +goose Down
ALTER TABLE entities DROP COLUMN production_date;
ALTER TABLE entities DROP COLUMN shelf_life_days;
ALTER TABLE entities DROP COLUMN expiry_date;
