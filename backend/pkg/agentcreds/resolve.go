package agentcreds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Credential resolution for the local machine.
//
// Cloud is handed a secret; the desktop daemon has to find one. That search is
// this file, and it is ordered: Claude Code consults its sources in a fixed
// precedence, and AO must report on the same one the agent will actually send.
// Reporting on a different source is worse than not checking, because it
// produces a confident verdict about a credential that is not in play.
//
// The precedence below was corrected by experiment, not read off the docs:
// CLAUDE_CODE_OAUTH_TOKEN beats ANTHROPIC_API_KEY. Each source is tagged with
// the Kind that selects its header, so no downstream code has to guess.

// Env is an environment lookup, injectable for tests.
type Env func(string) string

// ResolveOptions tunes local credential discovery.
type ResolveOptions struct {
	// Env reads environment variables. Defaults to the process environment.
	Env Env
	// ConfigDir overrides Claude Code's config directory.
	ConfigDir string
	// AllowKeychain permits reading the macOS keychain.
	//
	// This is the switch behind open question Q1. The Cloud provisioning
	// design forbids extracting local keychain state, but that rule is about
	// shipping a developer's credentials into a Cloud sandbox; whether it also
	// bars the local daemon from reading the local keychain for local
	// validation is not settled by its text. The assumption here is that it
	// does not — and every keychain call sits behind one function and this one
	// flag, so reversing the assumption is a one-line change that costs only
	// source 5.
	AllowKeychain bool
	// Runner executes the keychain helper. Defaults to the real one.
	Runner commandRunner
	// GOOS overrides platform detection, for tests.
	GOOS string
	// WorkingDir and CommandEnv reproduce the project launch context when
	// resolving Claude settings and explicit provider selection.
	WorkingDir string
	CommandEnv map[string]string
	// DisableStoredCredentials prevents gateway configuration read from a
	// workspace from inheriting a subscription token from the user's keychain or
	// credentials file. Explicit launch credentials remain eligible.
	DisableStoredCredentials bool
	// Validate asks the provider whether cred is accepted. When set, a
	// credential answered with HTTP 401 is skipped for the next source in the
	// ladder and recorded in Credential.RejectedSources. Any other answer
	// (including a transport failure, status 0) keeps the credential, because
	// only a 401 is evidence it is bad. Nil keeps the plain precedence order.
	// The validator receives the credential in the shape it will be probed in
	// (provider and base URL set) and must never log its Secret.
	Validate func(ctx context.Context, cred Credential) (ok bool, status int)
}

// WithClaudeSettings applies the shared settings resolver to the launch context.
// Explicit command environment values are also used for credential resolution;
// unrelated launch variables remain available to commands without loading them
// from settings files. Neither the caller's options nor its map is mutated.
func (o ResolveOptions) WithClaudeSettings(ctx context.Context) ResolveOptions {
	settings := ResolveClaudeSettings(ctx, o.WorkingDir, o.CommandEnv, o)
	base := o
	merged := make(map[string]string, len(o.CommandEnv)+len(settings.Env))
	for key, value := range o.CommandEnv {
		merged[key] = value
	}
	for key, value := range settings.Env {
		merged[key] = value
	}
	o.CommandEnv = merged
	if settings.WorkspaceProviderRouting {
		o.DisableStoredCredentials = true
	}
	o.Env = func(key string) string {
		if value, ok := merged[key]; ok {
			return value
		}
		return base.env(key)
	}
	return o
}

func (o ResolveOptions) env(name string) string {
	if o.Env != nil {
		return strings.TrimSpace(o.Env(name))
	}
	return strings.TrimSpace(os.Getenv(name))
}

func (o ResolveOptions) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

// ResolveProvider decides which API surface the local configuration points at.
//
// It is a gate, not a guess: an unrecognized apiProvider yields ok=false so
// the caller stays silent rather than probing api.anthropic.com with a
// credential that belongs to Bedrock. Explicit project environment selects the
// provider first because it is also what launch will use; only then may the
// CLI's report refine an otherwise implicit provider choice.
func ResolveProvider(reported string, opts ResolveOptions) (Provider, bool) {
	selected := make([]Provider, 0, 3)
	for _, candidate := range []struct {
		name     string
		provider Provider
	}{
		{"CLAUDE_CODE_USE_BEDROCK", ProviderBedrock},
		{"CLAUDE_CODE_USE_VERTEX", ProviderVertex},
		{"CLAUDE_CODE_USE_FOUNDRY", ProviderFoundry},
	} {
		if envTruthy(opts.env(candidate.name)) {
			selected = append(selected, candidate.provider)
		}
	}
	if len(selected) > 1 {
		return "", false
	}
	if len(selected) == 1 {
		return selected[0], true
	}
	if opts.env("ANTHROPIC_BASE_URL") != "" {
		return ProviderGateway, true
	}
	if trimmed := strings.TrimSpace(reported); trimmed != "" {
		return ParseProvider(trimmed)
	}
	return ProviderFirstParty, true
}

func envTruthy(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

// ResolveLocal finds the credential Claude Code will use for the given
// provider. ok=false means nothing was found, which is Unknown — never a
// statement that the user is signed out.
//
// With opts.Validate set the ladder is env, then keychain, then credentials
// file, and the first credential the provider does not reject with 401 wins.
func ResolveLocal(ctx context.Context, provider Provider, opts ResolveOptions) (Credential, bool) {
	switch provider {
	case ProviderFirstParty:
		return resolveFirstParty(ctx, opts, func(c Credential) Credential { return c })
	case ProviderGateway:
		return resolveFirstParty(ctx, opts, func(c Credential) Credential {
			c.Provider = ProviderGateway
			c.BaseURL = opts.env("ANTHROPIC_BASE_URL")
			return c
		})
	default:
		return Credential{}, false
	}
}

// firstPartySources is the precedence ladder, highest first. apiKeyHelper is
// deliberately absent: honoring it means executing a command named in a
// settings file, which Claude Code itself gates behind workspace trust. AO
// will not turn a credential check into a path for running arbitrary commands
// out of a repository, so a helper-sourced credential resolves to nothing here
// and the verdict stays Unknown.
var firstPartySources = []struct {
	env  string
	kind Kind
}{
	{"CLAUDE_CODE_OAUTH_TOKEN", KindOAuthToken},
	{"ANTHROPIC_API_KEY", KindAPIKey},
	{"ANTHROPIC_AUTH_TOKEN", KindAuthToken},
}

// resolveFirstParty walks the ladder lazily: a later source is read only when
// every earlier one was rejected, so without a validator exactly the first
// present credential is read, as before. finish shapes a credential for the
// provider it will be probed against.
//
// When every credential present is rejected the highest-precedence one is
// returned, because that is the one the agent would send; its source is then
// listed in RejectedSources too.
func resolveFirstParty(ctx context.Context, opts ResolveOptions, finish func(Credential) Credential) (Credential, bool) {
	var (
		first     Credential
		haveFirst bool
		rejected  []string
	)
	// try reports whether the walk stops at cred.
	try := func(cred Credential) (Credential, bool) {
		cred = finish(cred)
		if !haveFirst {
			first, haveFirst = cred, true
		}
		if opts.Validate != nil {
			if _, status := opts.Validate(ctx, cred); status == http.StatusUnauthorized {
				rejected = append(rejected, cred.Source)
				return Credential{}, false
			}
		}
		cred.RejectedSources = slices.Clone(rejected)
		return cred, true
	}
	for _, source := range firstPartySources {
		if secret := opts.env(source.env); secret != "" {
			if cred, ok := try(Credential{
				Kind: source.kind, Secret: secret, Source: source.env, Provider: ProviderFirstParty,
			}); ok {
				return cred, true
			}
		}
	}
	if !opts.DisableStoredCredentials {
		// Source 5: the subscription login, stored in the keychain on macOS
		// and in a plain file everywhere else.
		for _, read := range storedOAuthReaders(opts) {
			stored, source, ok := read(ctx, opts)
			if !ok {
				continue
			}
			if cred, ok := try(Credential{
				Kind: stored.kind, Secret: stored.token, Source: source, Provider: ProviderFirstParty,
				ExpiresAt: stored.expiresAt, Renewable: stored.renewable,
			}); ok {
				return cred, true
			}
		}
	}
	if !haveFirst {
		return Credential{}, false
	}
	first.RejectedSources = slices.Clone(rejected)
	return first, true
}

// storedOAuth is the subscription login Claude Code persisted: the access
// token plus the expiry metadata stored next to it. The refresh token itself
// is never retained; only its presence is recorded.
type storedOAuth struct {
	token     string
	kind      Kind
	expiresAt time.Time
	renewable bool
}

// storedOAuthReaders is the entire platform surface of this package, in
// ladder order.
//
// macOS keeps the subscription token in the keychain; Linux and Windows keep
// it in .credentials.json, and the Claude Code binary contains no reference to
// Windows Credential Manager, wincred, CredRead, or DPAPI — so there is no
// third storage backend to implement. On macOS an absent, locked, or denied
// keychain falls through to the file, which is the path the other two
// platforms always take, so non-Mac platforms exercise strictly less code
// rather than different code.
func storedOAuthReaders(opts ResolveOptions) []func(context.Context, ResolveOptions) (storedOAuth, string, bool) {
	var readers []func(context.Context, ResolveOptions) (storedOAuth, string, bool)
	if opts.goos() == "darwin" && opts.AllowKeychain {
		readers = append(readers, func(ctx context.Context, opts ResolveOptions) (storedOAuth, string, bool) {
			stored, ok := readKeychain(ctx, opts)
			return stored, "keychain", ok
		})
	}
	return append(readers, readCredentialsFile)
}

// readCredentialsFile reads ~/.claude/.credentials.json.
func readCredentialsFile(ctx context.Context, opts ResolveOptions) (stored storedOAuth, source string, ok bool) {
	_ = ctx
	dir, err := claudeConfigDir(opts)
	if err != nil {
		return storedOAuth{}, "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, ".credentials.json"))
	if err != nil {
		return storedOAuth{}, "", false
	}
	stored, ok = oauthTokenFromCredentialsJSON(data)
	if !ok {
		return storedOAuth{}, "", false
	}
	return stored, "credentials-file", true
}

// oauthTokenFromCredentialsJSON pulls the access token out of the credential
// file, tolerating both the nested and flat shapes Claude Code has written.
// The nested shape also records the access token's expiry (epoch milliseconds)
// and a refresh token, which together say whether an expired access token is
// one Claude Code will renew on its own.
func oauthTokenFromCredentialsJSON(data []byte) (storedOAuth, bool) {
	type oauthFields struct {
		AccessToken  string  `json:"accessToken"`
		RefreshToken string  `json:"refreshToken"`
		ExpiresAt    float64 `json:"expiresAt"`
	}
	var payload struct {
		ClaudeAiOauth oauthFields `json:"claudeAiOauth"`
		oauthFields
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return storedOAuth{}, false
	}
	for _, candidate := range []oauthFields{payload.ClaudeAiOauth, payload.oauthFields} {
		token := strings.TrimSpace(candidate.AccessToken)
		if token == "" {
			continue
		}
		stored := storedOAuth{token: token, kind: KindOAuthToken}
		if candidate.ExpiresAt > 0 {
			stored.expiresAt = time.UnixMilli(int64(candidate.ExpiresAt)).UTC()
		}
		stored.renewable = strings.TrimSpace(candidate.RefreshToken) != ""
		return stored, true
	}
	return storedOAuth{}, false
}

// claudeConfigDir resolves Claude Code's config directory, honoring the same
// overrides the CLI does. Relative directories are anchored to the launch cwd,
// rather than the daemon cwd, so readers and child commands agree.
func claudeConfigDir(opts ResolveOptions) (string, error) {
	dir := strings.TrimSpace(opts.ConfigDir)
	if dir == "" {
		dir = opts.env("CLAUDE_CONFIG_DIR")
	}
	if dir == "" {
		homeKey := "HOME"
		if opts.goos() == "windows" {
			homeKey = "USERPROFILE"
		}
		home := opts.env(homeKey)
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("agentcreds: resolve home directory: %w", err)
			}
		}
		dir = filepath.Join(home, ".claude")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(strings.TrimSpace(opts.WorkingDir), dir)
	}
	return filepath.Abs(dir)
}
