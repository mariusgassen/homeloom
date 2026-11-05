-- +goose Up
-- What each member swiped on movie night. A swipe is about a film, not a Plex id, since two
-- people's Plex accounts may name the same film differently: key is its folded name, year and
-- kind. title_id is the swiper's own id, so they can open it again. The API ignores swipes from
-- before this morning.
CREATE TABLE watch_swipes (
    household_id uuid NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key          text NOT NULL,
    title_id     text NOT NULL,
    name         text NOT NULL,
    year         integer NOT NULL DEFAULT 0,
    liked        boolean NOT NULL,
    swiped_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id, key)
);

-- +goose Down
DROP TABLE watch_swipes;
