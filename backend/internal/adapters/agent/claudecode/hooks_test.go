package claudecode

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/ports"
)

type settingsPermissions struct {
	Permissions map[string][]string `json:"permissions"`
	Managed     map[string][]string `json:"aoManagedPermissions"`
}

func readSettingsPermissions(t *testing.T, workspace string) settingsPermissions {
	t.Helper()
	data, err := os.ReadFile(claudeSettingsPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	var got settingsPermissions
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse settings: %v\n%s", err, data)
	}
	return got
}

func roleHookConfig(workspace string, role domain.SessionRole, allow []string) ports.WorkspaceHookConfig {
	return ports.WorkspaceHookConfig{
		WorkspacePath:    workspace,
		Role:             role,
		PermissionsDeny:  DenyRulesFor(role),
		PermissionsAllow: allow,
	}
}

// TestGetAgentHooksWritesRoleDenyList asserts each role's deny list lands in
// permissions.deny and the configured allow rules in permissions.allow, and
// that the reviewer role writes no permission rules at all.
func TestGetAgentHooksWritesRoleDenyList(t *testing.T) {
	allow := []string{"Bash(go test:*)"}
	for _, role := range []domain.SessionRole{domain.SessionRoleWriter, domain.SessionRoleController, domain.SessionRoleOrchestrator} {
		t.Run(string(role), func(t *testing.T) {
			workspace := t.TempDir()
			if err := (&Plugin{}).GetAgentHooks(t.Context(), roleHookConfig(workspace, role, allow)); err != nil {
				t.Fatal(err)
			}
			got := readSettingsPermissions(t, workspace)
			if want := DenyRulesFor(role); !reflect.DeepEqual(got.Permissions["deny"], want) {
				t.Fatalf("permissions.deny = %#v, want %#v", got.Permissions["deny"], want)
			}
			if !reflect.DeepEqual(got.Permissions["allow"], allow) {
				t.Fatalf("permissions.allow = %#v, want %#v", got.Permissions["allow"], allow)
			}
		})
	}
	t.Run("reviewer", func(t *testing.T) {
		workspace := t.TempDir()
		if err := (&Plugin{}).GetAgentHooks(t.Context(), roleHookConfig(workspace, domain.SessionRoleReviewer, allow)); err != nil {
			t.Fatal(err)
		}
		got := readSettingsPermissions(t, workspace)
		if got.Permissions != nil || got.Managed != nil {
			t.Fatalf("reviewer wrote permission rules: %#v managed %#v", got.Permissions, got.Managed)
		}
	})
}

// TestDenyListSurvivesReinstall asserts reinstalling the same role's hooks
// rewrites the settings file byte for byte.
func TestDenyListSurvivesReinstall(t *testing.T) {
	workspace := t.TempDir()
	cfg := roleHookConfig(workspace, domain.SessionRoleWriter, []string{"Bash(go test:*)"})
	if err := (&Plugin{}).GetAgentHooks(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(claudeSettingsPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Plugin{}).GetAgentHooks(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(claudeSettingsPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("reinstall changed the settings file\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if got := readSettingsPermissions(t, workspace); !reflect.DeepEqual(got.Permissions["deny"], cfg.PermissionsDeny) {
		t.Fatalf("permissions.deny after reinstall = %#v, want %#v", got.Permissions["deny"], cfg.PermissionsDeny)
	}
}

// TestEmptyRoleLeavesManagedPermissionsUntouched asserts a hooks install that
// names no role (the native reviewer's PreLaunch in the worker's checkout)
// keeps the writer's deny list in place.
func TestEmptyRoleLeavesManagedPermissionsUntouched(t *testing.T) {
	workspace := t.TempDir()
	if err := (&Plugin{}).GetAgentHooks(t.Context(), roleHookConfig(workspace, domain.SessionRoleWriter, nil)); err != nil {
		t.Fatal(err)
	}
	if err := (&Plugin{}).GetAgentHooks(t.Context(), ports.WorkspaceHookConfig{WorkspacePath: workspace}); err != nil {
		t.Fatal(err)
	}
	got := readSettingsPermissions(t, workspace)
	if want := DenyRulesFor(domain.SessionRoleWriter); !reflect.DeepEqual(got.Permissions["deny"], want) {
		t.Fatalf("permissions.deny after an empty-role install = %#v, want the writer's %#v", got.Permissions["deny"], want)
	}
}
