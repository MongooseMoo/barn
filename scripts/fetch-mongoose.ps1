[CmdletBinding()]
param([string]$RunDir = '.tmp/mongoose-account-20260917')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Set-Location (Split-Path $PSScriptRoot -Parent)
New-Item -ItemType Directory -Force $RunDir | Out-Null
$run = (Resolve-Path -LiteralPath $RunDir).Path
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
& scp -C mongoose@mongoose.world:~/mongoose/mongoose.db.new "$run/download-$stamp.db"
$code = $LASTEXITCODE
if ($code -ne 0) { throw "Database download failed: $code" }
$remoteBackup = "/tmp/barn-sound-$stamp.sqlite"
$backupScript = @"
import sqlite3
source = sqlite3.connect('file:/home/mongoose/mongoose/files/sqlite/sound.sqlite?mode=ro', uri=True)
destination = sqlite3.connect('$remoteBackup')
source.backup(destination)
result = destination.execute('pragma integrity_check').fetchone()[0]
print('sound_backup_integrity=' + result)
destination.close()
source.close()
assert result == 'ok'
"@
$backupScript | & ssh mongoose@mongoose.world python3 -
$code = $LASTEXITCODE
if ($code -ne 0) { throw "SQLite backup failed: $code" }
& scp -C "mongoose@mongoose.world:$remoteBackup" "$run/sound-$stamp.sqlite"
$code = $LASTEXITCODE
if ($code -ne 0) { throw "Sound download failed: $code" }
New-Item -ItemType Directory -Force files/sqlite | Out-Null
foreach ($pair in @(@('mongoose.db.new', "previous-$stamp.db"), @('files/sqlite/sound.sqlite', "previous-sound-$stamp.sqlite"))) {
    if (Test-Path -LiteralPath $pair[0]) { Copy-Item -LiteralPath $pair[0] -Destination (Join-Path $run $pair[1]) }
}
Copy-Item -LiteralPath "$run/download-$stamp.db" -Destination "$run/mongoose.db.new" -Force
Copy-Item -LiteralPath "$run/download-$stamp.db" -Destination mongoose.db.new -Force
Copy-Item -LiteralPath "$run/sound-$stamp.sqlite" -Destination "$run/sound.sqlite" -Force
Copy-Item -LiteralPath "$run/sound-$stamp.sqlite" -Destination files/sqlite/sound.sqlite -Force
Get-FileHash -LiteralPath "$run/mongoose.db.new", "$run/sound.sqlite" | Format-List Path,Hash
Write-Output "Remote SQLite backup retained at $remoteBackup"
