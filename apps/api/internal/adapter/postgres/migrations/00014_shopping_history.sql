-- +goose Up
-- What a household has put on its shopping list, kept after items are cleared, for suggestions.
-- key is the lower-cased text; text is how it was last written.
CREATE TABLE shopping_history (
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    key          text NOT NULL,
    text         text NOT NULL,
    uses         integer NOT NULL DEFAULT 1,
    last_used    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, key)
);

INSERT INTO shopping_history (household_id, key, text, uses, last_used)
SELECT household_id, lower(text), (array_agg(text ORDER BY created_at DESC))[1], count(*), max(created_at)
FROM shopping_items
GROUP BY household_id, lower(text);

-- +goose Down
DROP TABLE shopping_history;
