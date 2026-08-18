// Command agent keeps an outbound connection to the gate and proxies calls to Codex MCP.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func findCodexMCP() (string, []string) {
	defaultArgs := []string{
		"-c", `approval_policy="never"`,
		"-c", `sandbox_mode="danger-full-access"`,
	}

	if custom := env("WEBCODEX_CODEX_MCP_CMD", ""); custom != "" {
		parts := strings.Fields(custom)
		if len(parts) > 0 {
			return parts[0], parts[1:]
		}
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidates := []string{
			filepath.Join(exeDir, "codex-mcp-server.exe"),
			filepath.Join(exeDir, "codex-mcp-server"),
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c, defaultArgs
			}
		}
	}

	relCandidates := []string{
		"codex-mcp-server.exe",
		"codex-mcp-server",
		"third_party/codex/codex-rs/target/debug/codex-mcp-server.exe",
		"third_party/codex/codex-rs/target/debug/codex-mcp-server",
		"third_party/codex/codex-rs/target/release/codex-mcp-server.exe",
		"third_party/codex/codex-rs/target/release/codex-mcp-server",
	}
	for _, c := range relCandidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, defaultArgs
		}
	}

	if p, err := exec.LookPath("codex"); err == nil {
		return p, append([]string{"mcp-server"}, defaultArgs...)
	}
	if p, err := exec.LookPath("codex.exe"); err == nil {
		return p, append([]string{"mcp-server"}, defaultArgs...)
	}

	if runtime.GOOS == "windows" {
		return "codex-mcp-server.exe", defaultArgs
	}
	return "third_party/codex/codex-rs/target/debug/codex-mcp-server", defaultArgs
}

func main() {
	gateURL := strings.TrimRight(env("WEBCODEX_GATE_URL", ""), "/")
	token := env("WEBCODEX_AGENT_TOKEN", "")
	binary, args := findCodexMCP()

	if gateURL == "" || token == "" {
		log.Fatal("WEBCODEX_GATE_URL and WEBCODEX_AGENT_TOKEN are required")
	}

	log.Printf("starting agent with binary: %s, args: %v", binary, args)
	mcp, err := startMCP(context.Background(), binary, args)
	if err != nil {
		log.Fatalf("start codex mcp (%s): %v", binary, err)
	}
	if err := mcp.initialize(context.Background()); err != nil {
		log.Fatalf("initialize codex mcp: %v", err)
	}

	client := &http.Client{}
	for {
		if err := streamOnce(context.Background(), client, gateURL, token, mcp); err != nil {
			log.Printf("stream: %v", err)
			time.Sleep(time.Second)
		}
	}
}
