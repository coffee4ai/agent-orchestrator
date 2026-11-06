package claudecode

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

// TestDenyRulesForRoles pins the exact rule strings per role and the prefix
// form every rule must use.
func TestDenyRulesForRoles(t *testing.T) {
	destructive := []string{
		"Bash(git push --force:*)", "Bash(git push -f:*)", "Bash(git reset --hard:*)",
		"Bash(git clean -fdx:*)", "Bash(git clean -fd:*)", "Bash(git branch -D:*)",
		"Bash(rm -rf /:*)", "Bash(rm -rf ~:*)", "Bash(rm -rf ..:*)",
	}
	writer := []string{
		"Bash(git push:*)", "Bash(gh pr create:*)", "Bash(gh pr merge:*)",
		"Bash(ao pr merge:*)", "Bash(ao spawn:*)", "Bash(ao decision record:*)",
		"Bash(ao project set-config:*)", "Bash(ao project add:*)", "Bash(ao project rm:*)",
	}
	controller := []string{
		"Bash(ao pr merge:*)", "Bash(gh pr merge:*)", "Bash(ao spawn:*)",
		"Bash(ao decision record:*)", "Bash(ao session handoff-confirm:*)",
		"Bash(ao project set-config:*)", "Bash(ao project add:*)", "Bash(ao project rm:*)",
	}
	reviewer := []string{"Bash(ao session get:*)", "Bash(ao session history:*)", "Bash(ao session export:*)"}
	cases := map[domain.SessionRole][]string{
		domain.SessionRoleWriter:       append(append([]string{}, writer...), destructive...),
		domain.SessionRoleController:   append(append([]string{}, controller...), destructive...),
		domain.SessionRoleReviewer:     append(append(append([]string{}, reviewer...), writer...), destructive...),
		domain.SessionRoleOrchestrator: nil,
		"":                             nil,
	}
	for role, want := range cases {
		got := DenyRulesFor(role)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("DenyRulesFor(%q) = %#v, want %#v", role, got, want)
		}
		for _, rule := range got {
			if !strings.HasPrefix(rule, "Bash(") || !strings.HasSuffix(rule, ":*)") || strings.Contains(rule, " :*") {
				t.Fatalf("DenyRulesFor(%q) rule %q is not in the Tool(prefix:*) form", role, rule)
			}
		}
	}
}
