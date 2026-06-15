package logs

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/ongridio/ongrid/internal/edgeagent/plugins"
)

// Name is the OTel signal name used as plugin identifier.
const Name = "logs"

// plugin runs promtail (journald / file_paths) and optionally a Docker API
// log collector (docker.sock — same permission chain as docker logs -f).
type plugin struct {
	sub    *plugins.SubprocessPlugin
	docker *dockerCollector
	log    *slog.Logger

	mu     sync.Mutex
	cfg    plugins.PluginConfig
	health plugins.PluginHealth
}

// New constructs the logs plugin.
func New(binDir, workDir string, log *slog.Logger) plugins.Plugin {
	if log == nil {
		log = slog.Default()
	}
	work := filepath.Join(workDir, Name)
	p := &plugin{
		docker: newDockerCollector(work, log),
		log:    log.With(slog.String("plugin", Name)),
		health: plugins.PluginHealth{
			Name:      Name,
			State:     plugins.StateStopped,
			UpdatedAt: time.Now(),
		},
	}

	renderWithLog := func(cfg plugins.PluginConfig) ([]byte, error) {
		body, err := render(cfg)
		if err != nil {
			p.log.Warn("logs: promtail config render failed",
				slog.Uint64("label_device_id", cfg.EdgeID),
				slog.String("endpoint", cfg.Endpoint),
				slog.Any("err", err))
			return nil, err
		}
		p.log.Info("logs: promtail config rendered",
			slog.Uint64("label_device_id", cfg.EdgeID),
			slog.String("endpoint", cfg.Endpoint),
			slog.Bool("enable_docker_api", enableDockerAPI(cfg.Spec)),
			slog.Bool("needs_promtail", needsPromtail(cfg.Spec)))
		return body, nil
	}

	p.sub = plugins.NewSubprocess(plugins.SubprocessOpts{
		Name:         Name,
		Binary:       filepath.Join(binDir, "promtail"),
		WorkDir:      work,
		ConfigFile:   filepath.Join(work, "promtail.yaml"),
		ConfigRender: renderWithLog,
		Args: func(_ plugins.PluginConfig, configFile string) []string {
			return []string{
				"-config.file=" + configFile,
				"-positions.file=" + filepath.Join(filepath.Dir(configFile), "positions.yaml"),
			}
		},
		Log: log,
	})

	return p
}

func (p *plugin) Name() string { return Name }

func (p *plugin) Configure(cfg plugins.PluginConfig) error {
	if cfg.Endpoint == "" {
		return fmt.Errorf("logs plugin: endpoint required")
	}
	if cfg.EdgeID == 0 {
		return fmt.Errorf("logs plugin: device_id required")
	}
	if err := p.docker.configure(cfg); err != nil {
		return err
	}
	if needsPromtail(cfg.Spec) {
		if err := p.sub.Configure(cfg); err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()
	return nil
}

func (p *plugin) Start(ctx context.Context) error {
	p.mu.Lock()
	cfg := p.cfg
	p.mu.Unlock()

	if needsPromtail(cfg.Spec) {
		if err := p.sub.Start(ctx); err != nil {
			return err
		}
	}
	if enableDockerAPI(cfg.Spec) {
		if err := p.docker.start(ctx); err != nil {
			if needsPromtail(cfg.Spec) {
				stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				_ = p.sub.Stop(stopCtx)
				cancel()
			}
			return err
		}
	}
	p.setState(plugins.StateRunning, nil)
	return nil
}

func (p *plugin) Stop(ctx context.Context) error {
	p.docker.stop(ctx)
	p.mu.Lock()
	cfg := p.cfg
	p.mu.Unlock()
	if needsPromtail(cfg.Spec) {
		stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := p.sub.Stop(stopCtx)
		cancel()
		if err != nil {
			p.setState(plugins.StateCrashed, err)
			return err
		}
	}
	p.setState(plugins.StateStopped, nil)
	return nil
}

func (p *plugin) HealthSnapshot() plugins.PluginHealth {
	p.mu.Lock()
	cfg := p.cfg
	p.mu.Unlock()

	h := plugins.PluginHealth{
		Name:      Name,
		State:     plugins.StateStopped,
		UpdatedAt: time.Now(),
	}
	running := false
	crashed := false

	if needsPromtail(cfg.Spec) {
		sub := p.sub.HealthSnapshot()
		h.PID = sub.PID
		h.RestartCount = sub.RestartCount
		h.StartedAt = sub.StartedAt
		if sub.State == plugins.StateRunning {
			running = true
		}
		if sub.State == plugins.StateCrashed {
			crashed = true
			h.LastError = sub.LastError
		}
	}
	if enableDockerAPI(cfg.Spec) {
		state, errMsg := p.docker.health()
		if state == "running" {
			running = true
		}
		if errMsg != "" {
			crashed = true
			h.LastError = errMsg
		}
	}
	switch {
	case crashed:
		h.State = plugins.StateCrashed
	case running:
		h.State = plugins.StateRunning
	default:
		h.State = plugins.StateStopped
	}
	return h
}

func (p *plugin) setState(st plugins.PluginState, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.health.State = st
	p.health.UpdatedAt = time.Now()
	if err != nil {
		p.health.LastError = err.Error()
	} else if st == plugins.StateRunning {
		p.health.LastError = ""
	}
}
