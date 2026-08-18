package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewServerRequiresAdminPassword(t *testing.T) {
	t.Setenv("WEBCODEX_ADMIN_PASSWORD", "")
	tempDir := t.TempDir()
	t.Setenv("WEBCODEX_DB_PATH", filepath.Join(tempDir, "test.db"))

	_, err := newServer()
	if err == nil {
		t.Fatal("newServer succeeded without required admin password")
	}
	if !strings.Contains(err.Error(), "WEBCODEX_ADMIN_PASSWORD") {
		t.Fatalf("error = %q, want WEBCODEX_ADMIN_PASSWORD is required", err)
	}
}

func TestSameAgentStreamReplacesPrevious(t *testing.T) {
	rt := newAgentRuntime("home")

	firstCtx, _, _ := rt.activateAgentStream(context.Background())
	secondCtx, second, replaced := rt.activateAgentStream(context.Background())
	defer rt.deactivateAgentStream(second)

	if !replaced {
		t.Fatal("second agent stream did not replace the first")
	}

	select {
	case <-firstCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("previous agent stream was not cancelled")
	}

	if secondCtx.Err() != nil {
		t.Fatal("new agent stream was cancelled")
	}
}

func TestDifferentAgentsCanHaveStreams(t *testing.T) {
	home := newAgentRuntime("home")
	work := newAgentRuntime("work")

	homeCtx, homeStream, replaced1 := home.activateAgentStream(context.Background())
	defer home.deactivateAgentStream(homeStream)

	if replaced1 {
		t.Fatal("home stream unexpectedly replaced an existing stream")
	}

	workCtx, workStream, replaced2 := work.activateAgentStream(context.Background())
	defer work.deactivateAgentStream(workStream)

	if replaced2 {
		t.Fatal("work stream replaced home stream")
	}

	if homeCtx.Err() != nil {
		t.Fatal("home stream was cancelled when work connected")
	}
	if workCtx.Err() != nil {
		t.Fatal("work stream was cancelled")
	}

	if !home.isOnline() || !work.isOnline() {
		t.Fatalf("expected both agents to be online: home=%t, work=%t", home.isOnline(), work.isOnline())
	}
}
