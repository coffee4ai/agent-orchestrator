-- +goose Up
-- What a workspace-project session was spawned on (`ao spawn --repo`), stored
-- verbatim: a child repository's name, '__root__', 'all', or '' for a spawn that
-- named none (the root with every child, as before). No CDC change: repo_name
-- is fixed at spawn and is not in the session_created payload; consumers read
-- the session row.
ALTER TABLE sessions ADD COLUMN repo_name TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE sessions DROP COLUMN repo_name;
