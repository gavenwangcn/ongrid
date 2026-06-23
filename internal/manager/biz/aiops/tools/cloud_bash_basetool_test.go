package tools

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ongridio/ongrid/internal/manager/biz/aiops/tools/basetool"
)

type recProposer struct {
	gotSession string
	gotCmd     string
	gotCreds   []string
}

func (r *recProposer) Propose(_ context.Context, command string, credentials []string, sessionID string, _ uint64) (string, error) {
	r.gotSession = sessionID
	r.gotCmd = command
	r.gotCreds = credentials
	return "approval-1", nil
}

// TestCloudBash_ThreadsSessionID locks the HLD-019 regression: cloud_bash must
// thread the chat session id from ctx into Propose so the approval can resolve
// a per-session workspace at exec time. It was previously hardcoded to "",
// which silently dropped every command back into a throwaway temp dir.
func TestCloudBash_ThreadsSessionID(t *testing.T) {
	p := &recProposer{}
	tool := NewCloudBashTool(p, slog.Default())
	ctx := basetool.WithSessionID(context.Background(), "sess-xyz")
	if _, err := tool.InvokableRun(ctx, `{"command":"terraform version"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if p.gotSession != "sess-xyz" {
		t.Errorf("Propose sessionID = %q, want %q", p.gotSession, "sess-xyz")
	}
	if p.gotCmd != "terraform version" {
		t.Errorf("Propose command = %q", p.gotCmd)
	}
}

// Empty ctx (no session attached) → empty session id, not a panic. The
// executor then falls back to a transient dir (legacy behavior).
func TestCloudBash_NoSessionID(t *testing.T) {
	p := &recProposer{}
	tool := NewCloudBashTool(p, slog.Default())
	if _, err := tool.InvokableRun(context.Background(), `{"command":"echo hi"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if p.gotSession != "" {
		t.Errorf("Propose sessionID = %q, want empty", p.gotSession)
	}
}
