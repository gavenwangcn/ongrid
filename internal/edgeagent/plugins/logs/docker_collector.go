package logs

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ongridio/ongrid/internal/edgeagent/plugins"
)

const (
	dockerAPIVersion       = "v1.41"
	dockerReconcilePeriod  = 15 * time.Second
	dockerPushBatchWait    = 1 * time.Second
	dockerPushBatchMax     = 200
	dockerPositionsFile    = "docker-positions.json"
	dockerLogMultiplexHdr  = 8
)

type dockerContainer struct {
	ID   string `json:"Id"`
	Name string `json:"-"`
}

type dockerCollector struct {
	socket      string
	endpoint    string
	authUser    string
	authPass    string
	deviceID    uint64
	extraLabels map[string]string
	workDir     string
	log         *slog.Logger

	mu          sync.Mutex
	positions   map[string]int64 // container id -> last unix seconds
	followers   map[string]context.CancelFunc
	lastError   string
	state       string
	wantRunning bool
	cancelRun   context.CancelFunc
}

func newDockerCollector(workDir string, log *slog.Logger) *dockerCollector {
	if log == nil {
		log = slog.Default()
	}
	return &dockerCollector{
		workDir:   workDir,
		log:       log.With(slog.String("comp", "logs.docker_api")),
		positions: map[string]int64{},
		followers: map[string]context.CancelFunc{},
		state:     "stopped",
	}
}

func (d *dockerCollector) configure(cfg plugins.PluginConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.socket = dockerSocket(cfg.Spec)
	d.endpoint = cfg.Endpoint
	d.authUser = cfg.AuthUser
	d.authPass = cfg.AuthPass
	d.deviceID = cfg.EdgeID
	d.extraLabels = stringMap(cfg.Spec, specExtraLabels)
	if err := d.loadPositions(); err != nil {
		d.log.Warn("docker positions load failed; starting from tail",
			slog.Any("err", err))
	}
	d.log.Info("docker api collector configured",
		slog.String("socket", d.socket),
		slog.String("endpoint", d.endpoint),
		slog.Uint64("label_device_id", d.deviceID))
	return nil
}

func (d *dockerCollector) start(ctx context.Context) error {
	d.mu.Lock()
	if d.wantRunning {
		d.mu.Unlock()
		return nil
	}
	d.wantRunning = true
	runCtx, cancel := context.WithCancel(ctx)
	d.cancelRun = cancel
	d.state = "running"
	d.lastError = ""
	d.mu.Unlock()

	go d.run(runCtx)
	return nil
}

func (d *dockerCollector) stop(ctx context.Context) {
	d.mu.Lock()
	if !d.wantRunning {
		d.mu.Unlock()
		return
	}
	d.wantRunning = false
	cancel := d.cancelRun
	for id, c := range d.followers {
		c()
		delete(d.followers, id)
	}
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.mu.Lock()
	d.state = "stopped"
	d.mu.Unlock()
}

func (d *dockerCollector) health() (state string, lastErr string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.state, d.lastError
}

func (d *dockerCollector) run(ctx context.Context) {
	client := d.dockerHTTPClient()
	ticker := time.NewTicker(dockerReconcilePeriod)
	defer ticker.Stop()

	d.reconcile(ctx, client)

	for {
		select {
		case <-ctx.Done():
			d.stopAllFollowers()
			return
		case <-ticker.C:
			d.reconcile(ctx, client)
			if err := d.savePositions(); err != nil {
				d.log.Warn("docker positions save failed", slog.Any("err", err))
			}
		}
	}
}

func (d *dockerCollector) dockerHTTPClient() *http.Client {
	socket := d.socket
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
		// Follow=true on logs has no timeout; per-request handled in goroutines.
		Timeout: 0,
	}
}

func (d *dockerCollector) reconcile(ctx context.Context, client *http.Client) {
	containers, err := d.listRunningContainers(ctx, client)
	if err != nil {
		d.setError(fmt.Errorf("list containers: %w", err))
		return
	}
	d.clearError()

	want := make(map[string]dockerContainer, len(containers))
	for _, c := range containers {
		want[c.ID] = c
	}

	d.mu.Lock()
	// Stop followers for removed containers.
	for id, cancel := range d.followers {
		if _, ok := want[id]; !ok {
			cancel()
			delete(d.followers, id)
		}
	}
	// Start followers for new containers.
	for id, c := range want {
		if _, ok := d.followers[id]; ok {
			continue
		}
		followCtx, cancel := context.WithCancel(ctx)
		d.followers[id] = cancel
		go d.followContainer(followCtx, client, c)
	}
	d.mu.Unlock()
}

func (d *dockerCollector) stopAllFollowers() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, cancel := range d.followers {
		cancel()
		delete(d.followers, id)
	}
}

func (d *dockerCollector) listRunningContainers(ctx context.Context, client *http.Client) ([]dockerContainer, error) {
	url := fmt.Sprintf("http://docker/%s/containers/json?filters=%s",
		dockerAPIVersion,
		`{"status":["running"]}`,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("docker api %s: %s", resp.Status, string(b))
	}
	var raw []struct {
		ID    string   `json:"Id"`
		Names []string `json:"Names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]dockerContainer, 0, len(raw))
	for _, r := range raw {
		name := r.ID
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		out = append(out, dockerContainer{ID: r.ID, Name: name})
	}
	return out, nil
}

func (d *dockerCollector) followContainer(ctx context.Context, client *http.Client, c dockerContainer) {
	d.log.Info("docker log follow started",
		slog.String("container_id", shortID(c.ID)),
		slog.String("container_name", c.Name))

	for {
		if ctx.Err() != nil {
			return
		}
		err := d.streamContainerLogs(ctx, client, c)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			d.log.Warn("docker log follow ended",
				slog.String("container_id", shortID(c.ID)),
				slog.String("container_name", c.Name),
				slog.Any("err", err))
			d.setError(err)
		}
		// Backoff before reconnect (container still running).
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (d *dockerCollector) streamContainerLogs(ctx context.Context, client *http.Client, c dockerContainer) error {
	since := d.getSince(c.ID)
	q := fmt.Sprintf("follow=true&stdout=true&stderr=true&timestamps=true&tail=100")
	if since > 0 {
		q += fmt.Sprintf("&since=%d", since)
	}
	url := fmt.Sprintf("http://docker/%s/containers/%s/logs?%s", dockerAPIVersion, c.ID, q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker logs %s: %s", resp.Status, string(b))
	}

	pusher := newLokiPusher(d.endpoint, d.authUser, d.authPass)
	batcher := newLogBatcher(pusher, dockerPushBatchWait, dockerPushBatchMax)

	err = readDockerMultiplexedLogs(resp.Body, func(stream string, ts time.Time, line string) error {
		if line == "" {
			return nil
		}
		d.updateSince(c.ID, ts)
		labels := d.baseLabels(c, stream)
		batcher.add(labels, ts, line)
		if batcher.shouldFlush() {
			return batcher.flush(ctx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return batcher.flush(ctx)
}

func (d *dockerCollector) baseLabels(c dockerContainer, stream string) map[string]string {
	labels := map[string]string{
		"device_id":     fmt.Sprintf("%d", d.deviceID),
		"ongrid_source": "docker_api",
		"container":     c.Name,
		"container_id":  shortID(c.ID),
		"stream":        stream,
	}
	for k, v := range d.extraLabels {
		labels[k] = v
	}
	return labels
}

func readDockerMultiplexedLogs(r io.Reader, onLine func(stream string, ts time.Time, line string) error) error {
	br := bufio.NewReader(r)
	var lineBuf strings.Builder
	currentStream := "stdout"

	flushLine := func() error {
		if lineBuf.Len() == 0 {
			return nil
		}
		raw := lineBuf.String()
		lineBuf.Reset()
		ts, msg := parseDockerTimestampLine(raw)
		return onLine(currentStream, ts, msg)
	}

	for {
		header, err := br.Peek(dockerLogMultiplexHdr)
		if err != nil {
			if err == io.EOF {
				if err := flushLine(); err != nil {
					return err
				}
				return nil
			}
			return err
		}
		if header[0] == 1 || header[0] == 2 {
			if _, err := io.ReadFull(br, header); err != nil {
				return err
			}
			size := binary.BigEndian.Uint32(header[4:8])
			if header[0] == 1 {
				currentStream = "stdout"
			} else {
				currentStream = "stderr"
			}
			payload := make([]byte, size)
			if _, err := io.ReadFull(br, payload); err != nil {
				return err
			}
			for i, b := range payload {
				if b == '\n' {
					if err := flushLine(); err != nil {
						return err
					}
					continue
				}
				if i == 0 && lineBuf.Len() > 0 {
					// continuation of previous partial line
				}
				lineBuf.WriteByte(b)
			}
			continue
		}
		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				lineBuf.WriteString(strings.TrimRight(line, "\r\n"))
				return flushLine()
			}
			return err
		}
		lineBuf.WriteString(strings.TrimRight(line, "\r\n"))
		if err := flushLine(); err != nil {
			return err
		}
	}
}

func parseDockerTimestampLine(line string) (time.Time, string) {
	if len(line) < 20 || line[0] != '2' {
		return time.Now(), line
	}
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return time.Now(), line
	}
	ts, err := time.Parse(time.RFC3339Nano, line[:sp])
	if err != nil {
		return time.Now(), line
	}
	return ts, line[sp+1:]
}

type logBatcher struct {
	pusher   *lokiPusher
	wait     time.Duration
	maxLines int
	streams  map[string]*lokiStream
	count    int
	lastPush time.Time
}

func newLogBatcher(p *lokiPusher, wait time.Duration, max int) *logBatcher {
	return &logBatcher{
		pusher:   p,
		wait:     wait,
		maxLines: max,
		streams:  map[string]*lokiStream{},
	}
}

func (b *logBatcher) add(labels map[string]string, ts time.Time, line string) error {
	key := labelsKey(labels)
	st, ok := b.streams[key]
	if !ok {
		st = &lokiStream{Stream: labels}
		b.streams[key] = st
	}
	st.Values = append(st.Values, [2]string{
		fmt.Sprintf("%d", ts.UnixNano()),
		line,
	})
	b.count++
	return nil
}

func (b *logBatcher) shouldFlush() bool {
	return b.count >= b.maxLines
}

func (b *logBatcher) flush(ctx context.Context) error {
	if len(b.streams) == 0 {
		return nil
	}
	out := make([]lokiStream, 0, len(b.streams))
	for _, s := range b.streams {
		out = append(out, *s)
	}
	err := b.pusher.push(ctx, out)
	b.streams = map[string]*lokiStream{}
	b.count = 0
	b.lastPush = time.Now()
	return err
}

func labelsKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
		b.WriteByte('|')
	}
	return b.String()
}

func sortStrings(a []string) {
	// tiny sort for label key stability
	for i := 0; i < len(a); i++ {
		for j := i + 1; j < len(a); j++ {
			if a[j] < a[i] {
				a[i], a[j] = a[j], a[i]
			}
		}
	}
}

func (d *dockerCollector) getSince(id string) int64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.positions[id]
}

func (d *dockerCollector) updateSince(id string, ts time.Time) {
	sec := ts.Unix()
	d.mu.Lock()
	if sec > d.positions[id] {
		d.positions[id] = sec
	}
	d.mu.Unlock()
}

func (d *dockerCollector) loadPositions() error {
	path := filepath.Join(d.workDir, dockerPositionsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var pos map[string]int64
	if err := json.Unmarshal(data, &pos); err != nil {
		return err
	}
	d.positions = pos
	return nil
}

func (d *dockerCollector) savePositions() error {
	d.mu.Lock()
	data, err := json.Marshal(d.positions)
	d.mu.Unlock()
	if err != nil {
		return err
	}
	path := filepath.Join(d.workDir, dockerPositionsFile)
	return os.WriteFile(path, data, 0o600)
}

func (d *dockerCollector) setError(err error) {
	d.mu.Lock()
	d.lastError = err.Error()
	d.state = "crashed"
	d.mu.Unlock()
}

func (d *dockerCollector) clearError() {
	d.mu.Lock()
	if d.lastError != "" {
		d.lastError = ""
	}
	if d.wantRunning {
		d.state = "running"
	}
	d.mu.Unlock()
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
