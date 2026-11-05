package store_test

import (
	"context"
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
