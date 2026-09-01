# Open the tracker web UI by reading the dynamic port from .codraft/tracker.port.
$PortFileName = "tracker.port"
$CodraftDir = ".codraft"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Resolve-Path (Join-Path $scriptDir "..")
$portFile = Join-Path (Join-Path $projectRoot $CodraftDir) $PortFileName

if (-not (Test-Path $portFile)) {
    $portFile = Join-Path $projectRoot $PortFileName
}

if (Test-Path $portFile) {
    $port = (Get-Content $portFile -Raw).Trim()
    if ($port -match '^\d+$') {
        $url = "http://localhost:$port"
        Write-Host "Tracker UI active at $url (opened automatically in IDE Simple Browser)"
    }
    else {
        Write-Host "Invalid port in $portFile"
    }
}
else {
    Write-Host "Tracker not running yet (no $portFile). Start MCP or run codraft.exe first."
}
