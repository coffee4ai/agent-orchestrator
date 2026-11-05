package sessionmanager

import (
	"errors"
	"fmt"

	claudeagent "github.com/aoagents/agent-orchestrator/backend/internal/adapters/agent/claudecode"
	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

// rolePermissionRules resolves the permission rules a session's settings file
// carries: its role's deny rules when the project enforces strictRoles (none
// otherwise) and the project's allow rules for the session's repository and
// role.
func rolePermissionRules(projectConfig domain.ProjectConfig, role domain.SessionRole, repoName string) (deny, allow []string) {
	if role == "" {
		return nil, nil
	}
	if projectConfig.StrictRolesEnabled() {
		deny = claudeagent.DenyRulesFor(role)
	}
	return deny, projectConfig.AllowRulesFor(repoName, role)
}

// ErrPermissionModeNotAllowedForRole reports a spawn, under a project's
// strictRoles, of a writer or controller whose resolved permission mode would
// let it run tools without the deny rules applying (auto or
// bypass-permissions).
var ErrPermissionModeNotAllowedForRole = errors.New("permission mode not allowed for role")

// checkRolePermissionMode refuses, under strictRoles, a writer or controller
// whose resolved permission mode is auto or bypass-permissions.
func checkRolePermissionMode(projectConfig domain.ProjectConfig, role domain.SessionRole, mode domain.PermissionMode) error {
	if !projectConfig.StrictRolesEnabled() || (role != domain.SessionRoleWriter && role != domain.SessionRoleController) {
		return nil
	}
	switch ports.NormalizePermissionMode(mode) {
	case ports.PermissionModeAuto, ports.PermissionModeBypassPermissions:
		return fmt.Errorf("%w: role %q cannot run in %q mode while the project enforces strictRoles", ErrPermissionModeNotAllowedForRole, role, mode)
	}
	return nil
}
