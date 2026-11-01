-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS users (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
