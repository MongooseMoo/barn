[CmdletBinding()]
param(
    [ValidateSet('Barn', 'Toast')][string]$Engine = 'Barn',
    [string]$RunDir = '.tmp/mongoose-account-20260917',
    [string]$Distribution = 'Debian',
    [string]$OracleDir = '/root/src/toaststunt-mongoose-login-20260917',
    [int]$Port = 0,
    [switch]$Start,
    [switch]$BuildOracle,
    [switch]$CaptureDebug,
    [switch]$VerifyTimeOffset,
    [switch]$UntilCleanLogin,
    [int]$LoginDeadline = 300,
    [string]$CleanLoginMarker = 'MESSAGE OF THE DAY:',
    [switch]$Proxy,
    [string]$Username = '',
    [string]$Password = '',
    [string[]]$Commands = @(),
    [int]$BannerWait = 3000,
    [int]$InterCommand = 2500,
    [int]$Timeout = 20,
    [int]$MaxDuration = 60
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($UntilCleanLogin -and (-not $Start -or -not $Username -or -not $CleanLoginMarker)) {
    throw 'UntilCleanLogin requires Start, Username and a nonempty CleanLoginMarker.'
}
$processStarted = $null
if ($Timeout * 1000 -le [Math]::Max($BannerWait, $InterCommand)) {
    throw 'Timeout must exceed BannerWait and InterCommand so the socket reader stays active while commands are sent.'
}
$repo = Split-Path $PSScriptRoot -Parent
Set-Location $repo
$run = (Resolve-Path -LiteralPath $RunDir).Path
if ($Port -eq 0) { $Port = if ($Engine -eq 'Barn') { 11485 } else { 17880 } }
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$prefix = Join-Path $run "$($Engine.ToLower())-$stamp"

function Assert-NativeExit([int]$Code, [string]$Operation) {
    if ($Code -ne 0) { throw "$Operation failed (exit $Code)" }
}

function Quote-Bash([string]$Value) {
    return "'" + $Value.Replace("'", "'\''") + "'"
}

if ($Engine -eq 'Toast' -or $BuildOracle) {
    $installed = (& wsl --list --quiet) -replace "`0", ''
    $code = $LASTEXITCODE
    Assert-NativeExit $code 'list WSL distributions'
    if ($Distribution -notin ($installed | ForEach-Object { $_.Trim() })) {
        throw "WSL distribution is not installed: $Distribution"
    }
    $linuxRun = & wsl -d $Distribution --exec wslpath -a $run.Replace('\', '\\')
    $code = $LASTEXITCODE
    Assert-NativeExit $code 'translate run directory'
    $linuxRun = $linuxRun.Trim()
}

if ($BuildOracle) {
    $body = 'set -euo pipefail; cd ' + (Quote-Bash $OracleDir) + '; git log -1 --oneline; mkdir -p build-release; cd build-release; cmake .. -DCMAKE_BUILD_TYPE=Release; cmake --build . --clean-first -j 8; sha256sum moo'
    & wsl -d $Distribution --exec bash -lc $body 2>&1 | Tee-Object "$prefix-build.txt"
    $code = $LASTEXITCODE
    Assert-NativeExit $code 'build Mongoose Toast'
}

if ($Start) {
    if ($Engine -eq 'Barn') {
        $package = & go list -f '{{.ImportPath}} {{.Name}}' ./cmd/barn
        $code = $LASTEXITCODE
        Assert-NativeExit $code 'resolve Barn main package'
        if ($package -ne 'github.com/MongooseMoo/barn/cmd/barn main') { throw "Unexpected main package: $package" }
        & go build -o "$run/barn.exe" ./cmd/barn
        $code = $LASTEXITCODE
        Assert-NativeExit $code 'build Barn'
        $cwd = Join-Path $run 'barn'
        New-Item -ItemType Directory -Force "$cwd/files/sqlite" | Out-Null
        New-Item -ItemType Directory -Force "$cwd/executables" | Out-Null
        & go build -o "$cwd/executables/tz.exe" ./cmd/mongoose_tz
        $code = $LASTEXITCODE
        Assert-NativeExit $code 'build Mongoose timezone helper'
        Copy-Item -LiteralPath "$run/mongoose.db.new" -Destination "$cwd/mongoose.db.new" -Force
        Copy-Item -LiteralPath "$run/sound.sqlite" -Destination "$cwd/files/sqlite/sound.sqlite" -Force
        $processStarted = [DateTime]::UtcNow
        $proc = Start-Process -FilePath "$run/barn.exe" -ArgumentList "-db mongoose.db.new -promote-numbers -port $Port -debug-addr 127.0.0.1:11486 -operator-addr 127.0.0.1:11487 -checkpoint-interval 0 -log-level debug -log-dir logs" -WorkingDirectory $cwd -WindowStyle Hidden -PassThru -RedirectStandardOutput "$prefix-server-out.txt" -RedirectStandardError "$prefix-server-err.txt"
    } else {
        $toastRun = '/tmp/barn-mongoose-account-' + $stamp
        if (-not (Test-Path -LiteralPath "$run/tz")) { throw 'Missing tz helper; run scripts/fetch-mongoose.ps1 first.' }
        $body = 'set -euo pipefail; mkdir -p ' + (Quote-Bash ($toastRun + '/files/sqlite')) + '; cp ' + (Quote-Bash ($linuxRun + '/mongoose.db.new')) + ' ' + (Quote-Bash ($toastRun + '/input.db')) + '; cp ' + (Quote-Bash ($linuxRun + '/sound.sqlite')) + ' ' + (Quote-Bash ($toastRun + '/files/sqlite/sound.sqlite')) + '; cd ' + (Quote-Bash $toastRun) + '; exec ' + (Quote-Bash ($OracleDir + '/build-release/moo')) + ' -o -i files input.db output.db ' + $Port
        $install = 'mkdir -p ' + (Quote-Bash ($toastRun + '/executables')) + '; cp ' + (Quote-Bash ($linuxRun + '/tz')) + ' ' + (Quote-Bash ($toastRun + '/executables/tz')) + '; chmod +x ' + (Quote-Bash ($toastRun + '/executables/tz')) + '; '
        $body = $body.Replace('set -euo pipefail; ', 'set -euo pipefail; ' + $install)
        $launch = "$run/toast-$stamp.sh"
        [IO.File]::WriteAllText($launch, $body + "`n", [Text.UTF8Encoding]::new($false))
        $processStarted = [DateTime]::UtcNow
        $proc = Start-Process -FilePath wsl.exe -ArgumentList "-d $Distribution --exec bash $linuxRun/toast-$stamp.sh" -WindowStyle Hidden -PassThru -RedirectStandardOutput "$prefix-server-out.txt" -RedirectStandardError "$prefix-server-err.txt"
    }
    $proc.Id | Set-Content "$run/$($Engine.ToLower())-pid.txt"
    Write-Output "Started $Engine relay/process PID $($proc.Id); logs: $prefix-server-err.txt"
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do {
        if ($proc.HasExited) { throw "$Engine exited; inspect $prefix-server-err.txt" }
        $ready = Select-String -LiteralPath "$prefix-server-err.txt" -Pattern 'msg=listening|LISTEN:.*port' -Quiet
        if (-not $ready) { Start-Sleep -Milliseconds 200 }
    } until ($ready -or [DateTime]::UtcNow -ge $deadline)
    if (-not $ready) { throw "$Engine did not report a listener within 60 seconds" }
}

$client = Join-Path $run 'moo_client.exe'
if ($Engine -eq 'Barn') {
    & go build -o $client ./cmd/moo_client
    $code = $LASTEXITCODE
    Assert-NativeExit $code 'build socket client'
} else {
    $savedGOOS = $env:GOOS
    $savedCGO = $env:CGO_ENABLED
    try {
        $env:GOOS = 'linux'; $env:CGO_ENABLED = '0'
        & go build -o "$run/moo_client" ./cmd/moo_client
        $code = $LASTEXITCODE
        Assert-NativeExit $code 'build Linux socket client'
    } finally { $env:GOOS = $savedGOOS; $env:CGO_ENABLED = $savedCGO }
}
$attempt = 0
do {
    $attempt++
    if ($attempt -gt 1) {
        $stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
        $prefix = Join-Path $run "$($Engine.ToLower())-$stamp"
    }
    $probeStarted = [DateTime]::UtcNow
    $lines = @()
    if ($Proxy) { $lines += "PROXY TCP4 203.0.113.5 127.0.0.1 50000 $Port" }
    if ($Username) { $lines += $Username; $lines += $Password }
    if ($VerifyTimeOffset) {
        $lines += ';return {"mongoose-time-offset", exec({"tz", "UTC"}, "", {}), #43:time_offset("America/Denver")};'
    }
    $lines += $Commands
    $inputFile = "$run/input-$stamp.txt"
    [IO.File]::WriteAllLines($inputFile, $lines, [Text.UTF8Encoding]::new($false))
    try {
        $args = @('-host', '127.0.0.1', '-port', $Port, '-banner-wait', $BannerWait, '-inter-cmd', $InterCommand, '-timeout', $Timeout, '-max-duration', $MaxDuration)
        if ($Engine -eq 'Barn') {
            & $client @args -file $inputFile -event-log "$prefix-events.jsonl" 2>&1 |
                ForEach-Object { if ($Password) { $_.ToString().Replace($Password, '[redacted]') } else { $_.ToString() } } |
                Tee-Object "$prefix-client.txt"
        } else {
            & wsl -d $Distribution --exec "$linuxRun/moo_client" @args -file "$linuxRun/input-$stamp.txt" -event-log "$linuxRun/$(Split-Path $prefix -Leaf)-events.jsonl" 2>&1 |
                ForEach-Object { if ($Password) { $_.ToString().Replace($Password, '[redacted]') } else { $_.ToString() } } |
                Tee-Object "$prefix-client.txt"
        }
        $code = $LASTEXITCODE
        Assert-NativeExit $code 'socket probe'
    } finally {
        Remove-Item -LiteralPath $inputFile
        if ($CaptureDebug -and $Engine -eq 'Barn') {
            foreach ($endpoint in @('vars', 'pprof/goroutine?debug=2')) {
                $name = if ($endpoint -eq 'vars') { 'vars.json' } else { 'goroutines.txt' }
                (Invoke-WebRequest "http://127.0.0.1:11486/debug/$endpoint").Content | Set-Content "$prefix-$name"
            }
        }
    }
    Write-Output "Evidence prefix: $prefix"
    $milestones = [ordered]@{
        banner = 'Welcome to...'
        username_prompt = 'Enter your username or email:'
        guest_welcome = '(***) WELCOME! (***)'
        account_welcome = 'Welcome!'
        character_selection = 'Please choose a character to log in as:'
        room = "[Georgie's Guesthouse; The Parlor]"
        access_denied = 'Access Denied'
        confunc_error = 'Confunc failed:'
        clean_login = $CleanLoginMarker
    }
    $seen = @{}
    $received = ''
    foreach ($line in Get-Content -LiteralPath "$prefix-events.jsonl") {
        $event = $line | ConvertFrom-Json
        if ($event.event -ne 'receive') { continue }
        $received += $event.text
        foreach ($name in $milestones.Keys) {
            if (-not $seen.ContainsKey($name) -and $received.Contains($milestones[$name])) {
                $seen[$name] = $event.elapsed_ms
                Write-Output "Milestone ${name}_ms=$($event.elapsed_ms)"
            }
        }
    }
    if ($VerifyTimeOffset) {
        if ($received -notmatch '=> \{"mongoose-time-offset", \{0, "\+0000", ""\}, -(21600|25200)\}') {
            throw "Time-offset smoke check failed; inspect $prefix-client.txt (requires a wizard character)."
        }
        Write-Output 'Time-offset smoke check passed: UTC=+0000 and Denver offset valid'
    }
    if (-not $UntilCleanLogin) { break }
    $clean = $seen.ContainsKey('clean_login') -and -not $seen.ContainsKey('confunc_error') -and -not $seen.ContainsKey('access_denied')
    $timing = [ordered]@{
        engine = $Engine
        attempt = $attempt
        clean = $clean
        process_started_utc = $processStarted.ToString('o')
        observed_at_ms = [Math]::Round(([DateTime]::UtcNow - $processStarted).TotalMilliseconds)
        clean_login_ms = if ($clean) { [Math]::Round(($probeStarted - $processStarted).TotalMilliseconds + $seen['clean_login']) } else { $null }
        evidence_prefix = $prefix
    }
    $timing | ConvertTo-Json | Tee-Object "$prefix-login-timing.json"
    if ($clean) { break }
    if ($seen.ContainsKey('access_denied')) { throw 'Clean-login probe was denied; check the account and checkpoint.' }
    if (([DateTime]::UtcNow - $processStarted).TotalSeconds -ge $LoginDeadline) {
        throw "No clean login observed within $LoginDeadline seconds; inspect the attempt evidence."
    }
} while ($true)
