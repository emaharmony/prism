param(
    [switch]$Verify,
    [switch]$Race,
    [string]$Output = ""
)

$ErrorActionPreference = "Continue"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $root
try {
    if (-not $env:GOCACHE) { $env:GOCACHE = Join-Path $root ".cache/go-build" }
    New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null
    $lines = [System.Collections.Generic.List[string]]::new()
    $failed = $false
    function Add([string]$line) { $script:lines.Add($line) }
    function Run([string]$name, [scriptblock]$command) {
        $watch = [Diagnostics.Stopwatch]::StartNew()
        & $command *> $null
        $code = $LASTEXITCODE
        $watch.Stop()
        $status = if ($code -eq 0) { "PASS" } else { "FAIL" }
        if ($code -ne 0) { $script:failed = $true }
        Add "| $name | $status | $([math]::Round($watch.Elapsed.TotalSeconds, 2))s |"
    }

    $packages = @(go list ./... 2>$null).Count
    $testFiles = @(git ls-files "*_test.go").Count
    $goTests = 0
    $benchmarks = 0
    foreach ($file in git ls-files "*_test.go") {
        $goTests += @(Select-String -Path $file -Pattern '^func Test[A-Za-z0-9_]+').Count
        $benchmarks += @(Select-String -Path $file -Pattern '^func Benchmark[A-Za-z0-9_]+').Count
    }
    $pythonTests = @(Get-ChildItem remembrance,sdk -Recurse -Filter *.py -ErrorAction SilentlyContinue |
        Select-String -Pattern '^\s*(async\s+)?def test_').Count

    Add "# Prism quality report"
    Add ""
    Add "Generated: $(Get-Date -Format o)"
    Add ""
    Add "| Metric | Verified value |"
    Add "|---|---:|"
    Add "| Go packages | $packages |"
    Add "| Go test files | $testFiles |"
    Add "| Go tests | $goTests |"
    Add "| Go benchmarks | $benchmarks |"
    Add "| Python tests | $pythonTests |"
    Add ""
    Add "| Check | Status | Duration |"
    Add "|---|---|---:|"
    if ($Verify) {
        Run "gofmt" {
            $unformatted = @(gofmt -l (git ls-files "*.go"))
            $global:LASTEXITCODE = if ($unformatted.Count -eq 0) { 0 } else { 1 }
        }
        Run "go vet ./..." { go vet ./... }
        Run "go build ./..." { go build ./... }
        Run "go test ./... -count=1" { go test ./... -count=1 }
        Run "coverage gates" { & (Join-Path $PSScriptRoot "coverage-gate.ps1") -Output (Join-Path $root "coverage-gate.md") }
        if ($Race) { Run "go test -race ./... -count=1" { go test -race ./... -count=1 } }
    } else {
        Add "| verification | SKIPPED | run with -Verify |"
    }
    $clean = [string]::IsNullOrWhiteSpace((git status --porcelain --untracked-files=no))
    Add "| tracked worktree clean | $(if ($clean) {'PASS'} else {'FAIL'}) | n/a |"

    $text = $lines -join [Environment]::NewLine
    if ($Output) { Set-Content -LiteralPath $Output -Value $text -Encoding utf8 }
    $text
    if ($failed) { exit 1 }
} finally {
    Pop-Location
}
