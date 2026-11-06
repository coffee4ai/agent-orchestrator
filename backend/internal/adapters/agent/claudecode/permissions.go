package claudecode

import "github.com/aoagents/agent-orchestrator/backend/internal/domain"

// Claude Code permission rules AO denies per session role. Every rule uses the
// documented prefix form Tool(prefix:*) (the form the native reviewer's own
// policy uses), never a bare prefix* glob.

// destructiveDenyRules are refused for every enforced role except the
// orchestrator: force pushes, hard resets, cleans that delete untracked work,
// forced branch deletion, and recursive deletes of the filesystem root, the
// home directory or a parent directory.
var destructiveDenyRules = []string{
	"Bash(git push --force:*)",
	"Bash(git push -f:*)",
	"Bash(git reset --hard:*)",
	"Bash(git clean -fdx:*)",
	"Bash(git clean -fd:*)",
	"Bash(git branch -D:*)",
	"Bash(rm -rf /:*)",
	"Bash(rm -rf ~:*)",
	"Bash(rm -rf ..:*)",
}

// writerDenyRules keep a writer inside its worktree: it neither publishes
// (push, PR create or merge) nor spawns sessions nor records decisions.
var writerDenyRules = []string{
	"Bash(git push:*)",
	"Bash(gh pr create:*)",
	"Bash(gh pr merge:*)",
	"Bash(ao pr merge:*)",
	"Bash(ao spawn:*)",
	"Bash(ao decision record:*)",
}

// controllerDenyRules leave a controller its pushes and PRs but not merges,
// spawns, decisions, or confirming its own hand-off.
var controllerDenyRules = []string{
	"Bash(ao pr merge:*)",
	"Bash(gh pr merge:*)",
	"Bash(ao spawn:*)",
	"Bash(ao decision record:*)",
	"Bash(ao session handoff-confirm:*)",
}

// reviewerDenyRules keep a reviewer from reading other sessions' transcripts,
// on top of everything a writer is denied. They travel on the reviewer's
// launch; GetAgentHooks never writes them to the reviewed worktree's file.
var reviewerDenyRules = []string{
	"Bash(ao session get:*)",
	"Bash(ao session history:*)",
	"Bash(ao session export:*)",
}

// DenyRulesFor returns the Claude Code deny rules for a session role: a fresh
// slice the caller may modify. The orchestrator, an empty role and an unknown
// role get none.
func DenyRulesFor(role domain.SessionRole) []string {
	var groups [][]string
	switch role {
	case domain.SessionRoleWriter:
		groups = [][]string{writerDenyRules, destructiveDenyRules}
	case domain.SessionRoleController:
		groups = [][]string{controllerDenyRules, destructiveDenyRules}
	case domain.SessionRoleReviewer:
		groups = [][]string{reviewerDenyRules, writerDenyRules, destructiveDenyRules}
	default:
		return nil
	}
	var rules []string
	for _, group := range groups {
		rules = append(rules, group...)
	}
	return rules
}
