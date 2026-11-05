package hooksjson

import (
	"encoding/json"
	"fmt"
	"slices"
)

// permissionsKey is the settings file's permission-rule object; Claude Code
// reads its deny and allow lists and rejects unknown keys inside it.
const permissionsKey = "permissions"

// ManagedPermissionsKey is the top-level key that lists the exact rule strings
// AO wrote into permissions.deny and permissions.allow. It sits outside
// "permissions" because Claude Code rejects unknown keys there; Uninstall and a
// reinstall read it to tell AO's entries from the user's.
const ManagedPermissionsKey = "aoManagedPermissions"

// Permissions is the permission-rule set AO owns in a settings file.
type Permissions struct {
	Deny  []string `json:"deny,omitempty"`
	Allow []string `json:"allow,omitempty"`
}

// reconcilePermissions replaces AO's previously managed entries in
// permissions.deny and permissions.allow with want, preserving every user
// entry (in its original order, ahead of AO's) and every other key of the
// permissions object. A rule the user already lists is left theirs and is not
// recorded as AO's, so removing AO's entries never removes it. A nil want only
// removes AO's entries (Uninstall).
func reconcilePermissions(topLevel map[string]json.RawMessage, want *Permissions) error {
	var previous Permissions
	if raw, ok := topLevel[ManagedPermissionsKey]; ok {
		if err := json.Unmarshal(raw, &previous); err != nil {
			return fmt.Errorf("parse %s: %w", ManagedPermissionsKey, err)
		}
	}
	perms := map[string]json.RawMessage{}
	if raw, ok := topLevel[permissionsKey]; ok {
		if err := json.Unmarshal(raw, &perms); err != nil {
			return fmt.Errorf("parse %s: %w", permissionsKey, err)
		}
		if perms == nil {
			perms = map[string]json.RawMessage{}
		}
	}
	if want == nil {
		want = &Permissions{}
	}
	var managed Permissions
	var err error
	if managed.Deny, err = reconcileRuleList(perms, "deny", previous.Deny, want.Deny); err != nil {
		return err
	}
	if managed.Allow, err = reconcileRuleList(perms, "allow", previous.Allow, want.Allow); err != nil {
		return err
	}

	if len(perms) == 0 {
		delete(topLevel, permissionsKey)
	} else if err := setRawField(topLevel, permissionsKey, perms); err != nil {
		return err
	}
	if len(managed.Deny) == 0 && len(managed.Allow) == 0 {
		delete(topLevel, ManagedPermissionsKey)
		return nil
	}
	return setRawField(topLevel, ManagedPermissionsKey, managed)
}

// reconcileRuleList rewrites one rule list of the permissions object: it drops
// the entries AO wrote last time, keeps the user's, and appends want's entries
// the user does not already list. It returns the entries AO now owns.
func reconcileRuleList(perms map[string]json.RawMessage, key string, previous, want []string) ([]string, error) {
	var rules []string
	if raw, ok := perms[key]; ok {
		if err := json.Unmarshal(raw, &rules); err != nil {
			return nil, fmt.Errorf("parse %s.%s: %w", permissionsKey, key, err)
		}
	}
	kept := make([]string, 0, len(rules)+len(want))
	for _, rule := range rules {
		if !slices.Contains(previous, rule) {
			kept = append(kept, rule)
		}
	}
	var owned []string
	for _, rule := range want {
		if slices.Contains(kept, rule) {
			continue
		}
		kept = append(kept, rule)
		owned = append(owned, rule)
	}
	if len(kept) == 0 {
		delete(perms, key)
		return owned, nil
	}
	return owned, setRawField(perms, key, kept)
}
