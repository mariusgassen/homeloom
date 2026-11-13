-- +goose Up
-- How a reminders app knows a shopping item: the UID it gave it and the resource name it is kept
-- under. Both stay empty for items made in Homeloom.
ALTER TABLE shopping_items ADD COLUMN ical_uid text, ADD COLUMN ical_name text;
CREATE UNIQUE INDEX shopping_items_ical_name_idx ON shopping_items (household_id, ical_name) WHERE ical_name IS NOT NULL;

-- +goose Down
DROP INDEX shopping_items_ical_name_idx;
ALTER TABLE shopping_items DROP COLUMN ical_name, DROP COLUMN ical_uid;
