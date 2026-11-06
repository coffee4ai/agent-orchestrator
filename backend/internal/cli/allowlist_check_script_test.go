package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runAllowlistCheck(t *testing.T, content string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not installed")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "scripts", "allowlist-check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", script, file).CombinedOutput()
	return string(out), err
}

func TestAllowlistCheckAcceptsGoodFile(t *testing.T) {
	out, err := runAllowlistCheck(t, `{"writer":["Bash(git add:*)","Bash(make:*)","Read(docs/x.md)"],"controller":["Bash(git push:*)","Bash(gh pr create:*)"],"orchestrator":[]}`)
	if err != nil {
		t.Fatalf("expected exit 0, got %v: %s", err, out)
	}
}

func TestAllowlistCheckRejectsBareStar(t *testing.T) {
	out, err := runAllowlistCheck(t, `{"writer":["Bash(git add*)"],"controller":[],"orchestrator":[]}`)
	if err == nil {
		t.Fatalf("expected non-zero exit: %s", out)
	}
	if !strings.Contains(out, "fixture.json") || !strings.Contains(out, "Bash(git add*)") {
		t.Fatalf("message must name file and entry: %s", out)
	}
}

func TestAllowlistCheckRejectsWriterPush(t *testing.T) {
	out, err := runAllowlistCheck(t, `{"writer":["Bash(git push:*)"],"controller":[],"orchestrator":[]}`)
	if err == nil {
		t.Fatalf("expected non-zero exit: %s", out)
	}
	if !strings.Contains(out, "fixture.json") || !strings.Contains(out, "Bash(git push:*)") {
		t.Fatalf("message must name file and entry: %s", out)
	}
}

func TestAllowlistCheckRejectsControllerMerge(t *testing.T) {
	out, err := runAllowlistCheck(t, `{"writer":[],"controller":["Bash(ao pr merge:*)"],"orchestrator":[]}`)
	if err == nil {
		t.Fatalf("expected non-zero exit: %s", out)
	}
}
