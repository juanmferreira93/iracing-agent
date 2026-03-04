package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/juanmferreira93/iracing-agent/internal/client"
	"github.com/juanmferreira93/iracing-agent/internal/config"
	"github.com/juanmferreira93/iracing-agent/internal/domain"
	"github.com/juanmferreira93/iracing-agent/internal/parser"
	"github.com/juanmferreira93/iracing-agent/internal/queue"
	"github.com/juanmferreira93/iracing-agent/internal/service"
	"github.com/juanmferreira93/iracing-agent/internal/state"
	"github.com/juanmferreira93/iracing-agent/internal/watcher"
)

const devTelemetryPath = "./dev-telemetry"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfgPath := os.Getenv("IRACING_AGENT_CONFIG")
	if cfgPath == "" {
		cfgPath = "config/agent.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	switch os.Args[1] {
	case "run":
		if err := run(cfg, os.Args[2:]); err != nil {
			log.Fatalf("run failed: %v", err)
		}
	case "doctor":
		if err := runDoctor(cfg, os.Args[2:]); err != nil {
			log.Fatalf("doctor failed: %v", err)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func run(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	logOnly := fs.Bool("log-only", false, "do not push to Rails; log payload to console")
	logsOnly := fs.Bool("logs-only", false, "alias for --log-only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	logOnlyEnabled := *logOnly || *logsOnly

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	idx, err := state.NewIndex(cfg.Agent.StateFile)
	if err != nil {
		return err
	}

	spool, err := queue.NewSpool(cfg.Agent.SpoolDir, cfg.Agent.MaxRetries)
	if err != nil {
		return err
	}

	watchPaths := effectiveWatchPaths(cfg, logOnlyEnabled)
	uploader := selectUploader(cfg, logOnlyEnabled)

	svc := service.New(
		cfg,
		watcher.NewFileWatcher(watchPaths),
		parser.NewIBTParser(),
		uploader,
		idx,
		spool,
	)

	if logOnlyEnabled {
		log.Printf("iracing-agent started in log-only mode (watching %d paths)", len(watchPaths))
	} else {
		log.Printf("iracing-agent started (watching %d paths)", len(watchPaths))
	}
	return svc.Run(ctx)
}

func runDoctor(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	logOnly := fs.Bool("log-only", false, "skip Rails checks; validate local runtime only")
	logsOnly := fs.Bool("logs-only", false, "alias for --log-only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	logOnlyEnabled := *logOnly || *logsOnly
	watchPaths := effectiveWatchPaths(cfg, logOnlyEnabled)

	if len(watchPaths) == 0 {
		return errors.New("agent.watch_paths cannot be empty")
	}

	for _, watchPath := range watchPaths {
		if _, err := os.Stat(watchPath); err != nil {
			return fmt.Errorf("watch path %s: %w", watchPath, err)
		}
	}

	if !logOnlyEnabled {
		rails := client.NewRailsClient(cfg.Rails)
		if err := rails.Ping(context.Background()); err != nil {
			return fmt.Errorf("rails api check failed: %w", err)
		}
	} else {
		log.Println("doctor: log-only mode enabled, skipping Rails connectivity check")
	}

	log.Println("doctor checks passed")
	return nil
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  iracing-agent run [--log-only|--logs-only]")
	fmt.Println("  iracing-agent doctor [--log-only|--logs-only]")
	fmt.Println("Config path via IRACING_AGENT_CONFIG (default: config/agent.yaml)")
	fmt.Println("In --log-only, dumps JSON files to IRACING_AGENT_JSON_DUMP_DIR (default: ./dev-output/parsed-json)")
}

func selectUploader(cfg config.Config, logOnly bool) interface {
	UploadTelemetry(context.Context, domain.UploadBundle) error
} {
	if logOnly {
		dumpDir := os.Getenv("IRACING_AGENT_JSON_DUMP_DIR")
		if dumpDir == "" {
			dumpDir = "./dev-output/parsed-json"
		}
		return client.NewLogUploader(dumpDir)
	}
	return client.NewRailsClient(cfg.Rails)
}

func effectiveWatchPaths(cfg config.Config, logOnly bool) []string {
	if logOnly {
		return []string{devTelemetryPath}
	}
	return cfg.Agent.WatchPaths
}
