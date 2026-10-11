# 本地接口冒烟测试：对正在运行的后端（默认 18626）走一遍 health/登录/配置/任务。
# 用法: ./scripts/dev-smoke.ps1
#   -Url <视频链接>  可选，附带一次真实解析
param(
  [string]$Url = "https://www.kuaishou.com/short-video/3xn4ybnreygegqg"
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "dev-env.ps1")
$Api = "http://127.0.0.1:$($env:VDCAT_ADDR.Split(':')[1])"

function Assert-OK($resp, $step) {
  if ($null -eq $resp -or -not $resp.ok) {
    Write-Host "[FAIL] $step -> $($resp | ConvertTo-Json -Compress)" -ForegroundColor Red
    exit 1
  }
  Write-Host "[OK]   $step" -ForegroundColor Green
}

Write-Host "==> 健康检查 $Api/api/health" -ForegroundColor Cyan
$health = Invoke-RestMethod "$Api/api/health" -UseBasicParsing
Assert-OK $health "health"

Write-Host "==> 登录 admin / $($env:VDCAT_ADMIN_PASSWORD)" -ForegroundColor Cyan
$login = Invoke-RestMethod "$Api/api/login" -Method POST -ContentType "application/json" `
  -Body (@{ username = $env:VDCAT_ADMIN_USER; password = $env:VDCAT_ADMIN_PASSWORD } | ConvertTo-Json) `
  -UseBasicParsing
Assert-OK $login "login"
$h = @{ Authorization = "Bearer $($login.data.token)" }

$cfg = Invoke-RestMethod "$Api/api/config" -Headers $h -UseBasicParsing
Assert-OK $cfg "get config (cookie_set=$($cfg.data.cookie_set), concurrency=$($cfg.data.concurrency))"

$tasks = Invoke-RestMethod "$Api/api/tasks" -Headers $h -UseBasicParsing
Assert-OK $tasks "list tasks (count=$($tasks.data.Count))"

$files = Invoke-RestMethod "$Api/api/files" -Headers $h -UseBasicParsing
Assert-OK $files "list files (count=$($files.data.files.Count))"

if ($Url) {
  Write-Host "==> 真实解析 $Url" -ForegroundColor Cyan
  try {
    $info = Invoke-RestMethod "$Api/api/parse" -Method POST -ContentType "application/json" `
      -Body (@{ url = $Url } | ConvertTo-Json) -Headers $h -UseBasicParsing
    Assert-OK $info "parse (source=$($info.data.source))"
    Write-Host "      title : $($info.data.title)" -ForegroundColor DarkGray
    Write-Host "      video : $($info.data.video_url.Substring(0, [Math]::Min(80, $info.data.video_url.Length)))..." -ForegroundColor DarkGray
  } catch {
    Write-Host "[WARN] 解析失败（可能触发风控/需 Cookie）: $($_.Exception.Message)" -ForegroundColor Yellow
  }
}

Write-Host "`n全部冒烟检查通过。" -ForegroundColor Green
