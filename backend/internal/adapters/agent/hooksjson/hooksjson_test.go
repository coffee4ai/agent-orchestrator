package hooksjson

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReconcileHookMovesCommandToDeclaredMatcher(t *testing.T) {
	oldMatcher := "old"
	newMatcher := "new"
	managed := "ao hooks test session-start"
	userOld := HookEntry{Type: "command", Command: "user old", Timeout: 3}
	userNew := HookEntry{Type: "command", Command: "user new", Timeout: 4}
	updated := HookEntry{Type: "command", Command: managed, Timeout: 30}
	groups := []MatcherGroup{
		{Matcher: &oldMatcher, Hooks: []HookEntry{{Type: "command", Command: managed, Timeout: 5}, userOld}},
		{Matcher: &newMatcher, Hooks: []HookEntry{userNew}},
	}

	got := reconcileHook(groups, updated, &newMatcher)
	want := []MatcherGroup{
		{Matcher: &oldMatcher, Hooks: []HookEntry{userOld}},
		{Matcher: &newMatcher, Hooks: []HookEntry{userNew, updated}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reconcileHook()\nwant: %#v\n got: %#v", want, got)
	}
}

func TestReconcileHookDeduplicatesCommandAcrossGroups(t *testing.T) {
	firstMatcher := "first"
	targetMatcher := "target"
	managed := "ao hooks test session-start"
	user := HookEntry{Type: "command", Command: "user hook", Timeout: 7}
	updated := HookEntry{Type: "command", Command: managed, Timeout: 30}
	groups := []MatcherGroup{
		{Hooks: []HookEntry{{Type: "command", Command: managed, Timeout: 1}}},
		{Matcher: &firstMatcher, Hooks: []HookEntry{user, {Type: "command", Command: managed, Timeout: 2}}},
		{Matcher: &targetMatcher, Hooks: []HookEntry{{Type: "command", Command: managed, Timeout: 3}}},
	}

	got := reconcileHook(groups, updated, &targetMatcher)
	want := []MatcherGroup{
		{Matcher: &firstMatcher, Hooks: []HookEntry{user}},
		{Matcher: &targetMatcher, Hooks: []HookEntry{updated}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reconcileHook()\nwant: %#v\n got: %#v", want, got)
	}
}

func TestReconcileHookDropsGroupsEmptiedByMove(t *testing.T) {
	oldMatcher := "old"
	newMatcher := "new"
	managed := "ao hooks test session-start"
	updated := HookEntry{Type: "command", Command: managed, Timeout: 30}
	groups := []MatcherGroup{
		{Matcher: &oldMatcher, Hooks: []HookEntry{{Type: "command", Command: managed, Timeout: 5}}},
	}

	got := reconcileHook(groups, updated, &newMatcher)
	want := []MatcherGroup{
		{Matcher: &newMatcher, Hooks: []HookEntry{updated}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reconcileHook()\nwant: %#v\n got: %#v", want, got)
	}
}

func TestManagerPreservesUnknownUserHookFields(t *testing.T) {
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, "hooks.json")
	original := []byte(`{
  "customTop": {"keep": true},
  "hooks": {
    "Notification": [{
      "matcher": "permission",
      "groupExtension": {"version": 2},
      "hooks": [{
        "type": "command",
        "command": "user hook",
        "timeout": 7,
        "async": true,
        "commandWindows": "user-hook.cmd",
        "command_windows": "legacy-user-hook.cmd",
        "future": {"enabled": true}
      }]
    }]
  }
}`)
	if err := os.WriteFile(hooksPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	manager := Manager{
		Label:         "test",
		CommandPrefix: "ao hooks test ",
		Timeout:       30,
		Path:          func(string) string { return hooksPath },
		Managed: []HookSpec{
			{Event: "Notification", Command: "ao hooks test notification"},
		},
	}
	if err := manager.Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	assertUnknownUserHookFields(t, hooksPath)

	if err := manager.Uninstall(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	assertUnknownUserHookFields(t, hooksPath)
}

func TestManagerPreservesManagedHookExtensionsOnReconcile(t *testing.T) {
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, "hooks.json")
	original := []byte(`{
  "hooks": {
    "Notification": [{
      "groupExtension": "keep",
      "hooks": [{
        "type": "command",
        "command": "ao hooks test notification",
        "timeout": 7,
        "async": true,
        "commandWindows": "ao-hooks.cmd"
      }]
    }]
  }
}`)
	if err := os.WriteFile(hooksPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	manager := Manager{
		Label:         "test",
		CommandPrefix: "ao hooks test ",
		Timeout:       30,
		Path:          func(string) string { return hooksPath },
		Managed: []HookSpec{
			{Event: "Notification", Command: "ao hooks test notification"},
		},
	}
	if err := manager.Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	group := settings["hooks"].(map[string]any)["Notification"].([]any)[0].(map[string]any)
	if group["groupExtension"] != "keep" {
		t.Fatalf("group extension changed: %#v", group["groupExtension"])
	}
	hook := group["hooks"].([]any)[0].(map[string]any)
	if hook["timeout"] != float64(30) || hook["async"] != true || hook["commandWindows"] != "ao-hooks.cmd" {
		t.Fatalf("managed hook fields = %#v", hook)
	}
}

func assertUnknownUserHookFields(t *testing.T, hooksPath string) {
	t.Helper()
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settings["customTop"], map[string]any{"keep": true}) {
		t.Fatalf("top-level extension changed: %#v", settings["customTop"])
	}
	hooks := settings["hooks"].(map[string]any)
	groups := hooks["Notification"].([]any)
	userGroup := groups[0].(map[string]any)
	if !reflect.DeepEqual(userGroup["groupExtension"], map[string]any{"version": float64(2)}) {
		t.Fatalf("matcher-group extension changed: %#v", userGroup["groupExtension"])
	}
	userHook := userGroup["hooks"].([]any)[0].(map[string]any)
	want := map[string]any{
		"type":            "command",
		"command":         "user hook",
		"timeout":         float64(7),
		"async":           true,
		"commandWindows":  "user-hook.cmd",
		"command_windows": "legacy-user-hook.cmd",
		"future":          map[string]any{"enabled": true},
	}
	if !reflect.DeepEqual(userHook, want) {
		t.Fatalf("user hook fields changed\nwant: %#v\n got: %#v", want, userHook)
	}
}

func permissionsTestManager(hooksPath string, perms *Permissions) Manager {
	return Manager{
		Label:         "test",
		CommandPrefix: "ao hooks test ",
		Timeout:       30,
		Path:          func(string) string { return hooksPath },
		Managed:       []HookSpec{{Event: "Stop", Command: "ao hooks test stop"}},
		Permissions:   perms,
	}
}

type permissionsFile struct {
	Theme       string              `json:"theme"`
	Permissions map[string][]string `json:"permissions"`
	Managed     map[string][]string `json:"aoManagedPermissions"`
	Hooks       map[string]any      `json:"hooks"`
}

func readPermissionsFile(t *testing.T, path string) permissionsFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got permissionsFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse %s: %v\n%s", path, err, data)
	}
	return got
}

// TestInstallWritesPermissionsBesideHooks asserts AO's deny and allow entries
// land in the top-level permissions key next to the user's own entries, are
// listed under aoManagedPermissions, and that Uninstall removes only them.
func TestInstallWritesPermissionsBesideHooks(t *testing.T) {
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooksPath, []byte(`{"theme":"x","permissions":{"allow":["Bash(ls:*)"]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	deny := []string{"Bash(git push:*)", "Bash(rm -rf /:*)"}
	allow := []string{"Bash(go test:*)"}
	manager := permissionsTestManager(hooksPath, &Permissions{Deny: deny, Allow: allow})
	if err := manager.Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}

	got := readPermissionsFile(t, hooksPath)
	if got.Theme != "x" {
		t.Fatalf("theme = %q, want the user's key kept", got.Theme)
	}
	if want := []string{"Bash(ls:*)", "Bash(go test:*)"}; !reflect.DeepEqual(got.Permissions["allow"], want) {
		t.Fatalf("permissions.allow = %#v, want %#v", got.Permissions["allow"], want)
	}
	if !reflect.DeepEqual(got.Permissions["deny"], deny) {
		t.Fatalf("permissions.deny = %#v, want exactly AO's %#v", got.Permissions["deny"], deny)
	}
	if want := map[string][]string{"deny": deny, "allow": allow}; !reflect.DeepEqual(got.Managed, want) {
		t.Fatalf("aoManagedPermissions = %#v, want %#v", got.Managed, want)
	}
	if got.Hooks["Stop"] == nil {
		t.Fatalf("hooks = %#v, want the managed Stop hook beside the permissions", got.Hooks)
	}

	if err := manager.Uninstall(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	got = readPermissionsFile(t, hooksPath)
	if got.Theme != "x" {
		t.Fatalf("theme after uninstall = %q", got.Theme)
	}
	if want := map[string][]string{"allow": {"Bash(ls:*)"}}; !reflect.DeepEqual(got.Permissions, want) {
		t.Fatalf("permissions after uninstall = %#v, want only the user's %#v", got.Permissions, want)
	}
	if got.Managed != nil {
		t.Fatalf("aoManagedPermissions after uninstall = %#v, want removed", got.Managed)
	}
}

// TestInstallIdempotentForPermissions asserts a second Install with the same
// permissions rewrites the file byte for byte, and a changed set replaces AO's
// previous entries without touching the user's.
func TestInstallIdempotentForPermissions(t *testing.T) {
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, "settings.local.json")
	if err := os.WriteFile(hooksPath, []byte(`{"permissions":{"deny":["Read(./secrets/**)"],"allow":["Bash(ls:*)"],"defaultMode":"default"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := permissionsTestManager(hooksPath, &Permissions{Deny: []string{"Bash(git push:*)"}, Allow: []string{"Bash(go test:*)"}})
	if err := manager.Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("reinstall changed the file\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	changed := permissionsTestManager(hooksPath, &Permissions{Deny: []string{"Bash(ao spawn:*)"}})
	if err := changed.Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Permissions map[string]any `json:"permissions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"deny":        []any{"Read(./secrets/**)", "Bash(ao spawn:*)"},
		"allow":       []any{"Bash(ls:*)"},
		"defaultMode": "default",
	}
	if !reflect.DeepEqual(settings.Permissions, want) {
		t.Fatalf("permissions after a changed install = %#v, want %#v", settings.Permissions, want)
	}
}

// TestInstallWithoutPermissionsLeavesManagedPermissionsUntouched asserts a
// Manager with no Permissions leaves AO's earlier entries in place.
func TestInstallWithoutPermissionsLeavesManagedPermissionsUntouched(t *testing.T) {
	workspace := t.TempDir()
	hooksPath := filepath.Join(workspace, "settings.local.json")
	if err := permissionsTestManager(hooksPath, &Permissions{Deny: []string{"Bash(git push:*)"}}).Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := permissionsTestManager(hooksPath, nil).Install(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("install without permissions changed the file\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}
