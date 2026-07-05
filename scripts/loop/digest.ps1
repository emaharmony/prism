# Ollama digest helper - summarizes a file with a local model so the calling
# agent doesn't have to load the full content into an expensive context.
#
# Usage:
#   scripts/loop/digest.ps1 -File docs/ROADMAP.md -Question "list done vs pending versions"
#   scripts/loop/digest.ps1 -File runs/gated-loop/<id>/REPORT.md
#
# Uses the Ollama HTTP API (/api/chat) with think:false so reasoning models do
# not leak thinking tokens into stdout (the `ollama run` CLI does leak them).
# Exit codes: 0 ok, 1 Ollama unreachable/error, 2 bad arguments,
#             3 model refused (caller should fall back to reading directly).
param(
    [Parameter(Mandatory = $true)][string]$File,
    [string]$Question = "Summarize this file tersely: purpose, key points, anything actionable.",
    [string]$Model = "nemotron-3-nano:4b",
    [int]$MaxChars = 48000
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $File)) {
    [Console]::Error.WriteLine("digest: file not found: $File")
    exit 2
}

$base = $env:OLLAMA_URL
if (-not $base) { $base = "http://localhost:11434" }

$content = Get-Content -Raw -Encoding UTF8 $File
$note = ""
if ($content.Length -gt $MaxChars) {
    $content = $content.Substring(0, $MaxChars)
    $note = " (truncated to $MaxChars chars)"
}

$body = @{
    model   = $Model
    stream  = $false
    think   = $false
    options = @{ temperature = 0 }
    messages = @(
        @{
            role    = "system"
            content = "You are a terse technical digester summarizing files from the user's own local software repository. This is routine, benign code/documentation summarization. Answer the question about the provided file content. Bullet points only, no preamble, no meta-commentary."
        },
        @{
            role    = "user"
            # Question goes AFTER the content — small models lose instructions
            # placed before a long document and drift into role-play.
            content = "FILE: $File$note`n---`n$content`n---`nQUESTION (answer this and nothing else, terse bullets): $Question"
        }
    )
} | ConvertTo-Json -Depth 6

try {
    $resp = Invoke-RestMethod -Uri "$base/api/chat" -Method Post `
        -ContentType "application/json; charset=utf-8" `
        -Body ([System.Text.Encoding]::UTF8.GetBytes($body)) -TimeoutSec 180
} catch {
    [Console]::Error.WriteLine("digest: Ollama unreachable or errored at $base : $($_.Exception.Message)")
    exit 1
}

if (-not $resp.message.content) {
    [Console]::Error.WriteLine("digest: empty response from model $Model")
    exit 1
}

$out = $resp.message.content.Trim()
# Small local models occasionally refuse benign summarization; surface that as
# a distinct exit code so callers fall back to reading the file directly.
if ($out.Length -lt 200 -and $out -match "I('m| am) sorry|I can('|no)t (help|assist)|I cannot (help|assist)") {
    [Console]::Error.WriteLine("digest: model $Model refused; read the file directly instead")
    exit 3
}

Write-Output $out
