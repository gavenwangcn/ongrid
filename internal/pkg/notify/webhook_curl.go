package notify

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
)

const curlHTTPCodeMarker = "\n__ONGRID_HTTP_CODE__:"

// sendOnceViaCurl posts the webhook with the container's curl binary.
// Used only when net/http fails with transient network errors — curl has
// been observed to succeed from the same container/IP where Go gets RST.
func sendOnceViaCurl(ctx context.Context, log *slog.Logger, endpoint string, headers map[string]string, body []byte) error {
	curlPath, err := exec.LookPath("curl")
	if err != nil {
		return fmt.Errorf("curl fallback unavailable: %w", err)
	}
	args := []string{
		"-sS",
		"--max-time", "30",
		"-X", "POST",
		endpoint,
		"-H", "Content-Type: application/json",
		"-H", "Accept: */*",
		"--data-binary", "@-",
		"-w", curlHTTPCodeMarker + "%{http_code}",
	}
	for k, v := range headers {
		args = append(args, "-H", k+": "+v)
	}
	log.Info("webhook send curl fallback",
		slog.Int("body_bytes", len(body)),
	)
	cmd := exec.CommandContext(ctx, curlPath, args...)
	cmd.Stdin = bytes.NewReader(body)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("curl: %w stderr=%s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fmt.Errorf("curl: %w", err)
	}
	status, preview, err := parseCurlWebhookOutput(out)
	if err != nil {
		return err
	}
	log.Debug("webhook curl response",
		slog.Int("status", status),
		slog.String("response_preview", preview),
	)
	if status < 200 || status >= 300 {
		return fmt.Errorf("unexpected status: %d body=%s", status, preview)
	}
	if err := checkWebhookResponseBody(preview); err != nil {
		return err
	}
	return nil
}

func parseCurlWebhookOutput(out []byte) (status int, preview string, err error) {
	raw := string(out)
	idx := strings.LastIndex(raw, curlHTTPCodeMarker)
	if idx < 0 {
		return 0, "", fmt.Errorf("curl: missing http_code marker in output")
	}
	preview = strings.TrimSpace(raw[:idx])
	codeStr := strings.TrimSpace(raw[idx+len(curlHTTPCodeMarker):])
	status, err = strconv.Atoi(codeStr)
	if err != nil {
		return 0, preview, fmt.Errorf("curl: parse http_code %q: %w", codeStr, err)
	}
	if len(preview) > 400 {
		preview = preview[:400] + "…"
	}
	return status, preview, nil
}

// curlAvailable reports whether the runtime ships a curl binary (prod image does).
func curlAvailable() bool {
	_, err := exec.LookPath("curl")
	return err == nil
}
