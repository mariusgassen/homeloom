-- +goose Up
-- Files kept in the S3 store (documents/<id>). Private to the owner until shared with the household.
CREATE TABLE documents (
    id           uuid PRIMARY KEY,
    household_id uuid        NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    owner_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         text        NOT NULL,
    content_type text        NOT NULL,
    size         bigint      NOT NULL CHECK (size >= 0),
    shared       boolean     NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX documents_household_idx ON documents (household_id, created_at DESC);

-- +goose Down
DROP TABLE documents;
