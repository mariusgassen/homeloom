-- +goose Up
-- When a photo of the receipt was kept in S3 (receipts/<id>.jpg); NULL when there is none.
ALTER TABLE expenses ADD COLUMN receipt_at timestamptz;

-- +goose Down
ALTER TABLE expenses DROP COLUMN receipt_at;
