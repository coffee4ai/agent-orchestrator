package claudecode

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/pkg/agentcreds"
)

func TestAuthCacheRequiresTheCurrentCredentialAndSupportsInvalidation(t *testing.T) {
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	cache := newAuthCache(time.Minute)
	cache.now = func() time.Time { return now }
	credential := agentcreds.Credential{Kind: agentcreds.KindAPIKey, Secret: "working"}
	result := agentcreds.Result{
		State: agentcreds.StateValid, Provider: agentcreds.ProviderFirstParty,
		Fingerprint: credential.Fingerprint(), CheckedAt: now,
	}

	cache.put(result)
	if _, ok := cache.get(credential.Fingerprint(), agentcreds.ProviderFirstParty); !ok {
		t.Fatal("current credential should reuse its cached verdict")
	}
	other := agentcreds.Credential{Kind: agentcreds.KindAPIKey, Secret: "replacement"}
	if _, ok := cache.get(other.Fingerprint(), agentcreds.ProviderFirstParty); ok {
		t.Fatal("a replacement credential must not reuse the previous verdict")
	}
	if _, ok := cache.get(credential.Fingerprint(), agentcreds.ProviderBedrock); ok {
		t.Fatal("a different provider must not reuse the previous verdict")
	}
	cache.invalidate()
	if _, ok := cache.get(credential.Fingerprint(), agentcreds.ProviderFirstParty); ok {
		t.Fatal("runtime rejection must invalidate the cached verdict")
	}
}

func TestAuthCacheUsesShortTTLForNegativeAndInconclusiveResults(t *testing.T) {
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	cache := newAuthCache(time.Minute)
	cache.now = func() time.Time { return now }
	credential := agentcreds.Credential{
		Kind: agentcreds.KindAPIKey, Secret: "working", Provider: agentcreds.ProviderFirstParty,
	}
	fingerprint := credential.Fingerprint()

	for _, state := range []agentcreds.State{agentcreds.StateInvalid, agentcreds.StateUnknown} {
		cache.put(agentcreds.Result{State: state, Provider: agentcreds.ProviderFirstParty, Fingerprint: fingerprint})
		if _, ok := cache.get(fingerprint, agentcreds.ProviderFirstParty); !ok {
			t.Fatalf("%s result should be reused briefly", state)
		}
		now = now.Add(30*time.Second + time.Nanosecond)
		if _, ok := cache.get(fingerprint, agentcreds.ProviderFirstParty); ok {
			t.Fatalf("%s result should expire after 30 seconds", state)
		}
	}
	cache.put(agentcreds.Result{State: agentcreds.StateValid, Provider: agentcreds.ProviderFirstParty, Fingerprint: fingerprint})
	now = now.Add(time.Minute + time.Nanosecond)
	if _, ok := cache.get(fingerprint, agentcreds.ProviderFirstParty); ok {
		t.Fatal("expired verdict must not be reused")
	}
}

func TestAuthCacheDoesNotStoreResultsWithoutAStableFingerprint(t *testing.T) {
	cache := newAuthCache(time.Minute)
	cache.put(agentcreds.Result{State: agentcreds.StateInvalid, Provider: agentcreds.ProviderFirstParty})
	if len(cache.entries) != 0 {
		t.Fatal("a result without a credential fingerprint must not be cached")
	}
}

func TestAuthCacheDoesNotStoreAProbeItsCallerCanceled(t *testing.T) {
	cache := newAuthCache(time.Minute)
	credential := agentcreds.Credential{Kind: agentcreds.KindAPIKey, Secret: "working"}
	canceled := fmt.Errorf("Get \"https://api.anthropic.com/v1/models\": %w", context.Canceled)
	cache.put(agentcreds.Result{
		State: agentcreds.StateUnknown, Provider: agentcreds.ProviderFirstParty,
		Fingerprint: credential.Fingerprint(), Err: canceled,
	})
	if _, ok := cache.get(credential.Fingerprint(), agentcreds.ProviderFirstParty); ok {
		t.Fatal("an abandoned probe must not answer later callers")
	}
}

// A fallback verdict set is two credentials (the rejected one and the working
// one); the second must not evict the first or every launch probes both again.
func TestAuthCacheKeepsEveryCredentialOfTheLadder(t *testing.T) {
	cache := newAuthCache(time.Minute)
	rejected := agentcreds.Result{State: agentcreds.StateInvalid, Provider: agentcreds.ProviderFirstParty, Fingerprint: "aaaaaaaaaaaa", HTTPStatus: 401}
	working := agentcreds.Result{State: agentcreds.StateValid, Provider: agentcreds.ProviderFirstParty, Fingerprint: "bbbbbbbbbbbb", HTTPStatus: 200}
	cache.put(rejected)
	cache.put(working)
	if got, ok := cache.get(rejected.Fingerprint, rejected.Provider); !ok || got.HTTPStatus != 401 {
		t.Fatalf("rejected verdict evicted: %+v ok=%v", got, ok)
	}
	if _, ok := cache.get(working.Fingerprint, working.Provider); !ok {
		t.Fatal("working verdict missing")
	}
}

func TestLadderValidatorProbesARejectedCredentialOncePerTTL(t *testing.T) {
	clearClaudeCredentialEnv(t)
	InvalidateAuthCache()
	t.Cleanup(InvalidateAuthCache)
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, ".credentials.json"),
		[]byte(`{"claudeAiOauth":{"accessToken":"stored-token"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	gets := 0
	server := withStubValidator(t, func(w http.ResponseWriter, r *http.Request) {
		gets++
		if r.Header.Get("x-api-key") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	env := map[string]string{
		"ANTHROPIC_API_KEY": "stale-key", "ANTHROPIC_BASE_URL": server.URL, "CLAUDE_CONFIG_DIR": configDir,
	}
	for i := 0; i < 3; i++ {
		got := (&Plugin{}).RejectedCredentialEnv(context.Background(), t.TempDir(), env)
		if len(got) != 1 || got[0] != "ANTHROPIC_API_KEY" {
			t.Fatalf("launch %d: rejected = %v", i, got)
		}
	}
	if gets != 2 {
		t.Fatalf("GETs over three launches = %d, want 2 (one per credential, then cached)", gets)
	}
}
