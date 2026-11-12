-- +goose Up
-- An occasion is a date with a repeat: once, every week, every month or every year.
ALTER TABLE occasions ADD COLUMN on_date date;
ALTER TABLE occasions ADD COLUMN repeat text NOT NULL DEFAULT 'yearly' CHECK (repeat IN ('never', 'weekly', 'monthly', 'yearly'));
UPDATE occasions SET on_date = make_date(coalesce(year, 2000), month, day);
ALTER TABLE occasions ALTER COLUMN on_date SET NOT NULL;
ALTER TABLE occasions DROP COLUMN month, DROP COLUMN day, DROP COLUMN year;

-- +goose Down
ALTER TABLE occasions ADD COLUMN month smallint, ADD COLUMN day smallint, ADD COLUMN year smallint;
UPDATE occasions SET month = extract(month FROM on_date), day = extract(day FROM on_date);
ALTER TABLE occasions ALTER COLUMN month SET NOT NULL, ALTER COLUMN day SET NOT NULL;
ALTER TABLE occasions DROP COLUMN repeat, DROP COLUMN on_date;
