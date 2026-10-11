# 后端实时调试：go run 直接运行源码（不产出二进制），轮询监测 .go 文件变更自动重启。
# 用法: ./scripts/dev-backend.ps1 [-NoWatch] [-Clean]
param(
  [switch]$NoWatch,
  [switch]$Clean
)

$ErrorActionPreference = "Stop"

. (Join-Path $PSScriptRoot "dev-env.ps1")
$RepoRoot = Split-Path -Parent $PSScriptRoot

if ($Clean -and (Test-Path (Join-Path $RepoRoot ".devdata"))) {
  Write-Host "[dev] 清理调试数据目录 .devdata" -ForegroundColor Yellow
  Remove-Item -Recurse -Force (Join-Path $RepoRoot ".devdata") -ErrorAction SilentlyContinue
}

$script:proc = $null

function Stop-Running {
  if ($script:proc -and -not $script:proc.HasExited) {
    Write-Host "[dev] 停止旧进程 (PID $($script:proc.Id))" -ForegroundColor Yellow
    # 结束 go run 及其编译出的子进程树
    taskkill /PID $script:proc.Id /T /F 2>$null | Out-Null
    try { $script:proc.Kill() } catch {}
    $script:proc.WaitForExit(3000) | Out-Null
  }
  $script:proc = $null
}

function Start-Backend {
  Stop-Running
  Write-Host "[dev] go run .  ->  $env:VDCAT_ADDR" -ForegroundColor Cyan
  $script:proc = Start-Process -FilePath "go" -ArgumentList "run", "." `
    -WorkingDirectory $RepoRoot -NoNewWindow -PassThru
}

# 收集需要监视的 Go 源文件及其最后写入时间
function Get-Snapshot {
  Get-ChildItem -Path $RepoRoot -Recurse -Filter *.go -File -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notmatch '\\(\.devdata|dist|node_modules)(\\|$)' } |
    ForEach-Object { @{ $_.FullName = $_.LastWriteTimeUtc.Ticks } } |
    ForEach-Object { $_.GetEnumerator() }
}

Start-Backend

if ($NoWatch) {
  try { $script:proc.WaitForExit() } catch {}
  exit 0
}

$snapshot = @{}
foreach ($i in Get-Snapshot) { $snapshot[$i.Key] = $i.Value }
$lastRestart = Get-Date

Write-Host "[dev] 监视 Go 源码变更中，Ctrl+C 退出（首次编译需稍候）..." -ForegroundColor Green

# 以"文件变更信号"统一驱动：进程在跑则保存即重启；
# 若进程因编译错误退出，继续改代码保存同样会触发重启。
try {
  while ($true) {
    Start-Sleep -Milliseconds 1000

    $changed = $false
    $current = @{}
    foreach ($i in Get-Snapshot) {
      $current[$i.Key] = $i.Value
      if (-not $snapshot.ContainsKey($i.Key) -or $snapshot[$i.Key] -ne $i.Value) { $changed = $true }
    }
    $snapshot = $current

    if ($changed -and ((Get-Date) - $lastRestart).TotalMilliseconds -gt 800) {
      Write-Host "`n[dev] 检测到源码变更，重新启动..." -ForegroundColor Magenta
      Start-Backend
      $lastRestart = Get-Date
    }
  }
} finally {
  Stop-Running
}
