# vdcat 一键构建脚本（仅编译，不安装任何全局依赖）
# 用法: ./build.ps1 [-Target linux|windows|all]
param(
  [ValidateSet("linux", "windows", "all")]
  [string]$Target = "all"
)
$ErrorActionPreference = "Stop"

Write-Host "==> [1/3] 构建前端 (Vue3 + Vuetify)..." -ForegroundColor Cyan
Push-Location "$PSScriptRoot/frontend"
try {
    npm.cmd run build
} finally {
    Pop-Location
}

New-Item -ItemType Directory -Force "$PSScriptRoot/dist" | Out-Null

if ($Target -eq "windows" -or $Target -eq "all") {
    Write-Host "==> [2/3] 编译 Windows 单二进制..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    go build -trimpath -ldflags "-s -w" -o "$PSScriptRoot/vdcat.exe" "$PSScriptRoot"
}

if ($Target -eq "linux" -or $Target -eq "all") {
    Write-Host "==> [3/3] 交叉编译 Linux amd64..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w" -o "$PSScriptRoot/dist/vdcat-linux-amd64" "$PSScriptRoot"
    Remove-Item Env:GOOS, Env:GOARCH
}

Write-Host "==> 完成" -ForegroundColor Green
Get-ChildItem "$PSScriptRoot/dist" -ErrorAction SilentlyContinue | Format-Table Name, Length
