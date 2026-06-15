package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// lokiStream is one stream block in the Loki push API.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiPusher struct {
	endpoint string
	user     string
	pass     string
	client   *http.Client
}

func newLokiPusher(endpoint, user, pass string) *lokiPusher {
	return &lokiPusher{
		endpoint: endpoint,
		user:     user,
		pass:     pass,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *lokiPusher) push(ctx context.Context, streams []lokiStream) error {
	if len(streams) == 0 {
		return nil
	}
	body, err := json.Marshal(map[string][]lokiStream{"streams": streams})
	if err != nil {
		return fmt.Errorf("marshal loki push: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Scope-OrgID", "ongrid")
	if p.user != "" {
		req.SetBasicAuth(p.user, p.pass)
	} else if p.pass != "" {
		req.Header.Set("Authorization", "Bearer "+p.pass)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("loki push %s: %s", resp.Status, string(slurp))
	}
	return nil
}
