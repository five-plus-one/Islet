$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$credentialPath = Join-Path $repo 'workspace/acceptance.key'
if (-not (Test-Path -LiteralPath $credentialPath)) {
    $credentials = @{ database = [guid]::NewGuid().ToString('N'); password = [guid]::NewGuid().ToString('N'); username = 'review-admin' } | ConvertTo-Json -Compress
    $credentials | ConvertTo-SecureString -AsPlainText -Force | ConvertFrom-SecureString | Set-Content -LiteralPath $credentialPath
}
$secure = Get-Content -LiteralPath $credentialPath | ConvertTo-SecureString
$credentials = [System.Net.NetworkCredential]::new('', $secure).Password | ConvertFrom-Json
$networkExists = docker network ls --filter name=^islet-review$ --format '{{.Name}}'
if (-not $networkExists) { docker network create islet-review | Out-Null }
$dbExists = docker ps -a --filter name=^islet-review-db$ --format '{{.Names}}'
if (-not $dbExists) {
    $env:MYSQL_PASSWORD = $credentials.database
    docker run -d --name islet-review-db --network islet-review --env MYSQL_RANDOM_ROOT_PASSWORD=yes --env MYSQL_DATABASE=islet --env MYSQL_USER=islet --env MYSQL_PASSWORD mysql:8.0 | Out-Null
    $env:MYSQL_PASSWORD = $null
} else { docker start islet-review-db | Out-Null }
$appExists = docker ps -a --filter name=^islet-review-app$ --format '{{.Names}}'
$storageExists = docker ps -a --filter name=^islet-review-storage$ --format '{{.Names}}'
if (-not $storageExists) {
    $env:MINIO_ROOT_PASSWORD = $credentials.database
    docker run -d --name islet-review-storage --network islet-review -p 127.0.0.1:29000:9000 --env MINIO_ROOT_USER=isletreview --env MINIO_ROOT_PASSWORD quay.io/minio/minio:RELEASE.2024-08-17T01-24-54Z server /data | Out-Null
    $env:MINIO_ROOT_PASSWORD = $null
} else { docker start islet-review-storage | Out-Null }
if (-not $appExists) {
    docker run -d --name islet-review-app --network islet-review -p 127.0.0.1:28090:8080 -p 127.0.0.1:28091:8081 --env CONFIG_FILE=/data/islet.json --env SETUP_DEFAULT_MYSQL_HOST=islet-review-db --env APP_ENV=production islet:workspace | Out-Null
} else { docker start islet-review-app | Out-Null }
Write-Output 'Acceptance services available on http://127.0.0.1:28091'
