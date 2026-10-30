-- +goose Up
-- Each household's pick for tonight. The title's name and link are kept, so members whose own Plex
-- can't reach it still see what was picked. The API ignores a pick from before this morning.
CREATE TABLE watch_tonight (
    household_id uuid PRIMARY KEY REFERENCES households (id) ON DELETE CASCADE,
    title_id     text NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('movie', 'show')),
    name         text NOT NULL,
    year         integer NOT NULL DEFAULT 0,
    url          text NOT NULL DEFAULT '',
    picked_by    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    picked_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE watch_tonight;
