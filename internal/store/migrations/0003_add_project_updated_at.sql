-- +goose Up
-- +goose StatementBegin
ALTER TABLE project ADD COLUMN updated_at TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE project DROP COLUMN updated_at;
-- +goose StatementEnd