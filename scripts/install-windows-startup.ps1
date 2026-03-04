param(
  [Parameter(Mandatory = $true)]
  [string]$SourceExe,

  [string]$SourceConfig = "",

  [ValidateSet("normal", "log-only")]
  [string]$Mode = "normal",

  [string]$InstallDir = "$env:LOCALAPPDATA\iracing-agent",

  [string]$TaskName = "iRacingAgent"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -Path $SourceExe)) {
  throw "Source exe not found: $SourceExe"
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir "config") | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $InstallDir "dev-output\parsed-json") | Out-Null

$exePath = Join-Path $InstallDir "iracing-agent.exe"
Copy-Item -Path $SourceExe -Destination $exePath -Force

$configPath = Join-Path $InstallDir "config\agent.yaml"
if ($SourceConfig -and (Test-Path -Path $SourceConfig)) {
  Copy-Item -Path $SourceConfig -Destination $configPath -Force
} elseif (-not (Test-Path -Path $configPath)) {
  @"
agent:
  # Optional. In normal mode, defaults to OS iRacing telemetry folders if omitted.
  # watch_paths:
  #   - "C:/Users/<your-user>/Documents/iRacing/telemetry"
  scan_interval_seconds: 10
  state_file: "$InstallDir/config/agent-state.json"
  spool_dir: "$InstallDir/config/spool"
  max_retries: 8

rails:
  base_url: "http://localhost:3000"
  api_key: "replace-with-generated-api-key"
  upload_path: "/api/v1/telemetry_uploads"
  health_path: "/up"
"@ | Set-Content -Path $configPath -Encoding UTF8
}

$runScriptPath = Join-Path $InstallDir "run-agent.ps1"
$runArgs = "run"
if ($Mode -eq "log-only") {
  $runArgs = "run --logs-only"
}

@"
4ErrorActionPreference = "Stop"
4env:IRACING_AGENT_CONFIG = "$configPath"
4env:IRACING_AGENT_JSON_DUMP_DIR = "$InstallDir\dev-output\parsed-json"
& "$exePath" $runArgs
"@ | Set-Content -Path $runScriptPath -Encoding UTF8

$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$runScriptPath`""
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries

Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Description "iRacing Agent startup task" -Force | Out-Null
Start-ScheduledTask -TaskName $TaskName

Write-Host "Installed iRacing Agent"
Write-Host "  exe:    $exePath"
Write-Host "  config: $configPath"
Write-Host "  mode:   $Mode"
Write-Host "  task:   $TaskName"
Write-Host ""
Write-Host "Useful checks:"
Write-Host "  Get-ScheduledTask -TaskName $TaskName"
Write-Host "  Get-ScheduledTaskInfo -TaskName $TaskName"
Write-Host "  Start-ScheduledTask -TaskName $TaskName"
Write-Host "  Stop-ScheduledTask -TaskName $TaskName"
