$ErrorActionPreference = "Stop"
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $root
try {
    $failed = $false
    $files = & git ls-files '*.md'
    foreach ($file in $files) {
        $content = Get-Content -LiteralPath $file -Raw
        foreach ($match in [regex]::Matches($content, '\[[^\]]*\]\(([^)]+)\)')) {
            $target = $match.Groups[1].Value.Trim('<', '>')
            $target = ($target -split '#', 2)[0].Replace('%20', ' ')
            if (-not $target -or $target.StartsWith('#') -or $target -match '^[a-zA-Z][a-zA-Z0-9+.-]*:') { continue }
            $parent = Split-Path $file -Parent
            $path = if ($parent) { Join-Path $parent $target } else { $target }
            if (-not (Test-Path -LiteralPath $path)) {
                Write-Error "$file`: missing local link target '$target'" -ErrorAction Continue
                $failed = $true
            }
        }
    }
    if ($failed) { exit 1 }
    "local Markdown links: PASS"
} finally {
    Pop-Location
}
