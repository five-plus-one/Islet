$ErrorActionPreference = 'Stop'
$mounts = docker inspect islet-review-app --format '{{json .Mounts}}' | ConvertFrom-Json
$volume = ($mounts | Where-Object Destination -eq '/data').Name
if (-not $volume -or $volume -notmatch '^[a-zA-Z0-9_-]+$') { throw 'Acceptance data volume could not be verified' }
docker stop islet-review-app | Out-Null
docker rm islet-review-app | Out-Null
docker run -d --name islet-review-app --network islet-review -p 127.0.0.1:28090:8080 -p 127.0.0.1:28091:8081 --mount "type=volume,source=$volume,target=/data" --env CONFIG_FILE=/data/islet.json --env APP_ENV=production islet:workspace | Out-Null
Write-Output 'Acceptance application updated; data volume retained'
