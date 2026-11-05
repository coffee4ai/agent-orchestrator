package claudecode

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/pkg/agentcreds"
)

const (
	defaultAuthCacheTTL  = 5 * time.Minute
	negativeAuthCacheTTL = 30 * time.Second
)

type authCacheEntry struct {
	result   agentcreds.Result
	storedAt time.Time
}

// maxAuthCacheEntries bounds the cache. The ladder holds at most a handful of
// credentials (three env variables, keychain, file), so a small cap keeps every
// one of them without letting launches with changing credentials grow it.
const maxAuthCacheEntries = 8

type authCache struct {
	mu      sync.RWMutex
	entries map[string]*authCacheEntry
	ttl     time.Duration
	now     func() time.Time
}

func newAuthCache(ttl time.Duration) *authCache {
	if ttl <= 0 {
		ttl = defaultAuthCacheTTL
	}
	return &authCache{ttl: ttl, now: time.Now, entries: make(map[string]*authCacheEntry)}
}

func authCacheKey(fingerprint string, provider agentcreds.Provider) string {
	return string(provider) + "\x00" + fingerprint
}

func (c *authCache) get(fingerprint string, provider agentcreds.Provider) (agentcreds.Result, bool) {
	if c == nil || fingerprint == "" {
		return agentcreds.Result{}, false
	}
	c.mu.RLock()
	entry := c.entries[authCacheKey(fingerprint, provider)]
	c.mu.RUnlock()
	if entry == nil {
		return agentcreds.Result{}, false
	}
	ttl := c.ttl
	if entry.result.State != agentcreds.StateValid {
		ttl = negativeAuthCacheTTL
	}
	if c.now().Sub(entry.storedAt) > ttl {
		return agentcreds.Result{}, false
	}
	return entry.result, true
}

func (c *authCache) put(result agentcreds.Result) {
	if c == nil || result.Fingerprint == "" {
		return
	}
	// A probe its caller abandoned says nothing about the credential or the
	// network. Cached, it would answer every other caller (model discovery
	// among them) with "context canceled" until the negative TTL ran out.
	if errors.Is(result.Err, context.Canceled) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= maxAuthCacheEntries {
		oldestKey, oldest := "", time.Time{}
		for key, entry := range c.entries {
			if oldestKey == "" || entry.storedAt.Before(oldest) {
				oldestKey, oldest = key, entry.storedAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[authCacheKey(result.Fingerprint, result.Provider)] = &authCacheEntry{result: result, storedAt: c.now()}
}

func (c *authCache) invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]*authCacheEntry)
	c.mu.Unlock()
	claudeAuthReports.invalidate()
}

// authReportCache remembers what `claude auth status` printed, keyed by
// authReportCacheKey, so chat starts do not run the CLI once for the launch
// check and again for model discovery. It shares the verdict cache's TTL and
// is dropped with it.
type authReportCache struct {
	mu      sync.Mutex
	entries map[string]authReportEntry
	ttl     time.Duration
	now     func() time.Time
}

type authReportEntry struct {
	report   claudeAuthReport
	storedAt time.Time
}

var claudeAuthReports = &authReportCache{ttl: defaultAuthCacheTTL, now: time.Now}

func (c *authReportCache) get(key string) (claudeAuthReport, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || c.now().Sub(entry.storedAt) > c.ttl {
		return claudeAuthReport{}, false
	}
	return entry.report, true
}

func (c *authReportCache) put(key string, report claudeAuthReport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]authReportEntry)
	}
	c.entries[key] = authReportEntry{report: report, storedAt: c.now()}
}

func (c *authReportCache) invalidate() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}
