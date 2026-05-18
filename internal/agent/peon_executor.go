package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/castleops/client/internal/api"
)

// executePeon downloads the peon entrypoint script and runs it with the appropriate interpreter.
func (a *Agent) executePeon(ctx context.Context, payload *api.RunPeonPayload) (string, error) {
	if payload.URL == "" || payload.Entry == "" {
		return "", fmt.Errorf("run_peon payload missing url or entry")
	}

	rawURL := buildPeonRawURL(payload.URL, payload.Entry)
	a.logger.Info().Str("url", rawURL).Str("type", payload.Type).Msg("Downloading peon script")

	scriptPath, cleanup, err := downloadPeonScript(ctx, rawURL, payload.Entry)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer cleanup()

	return runPeonScript(ctx, scriptPath, payload.Type, payload.Environment)
}

// buildPeonRawURL converts a GitHub repo URL + entry path to a raw.githubusercontent.com URL.
func buildPeonRawURL(repoURL, entry string) string {
	u := strings.TrimSuffix(repoURL, "/")
	u = strings.Replace(u, "https://github.com/", "https://raw.githubusercontent.com/", 1)
	return u + "/main/" + entry
}

// downloadPeonScript fetches the script to a temp file and returns its path plus a cleanup func.
func downloadPeonScript(ctx context.Context, rawURL, entry string) (path string, cleanup func(), err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "CastleOps-Client/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}

	ext := ""
	if idx := strings.LastIndex(entry, "."); idx >= 0 {
		ext = entry[idx:]
	}

	tmp, err := os.CreateTemp("", "peon-*"+ext)
	if err != nil {
		return "", nil, err
	}

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", nil, err
	}
	tmp.Close()

	if runtime.GOOS != "windows" {
		os.Chmod(tmp.Name(), 0700) //nolint:errcheck
	}

	return tmp.Name(), func() { os.Remove(tmp.Name()) }, nil //nolint:errcheck
}

// runPeonScript executes the downloaded script using the interpreter matching scriptType.
// Supported types: powershell / ps1, python / python3 / py, bash / sh.
func runPeonScript(ctx context.Context, scriptPath, scriptType string, env map[string]string) (string, error) {
	var cmd *exec.Cmd

	switch strings.ToLower(scriptType) {
	case "powershell", "ps1":
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "powershell.exe", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
		} else {
			// pwsh (PowerShell Core) on non-Windows
			cmd = exec.CommandContext(ctx, "pwsh", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
		}
	case "python", "python3", "py":
		cmd = exec.CommandContext(ctx, "python3", scriptPath)
	case "bash", "sh":
		cmd = exec.CommandContext(ctx, "bash", scriptPath)
	default:
		return "", fmt.Errorf("unsupported script type %q (supported: powershell, python, bash)", scriptType)
	}

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("script failed: %w", err)
	}
	return output, nil
}
