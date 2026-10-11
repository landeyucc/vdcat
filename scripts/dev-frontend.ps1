# 前端实时调试：Vite dev server，修改 .vue/.js 即时热更新；/api 自动代理到后端 18626。
# 用法: ./scripts/dev-frontend.ps1
#   -Install  首次运行先安装依赖
param(
  [switch]$Install
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "dev-env.ps1")
$FrontDir = Join-Path (Split-Path -Parent $PSScriptRoot) "frontend"

if ($Install -or -not (Test-Path (Join-Path $FrontDir "node_modules"))) {
  Write-Host "[dev] 安装前端依赖..." -ForegroundColor Cyan
  & npm.cmd --prefix $FrontDir install
}

Write-Host "[dev] 启动 Vite (http://127.0.0.1:$env:VDCAT_FRONT_PORT), /api -> $env:VDCAT_API_TARGET" -ForegroundColor Cyan
& npm.cmd --prefix $FrontDir run dev -- --host 127.0.0.1 --port $env:VDCAT_FRONT_PORT
