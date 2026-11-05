package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/aoagents/agent-orchestrator/backend/internal/daemonmeta"
)

func statusFlagsServer(t *testing.T, buildJSON string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/healthz":
			_, _ = fmt.Fprintf(w, `{"status":"ok","service":%q,"pid":%d}`, daemonmeta.ServiceName, os.Getpid())
		case "/readyz":
			_, _ = fmt.Fprintf(w, `{"status":"ready","service":%q,"pid":%d}`, daemonmeta.ServiceName, os.Getpid())
		case "/api/v1/system/build":
			_, _ = fmt.Fprint(w, buildJSON)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStatusPrintsTelemetryAndSentryFlags(t *testing.T) {
	cfg := setConfigEnv(t)
	srv := statusFlagsServer(t, `{"telemetryRemote":"off","sentryEnabled":false,"chatHibernation":false,"forkCommit":"abc1234"}`)
	writeRunFileFor(t, cfg, srv)

	out, errOut, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }}, "status")
	if err != nil {
		t.Fatalf("status failed: %v stderr=%s", err, errOut)
	}
	for _, want := range []string{"telemetry remote: off", "sentry: off (no DSN)", "chat hibernation: off", "fork commit: abc1234"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}

func TestStatusPrintsSentryOnAndRemoteTelemetry(t *testing.T) {
	cfg := setConfigEnv(t)
	srv := statusFlagsServer(t, `{"telemetryRemote":"posthog","sentryEnabled":true,"chatHibernation":true,"forkCommit":"abc1234"}`)
	writeRunFileFor(t, cfg, srv)

	out, _, err := executeCLI(t, Deps{ProcessAlive: func(int) bool { return true }}, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"telemetry remote: posthog", "sentry: on", "chat hibernation: on"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status output missing %q:\n%s", want, out)
		}
	}
}
