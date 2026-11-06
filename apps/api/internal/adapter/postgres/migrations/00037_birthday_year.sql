-- +goose Up
-- The year of birth is optional, so the others know which birthday it is.
ALTER TABLE birthdays ADD COLUMN year smallint CHECK (year BETWEEN 1900 AND 9999);

-- +goose Down
ALTER TABLE birthdays DROP COLUMN year;
