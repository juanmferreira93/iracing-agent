//go:build windows

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/getlantern/systray"
	"github.com/juanmferreira93/iracing-agent/internal/config"
)

func runTray(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("tray", flag.ContinueOnError)
	logOnly := fs.Bool("log-only", false, "do not push to Rails; log payload to console")
	logsOnly := fs.Bool("logs-only", false, "alias for --log-only")
	if err := fs.Parse(args); err != nil {
		return err
	}

	logOnlyEnabled := *logOnly || *logsOnly

	var once sync.Once
	onReady := func() {
		systray.SetTitle("iRacing Agent")
		systray.SetTooltip("iRacing Agent telemetry service")

		status := systray.AddMenuItem("Status: Stopped", "Current agent status")
		status.Disable()
		systray.AddSeparator()

		start := systray.AddMenuItem("Start Agent", "Start telemetry ingest")
		stop := systray.AddMenuItem("Stop Agent", "Stop telemetry ingest")
		stop.Disable()
		doctor := systray.AddMenuItem("Run Doctor", "Run health checks")
		systray.AddSeparator()
		openConfig := systray.AddMenuItem("Open Config Folder", "Open config folder")
		openDumps := systray.AddMenuItem("Open JSON Dump Folder", "Open parsed JSON folder")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Exit", "Exit tray app")

		watchPaths := effectiveWatchPaths(cfg, logOnlyEnabled)
		modeLabel := "normal"
		if logOnlyEnabled {
			modeLabel = "log-only"
		}
		log.Printf("tray mode started (%s), watch paths: %v", modeLabel, watchPaths)

		var runCancel context.CancelFunc
		running := false

		setStopped := func() {
			running = false
			status.SetTitle("Status: Stopped")
			start.Enable()
			stop.Disable()
		}

		setRunning := func() {
			running = true
			status.SetTitle("Status: Running")
			start.Disable()
			stop.Enable()
		}

		go func() {
			for {
				select {
				case <-start.ClickedCh:
					if running {
						continue
					}

					svc, err := newIngestService(cfg, logOnlyEnabled)
					if err != nil {
						log.Printf("tray start error: %v", err)
						continue
					}

					ctx, cancel := context.WithCancel(context.Background())
					runCancel = cancel
					setRunning()

					go func() {
						if err := svc.Run(ctx); err != nil {
							log.Printf("tray service error: %v", err)
						}
						setStopped()
					}()

				case <-stop.ClickedCh:
					if runCancel != nil {
						runCancel()
						runCancel = nil
					}
					setStopped()

				case <-doctor.ClickedCh:
					doctorArgs := []string{}
					if logOnlyEnabled {
						doctorArgs = append(doctorArgs, "--log-only")
					}
					if err := runDoctor(cfg, doctorArgs); err != nil {
						log.Printf("doctor failed: %v", err)
					}

				case <-openConfig.ClickedCh:
					openPath(filepath.Dir(cfg.Agent.StateFile))

				case <-openDumps.ClickedCh:
					dumpDir := resolveDumpDir()
					_ = os.MkdirAll(dumpDir, 0o755)
					openPath(dumpDir)

				case <-quit.ClickedCh:
					if runCancel != nil {
						runCancel()
					}
					systray.Quit()
					return
				}
			}
		}()
	}

	onExit := func() {
		once.Do(func() {})
	}

	systray.Run(onReady, onExit)
	return nil
}

func openPath(path string) {
	if path == "" {
		return
	}
	if err := exec.Command("explorer.exe", path).Start(); err != nil {
		log.Printf("open path failed (%s): %v", path, err)
	}
}
