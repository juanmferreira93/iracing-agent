# iracing-agent

Windows-first Go agent that scans iRacing `.ibt` files after sessions and uploads telemetry payloads to the Rails app `iracing-client`.

## Current MVP status

- Watches folders recursively for new `.ibt` files.
- Deduplicates already-ingested files with a local state index.
- Uploads JSON payloads to Rails with `X-API-Key` and `Idempotency-Key`.
- Stores failed uploads in a disk spool and retries with exponential backoff.
- Includes CLI commands: `run`, `doctor`.
- Parses telemetry into session metadata, laps, and full samples.

## Project structure

- `cmd/iracing-agent`: CLI entrypoint
- `internal/watcher`: `.ibt` discovery
- `internal/parser`: parser adapter
- `internal/client`: Rails HTTP client
- `internal/queue`: retry spool
- `internal/state`: deduplication index
- `internal/service`: ingest orchestration

## Quick start (Linux/macOS dev)

1. Copy config template:

   ```bash
   cp config/agent.example.yaml config/agent.yaml
   ```

2. Edit `config/agent.yaml`:
   - `agent.watch_paths` is optional. If omitted, the agent auto-detects the default telemetry folder from the OS.
   - Windows defaults to `Documents/iRacing/telemetry` (and also checks OneDrive Documents path when available).
   - Set `rails.base_url` and `rails.api_key`.

3. Build:

   ```bash
   go build ./...
   ```

4. Health checks:

   ```bash
   go run ./cmd/iracing-agent doctor
   ```

5. Run watcher loop:

   ```bash
   go run ./cmd/iracing-agent run
   ```

   For local development without Rails, use:

   ```bash
   go run ./cmd/iracing-agent run --logs-only
   ```

   In log-only mode it reads from `./dev-telemetry`, logs parsed previews, and saves full parsed JSON to `./dev-output/parsed-json`.
   You can override the dump folder with `IRACING_AGENT_JSON_DUMP_DIR`.

## Build for Windows

From Linux/macOS:

```bash
GOOS=windows GOARCH=amd64 go build -o dist/iracing-agent.exe ./cmd/iracing-agent
```

On Windows PowerShell:

```powershell
$env:IRACING_AGENT_CONFIG = "C:\path\to\agent.yaml"
.\iracing-agent.exe doctor
.\iracing-agent.exe run
```

Run as interactive system tray app on Windows:

```powershell
.\iracing-agent.exe tray
```

For development mode in tray (uses `./dev-telemetry` + JSON dump files):

```powershell
.\iracing-agent.exe tray --logs-only
```

Tray menu includes:
- Start Agent
- Stop Agent
- Run Doctor
- Open Config Folder
- Open JSON Dump Folder
- Exit

## Install on Windows startup

1. Build Windows binary (from Linux/macOS):

```bash
GOOS=windows GOARCH=amd64 go build -o dist/iracing-agent.exe ./cmd/iracing-agent
```

2. Copy to Windows machine:
   - `dist/iracing-agent.exe`
   - `scripts/install-windows-startup.ps1`
   - optional: your prepared `config/agent.yaml`

3. Run installer script in Windows PowerShell:

```powershell
.\install-windows-startup.ps1 -SourceExe .\iracing-agent.exe -Mode normal
```

For development mode (`--logs-only` + `dev-output/parsed-json` dumps):

```powershell
.\install-windows-startup.ps1 -SourceExe .\iracing-agent.exe -Mode log-only
```

If you already have a config file:

```powershell
.\install-windows-startup.ps1 -SourceExe .\iracing-agent.exe -SourceConfig .\agent.yaml -Mode normal
```

The script installs into `%LOCALAPPDATA%\iracing-agent` and registers a Scheduled Task named `iRacingAgent` to run at user logon.

## Rails endpoint contract (initial)

- `POST /api/v1/telemetry_uploads`
- Headers:
  - `X-API-Key: <key>`
  - `Idempotency-Key: <session external id>`
- JSON body:
  - `session`
  - `laps`
  - `samples`

## Next milestone

Replace metadata-only parser with full `.ibt` decoding into laps + samples while keeping the same upload contract.
