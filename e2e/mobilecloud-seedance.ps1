<#!
.SYNOPSIS
  Runs a smoke E2E test through the gateway's Mobile Cloud Seedance channel.

The Mobile Cloud Bearer key is intentionally not accepted on the command
line. Configure it in the gateway channel first; this script only uses the
customer-facing New API token.
#>
[CmdletBinding()]
param(
  [string]$BaseUrl = "http://HOST:PORT",
  [string]$Token = "TOKEN",
  [string]$Model = "doubao-seedance-2.0",
  [string]$Prompt = "A calm cinematic aerial shot over a coastal city at sunrise",
  [int]$PollSeconds = 5,
  [int]$MaxPolls = 24
)

$ErrorActionPreference = "Stop"
if ($BaseUrl -match "HOST|PORT" -or $Token -eq "TOKEN") {
  throw "Set BaseUrl and Token placeholders before running the E2E test."
}
$BaseUrl = $BaseUrl.TrimEnd('/')
$headers = @{ Authorization = "Bearer $Token"; "Content-Type" = "application/json" }
$idempotencyKey = [guid]::NewGuid().ToString()
$body = @{ model = $Model; prompt = $Prompt; seconds = 5; size = "1280x720" } | ConvertTo-Json

Write-Host "Creating video task..."
try {
  $created = Invoke-RestMethod -Method Post -Uri "$BaseUrl/v1/videos" -Headers ($headers + @{ "Idempotency-Key" = $idempotencyKey }) -Body $body
} catch {
  throw "Create failed. Check the channel configuration and server logs (request id is returned in the response headers). $($_.Exception.Message)"
}
$taskId = $created.id
if ([string]::IsNullOrWhiteSpace($taskId)) { $taskId = $created.task_id }
if ([string]::IsNullOrWhiteSpace($taskId)) { throw "Create response did not contain a task id." }
Write-Host "Task created: $taskId"

for ($i = 1; $i -le $MaxPolls; $i++) {
  Start-Sleep -Seconds $PollSeconds
  try {
    $task = Invoke-RestMethod -Method Get -Uri "$BaseUrl/v1/videos/$taskId" -Headers @{ Authorization = "Bearer $Token" }
  } catch {
    Write-Warning "Poll $i failed; retrying: $($_.Exception.Message)"
    continue
  }
  $status = [string]$task.status
  if ([string]::IsNullOrWhiteSpace($status) -and $task.data) { $status = [string]$task.data.status }
  Write-Host "Poll ${i}: status=$status"
  if ($status -in @("completed", "SUCCESS")) {
    Write-Host "E2E PASS: task completed. Use the gateway content endpoint to download the video."
    exit 0
  }
  if ($status -in @("failed", "FAILURE")) {
    throw "E2E FAIL: provider task failed. Review the redacted task error in the admin logs."
  }
}
throw "E2E TIMEOUT: task did not reach a terminal state within the polling window."
