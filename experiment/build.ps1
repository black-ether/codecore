param([string]$Artifacts, [switch]$Contextual)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if (!$Artifacts) { $Artifacts = Join-Path $root 'testdb/experiment-artifacts' }
$Artifacts = [IO.Path]::GetFullPath($Artifacts)
$bin = Join-Path $Artifacts 'bin'
$cache = Join-Path $Artifacts 'go-cache'
$manifest = if ($Contextual) { 'contextual_stages.json' } else { 'stages.json' }
$stages = Get-Content -LiteralPath (Join-Path $PSScriptRoot $manifest) -Raw | ConvertFrom-Json
New-Item -ItemType Directory -Force $bin, $cache | Out-Null

$previousCache = $env:GOCACHE
try {
    $env:GOCACHE = $cache
    for ($index = 0; $index -lt $stages.Count; $index++) {
        $stage = $stages[$index]
        $name = if ($Contextual) { $stage.name } else { "stage$($index + 1)-$($stage.name)" }
        $checkout = Join-Path $Artifacts $name
        if (!(Test-Path -LiteralPath $checkout)) {
            git -C $root worktree add --detach $checkout $stage.commit
            if ($LASTEXITCODE -ne 0) { throw "Cannot create detached worktree for $name" }
        }
        $head = git -C $checkout rev-parse HEAD
        if ($LASTEXITCODE -ne 0 -or $head -ne $stage.commit) { throw "Unexpected checkout for $name" }
        $status = git -C $checkout status --porcelain
        if ($LASTEXITCODE -ne 0 -or $status) { throw "Expected clean checkout for $name" }
        Push-Location -LiteralPath $checkout
        try {
            go test ./...
            if ($LASTEXITCODE -ne 0) { throw "Tests failed for $name" }
            go vet ./...
            if ($LASTEXITCODE -ne 0) { throw "Vet failed for $name" }
            go build -o (Join-Path $bin "codecore-$name.exe") .
            if ($LASTEXITCODE -ne 0) { throw "Build failed for $name" }
        } finally {
            Pop-Location
        }
    }
} finally {
    $env:GOCACHE = $previousCache
}
