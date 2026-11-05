-- +goose Up
-- What a session may do (`ao spawn --role`), fixed at spawn: orchestrator,
-- controller, writer or reviewer; '' for a session spawned before roles. The
-- role never changes, so only session_created carries it: the insert trigger
-- is recreated with role in its payload and no update trigger watches it.
ALTER TABLE sessions ADD COLUMN role TEXT NOT NULL DEFAULT ''
    CHECK (role IN ('', 'orchestrator', 'controller', 'writer', 'reviewer'));

DROP TRIGGER IF EXISTS sessions_cdc_insert;

-- +goose StatementBegin
CREATE TRIGGER sessions_cdc_insert
AFTER INSERT ON sessions
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (NEW.project_id, NEW.id, 'session_created',
        json_object('id', NEW.id, 'activity', NEW.activity_state, 'isTerminated', json(CASE WHEN NEW.is_terminated THEN 'true' ELSE 'false' END), 'role', NEW.role),
        NEW.updated_at);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS sessions_cdc_insert;

-- +goose StatementBegin
CREATE TRIGGER sessions_cdc_insert
AFTER INSERT ON sessions
BEGIN
    INSERT INTO change_log (project_id, session_id, event_type, payload, created_at)
    VALUES (NEW.project_id, NEW.id, 'session_created',
        json_object('id', NEW.id, 'activity', NEW.activity_state, 'isTerminated', json(CASE WHEN NEW.is_terminated THEN 'true' ELSE 'false' END)),
        NEW.updated_at);
END;
-- +goose StatementEnd

ALTER TABLE sessions DROP COLUMN role;
