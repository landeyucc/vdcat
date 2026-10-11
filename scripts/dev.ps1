# 一键本地实时调试：同时启动后端(go run 热重启) 与 前端(Vite 热更新)，各自独立窗口。
# 用法:
#   ./scripts/dev.ps1            # 启动前后端
#   ./scripts/dev.ps1 -Clean     # 重置调试数据库后启动
#   ./scripts/dev.ps1 -BackendOnly
param(
  [switch]$Clean,
  [switch]$BackendOnly
)

$ErrorActionPreference = "Stop"
$ScriptsDir = $PSScriptRoot
$RepoRoot = Split-Path -Parent $ScriptsDir

Write-Host "==> 启动后端（go run 源码，变更自动重启）..." -ForegroundColor Cyan
$beArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", (Join-Path $ScriptsDir "dev-backend.ps1"))
if ($Clean) { $beArgs += "-Clean" }
Start-Process -FilePath "powershell.exe" -ArgumentList $beArgs -WorkingDirectory $RepoRoot

if (-not $BackendOnly) {
  Write-Host "==> 启动前端（Vite 热更新）..." -ForegroundColor Cyan
  $feArgs = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", (Join-Path $ScriptsDir "dev-frontend.ps1"))
  Start-Process -FilePath "powershell.exe" -ArgumentList $feArgs -WorkingDirectory $RepoRoot
}

. (Join-Path $ScriptsDir "dev-env.ps1")
Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host " 后端 API : http://127.0.0.1:$($env:VDCAT_ADDR.Split(':')[1])" -ForegroundColor Green
if (-not $BackendOnly) {
  Write-Host " 前端页面 : http://127.0.0.1:$env:VDCAT_FRONT_PORT   （用此地址访问）" -ForegroundColor Green
}
Write-Host " 登录账号 : admin / dev123" -ForegroundColor Green
Write-Host " 调试数据 : $(Join-Path $RepoRoot '.devdata')" -ForegroundColor Green
Write-Host " 关闭对应窗口即停止服务" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
