package sqlite

import (
	"database/sql"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// The fork's single change_log rebuild widens event_type once for every table
// the fork adds. Later fork packages only add triggers writing these types.
const (
	forkEventTypesBaseVersion      int64 = 20261028124744441
	forkEventTypesMigrationVersion int64 = 20261105193701744
)

var forkEventTypes = []string{
	"permission_event_recorded",
	"decision_updated",
	"plan_updated",
	"task_updated",
	"task_round_updated",
	"lock_updated",
	"handoff_updated",
	"send_delivery_updated",
	"report_updated",
}

// forkEventTypesBaseTriggers is the full trigger inventory at the fork base
// (upstream 62a2de7f92). The rebuild drops and recreates the ones that write
// change_log; every name and body must be identical afterwards.
var forkEventTypesBaseTriggers = []string{
	"agent_model_catalog_cdc_insert",
	"agent_model_catalog_cdc_update",
	"agent_switches_cdc_insert",
	"agent_switches_cdc_update",
	"agent_switches_failed_recovery_marker_insert",
	"agent_switches_failed_recovery_marker_update",
	"agent_switches_target_native_scope_insert",
	"agent_switches_target_native_scope_update",
	"conversation_account_cdc_update",
	"conversation_activities_branch_insert",
	"conversation_activities_cdc_insert",
	"conversation_activities_cdc_update",
	"conversation_branch_root_provider_update",
	"conversation_messages_branch_insert",
	"conversation_messages_cdc_insert",
	"conversation_messages_cdc_update",
	"conversation_messages_fts_delete",
	"conversation_messages_fts_insert",
	"conversation_messages_fts_update",
	"conversation_provider_events_branch_insert",
	"conversation_turns_auto_resume_cdc_update",
	"conversation_turns_branch_insert",
	"conversation_turns_cdc_update",
	"pr_cdc_insert",
	"pr_cdc_update",
	"pr_checks_cdc_insert",
	"pr_checks_cdc_update",
	"pr_review_threads_cdc_insert",
	"pr_review_threads_cdc_update",
	"pr_session_cdc_update",
	"report_outputs_pr_created_cdc",
	"review_conversation_branch_root_provider_update",
	"review_conversation_title_cdc_update",
	"review_run_cdc_insert",
	"review_run_cdc_update",
	"session_cleanup_facts_cdc_insert",
	"session_cleanup_facts_cdc_update",
	"session_interface_transitions_cdc_insert",
	"session_interface_transitions_cdc_update",
	"sessions_cdc_insert",
	"sessions_cdc_update",
	"sessions_hibernation_cdc_update",
	"sessions_record_harness_release_insert",
	"sessions_record_harness_release_update",
	"sessions_revision_update",
	"sessions_stale_binary_cdc_update",
	"usage_bindings_cdc_insert",
	"usage_bindings_cdc_update",
	"usage_sources_cdc_update",
}

func TestForkEventTypesAccepted(t *testing.T) {
	db := openMigratedDatabaseCopy(t, forkEventTypesMigrationVersion)
	for _, eventType := range forkEventTypes {
		if _, err := db.Exec(`INSERT INTO change_log (project_id, session_id, event_type, payload)
VALUES (NULL, NULL, ?, '{}')`, eventType); err != nil {
			t.Errorf("insert change_log event_type %q: %v", eventType, err)
		}
	}
}

type forkChangeLogRow struct {
	seq       int64
	projectID sql.NullString
	sessionID sql.NullString
	eventType string
	payload   string
	createdAt string
}

func readForkChangeLogRows(t *testing.T, db *sql.DB) []forkChangeLogRow {
	t.Helper()
	rows, err := db.Query(`SELECT seq, project_id, session_id, event_type, payload, CAST(created_at AS TEXT)
FROM change_log ORDER BY seq`)
	if err != nil {
		t.Fatalf("read change_log: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []forkChangeLogRow
	for rows.Next() {
		var r forkChangeLogRow
		if err := rows.Scan(&r.seq, &r.projectID, &r.sessionID, &r.eventType, &r.payload, &r.createdAt); err != nil {
			t.Fatalf("scan change_log: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate change_log: %v", err)
	}
	return got
}

func changeLogSequence(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var seq int64
	if err := db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'change_log'`).Scan(&seq); err != nil {
		t.Fatalf("read change_log sqlite_sequence: %v", err)
	}
	return seq
}

func TestForkEventTypesRebuildKeepsRows(t *testing.T) {
	db := openMigratedDatabaseCopy(t, forkEventTypesBaseVersion)
	mustExec(t, db, `INSERT INTO projects (id, path, registered_at) VALUES ('fork-project', '/repo/fork', ?)`,
		time.Unix(100, 0).UTC())
	mustExec(t, db, `INSERT INTO change_log (seq, project_id, session_id, event_type, payload, created_at)
VALUES (41, 'fork-project', NULL, 'session_updated', '{"kind":"first"}', '2026-11-01 10:00:00'),
       (42, NULL, NULL, 'review_run_updated', '{"kind":"second"}', '2026-11-01 10:00:01'),
       (43, NULL, NULL, 'pr_updated', '{"kind":"pruned"}', '2026-11-01 10:00:02')`)
	// A pruned tail must not let the rebuilt table reuse its seq: consumers
	// track their own offset by seq.
	mustExec(t, db, `DELETE FROM change_log WHERE seq = 43`)
	before := readForkChangeLogRows(t, db)
	if len(before) != 2 {
		t.Fatalf("seeded %d rows, want 2", len(before))
	}

	upTo(t, db, forkEventTypesMigrationVersion)

	if after := readForkChangeLogRows(t, db); !reflect.DeepEqual(after, before) {
		t.Fatalf("change_log rows after rebuild = %+v, want %+v", after, before)
	}
	if got := changeLogSequence(t, db); got != 43 {
		t.Fatalf("change_log sqlite_sequence = %d, want 43", got)
	}
	var next int64
	if err := db.QueryRow(`INSERT INTO change_log (event_type, payload) VALUES ('task_updated', '{}') RETURNING seq`).Scan(&next); err != nil {
		t.Fatalf("insert after rebuild: %v", err)
	}
	if next != 44 {
		t.Fatalf("next seq after rebuild = %d, want 44", next)
	}
}

func schemaSQL(t *testing.T, db *sql.DB, query string) map[string]string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	defer func() { _ = rows.Close() }()
	got := map[string]string{}
	for rows.Next() {
		var name, body string
		if err := rows.Scan(&name, &body); err != nil {
			t.Fatalf("scan schema: %v", err)
		}
		got[name] = body
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema: %v", err)
	}
	return got
}

func triggerSQL(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	return schemaSQL(t, db, `SELECT name, sql FROM sqlite_master WHERE type = 'trigger'`)
}

func changeLogSchemaSQL(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	return schemaSQL(t, db, `SELECT type || ' ' || name, sql FROM sqlite_master WHERE tbl_name = 'change_log'`)
}

func assertSameSchema(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	for name, body := range want {
		if got[name] != body {
			t.Errorf("%s %s:\n got: %s\nwant: %s", what, name, got[name], body)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s %s appeared: %s", what, name, got[name])
		}
	}
}

func TestForkEventTypesRebuildKeepsTriggers(t *testing.T) {
	db := openMigratedDatabaseCopy(t, forkEventTypesBaseVersion)
	before := triggerSQL(t, db)
	names := make([]string, 0, len(before))
	for name := range before {
		names = append(names, name)
	}
	slices.Sort(names)
	if !slices.Equal(names, forkEventTypesBaseTriggers) {
		t.Fatalf("base triggers = %q, want %q", names, forkEventTypesBaseTriggers)
	}
	beforeIndexes := changeLogSchemaSQL(t, db)
	delete(beforeIndexes, "table change_log")

	upTo(t, db, forkEventTypesMigrationVersion)

	assertSameSchema(t, "trigger", triggerSQL(t, db), before)
	afterIndexes := changeLogSchemaSQL(t, db)
	delete(afterIndexes, "table change_log")
	assertSameSchema(t, "change_log", afterIndexes, beforeIndexes)
}

func TestForkEventTypesDownRestoresCheck(t *testing.T) {
	db := openMigratedDatabaseCopy(t, forkEventTypesBaseVersion)
	beforeTriggers := triggerSQL(t, db)
	beforeSchema := changeLogSchemaSQL(t, db)
	mustExec(t, db, `INSERT INTO change_log (seq, event_type, payload) VALUES (7, 'session_updated', '{}')`)

	upTo(t, db, forkEventTypesMigrationVersion)
	mustExec(t, db, `INSERT INTO change_log (seq, event_type, payload) VALUES (8, 'plan_updated', '{}')`)
	downTo(t, db, forkEventTypesBaseVersion)

	assertSameSchema(t, "trigger", triggerSQL(t, db), beforeTriggers)
	assertSameSchema(t, "change_log", changeLogSchemaSQL(t, db), beforeSchema)
	rows := readForkChangeLogRows(t, db)
	if len(rows) != 1 || rows[0].seq != 7 {
		t.Fatalf("change_log rows after down = %+v, want only seq 7", rows)
	}
	if got := changeLogSequence(t, db); got != 8 {
		t.Fatalf("change_log sqlite_sequence after down = %d, want 8", got)
	}
	_, err := db.Exec(`INSERT INTO change_log (event_type, payload) VALUES ('plan_updated', '{}')`)
	if err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("insert fork event type after down: err = %v, want CHECK failure", err)
	}
}
