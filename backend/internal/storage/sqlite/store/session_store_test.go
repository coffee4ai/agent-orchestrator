package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// TestSessionRepoNameRoundTrip asserts sessions.repo_name is stored verbatim
// and read back by GetSession and both session listings.
func TestSessionRepoNameRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	for _, repoName := range []string{"", "all", domain.RootWorkspaceRepoName, "api"} {
		rec := sampleRecord("mer")
		rec.RepoName = repoName
		created, err := s.CreateSession(ctx, rec)
		if err != nil {
			t.Fatalf("create repoName=%q: %v", repoName, err)
		}
		got, found, err := s.GetSession(ctx, created.ID)
		if err != nil || !found {
			t.Fatalf("get repoName=%q: found=%v err=%v", repoName, found, err)
		}
		if got.RepoName != repoName {
			t.Fatalf("GetSession repoName = %q, want %q", got.RepoName, repoName)
		}
		byProject, err := s.ListSessions(ctx, "mer")
		if err != nil {
			t.Fatal(err)
		}
		all, err := s.ListAllSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for name, list := range map[string][]domain.SessionRecord{"ListSessions": byProject, "ListAllSessions": all} {
			matched := false
			for _, row := range list {
				if row.ID == created.ID {
					matched = true
					if row.RepoName != repoName {
						t.Fatalf("%s repoName = %q, want %q", name, row.RepoName, repoName)
					}
				}
			}
			if !matched {
				t.Fatalf("%s missing session %s", name, created.ID)
			}
		}
	}
}

// TestSessionRoleRoundTrip asserts sessions.role is stored, read back by
// GetSession and both session listings, and carried in the session_created
// change_log payload.
func TestSessionRoleRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mer")
	for _, role := range []domain.SessionRole{"", domain.SessionRoleOrchestrator, domain.SessionRoleController, domain.SessionRoleWriter, domain.SessionRoleReviewer} {
		rec := sampleRecord("mer")
		rec.Role = role
		created, err := s.CreateSession(ctx, rec)
		if err != nil {
			t.Fatalf("create role=%q: %v", role, err)
		}
		got, found, err := s.GetSession(ctx, created.ID)
		if err != nil || !found {
			t.Fatalf("get role=%q: found=%v err=%v", role, found, err)
		}
		if got.Role != role {
			t.Fatalf("GetSession role = %q, want %q", got.Role, role)
		}
		byProject, err := s.ListSessions(ctx, "mer")
		if err != nil {
			t.Fatal(err)
		}
		all, err := s.ListAllSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for name, list := range map[string][]domain.SessionRecord{"ListSessions": byProject, "ListAllSessions": all} {
			matched := false
			for _, row := range list {
				if row.ID == created.ID {
					matched = true
					if row.Role != role {
						t.Fatalf("%s role = %q, want %q", name, row.Role, role)
					}
				}
			}
			if !matched {
				t.Fatalf("%s missing session %s", name, created.ID)
			}
		}
		events, err := s.EventsAfter(ctx, 0, 1000)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		for _, ev := range events {
			if ev.Type == "session_created" && ev.SessionID == string(created.ID) {
				if err := json.Unmarshal([]byte(ev.Payload), &payload); err != nil {
					t.Fatal(err)
				}
			}
		}
		if payload["role"] != string(role) {
			t.Fatalf("session_created payload = %#v, want role %q", payload, role)
		}
	}
}

// TestSessionRoleRejectsUnknownValue asserts the column's CHECK refuses a role
// outside the four (and the empty legacy value).
func TestSessionRoleRejectsUnknownValue(t *testing.T) {
	s := newTestStore(t)
	seedProject(t, s, "mer")
	rec := sampleRecord("mer")
	rec.Role = "admin"
	if _, err := s.CreateSession(context.Background(), rec); err == nil {
		t.Fatal("CreateSession with role admin succeeded, want the CHECK constraint to refuse it")
	}
}
