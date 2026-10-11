# 本地实时调试环境变量（被其他 dev 脚本 dot-source）
# 全部数据隔离在仓库 .devdata 下，端口与生产分离，不影响正式配置。

$RepoRoot = Split-Path -Parent $PSScriptRoot

# 后端监听（回环 + 调试专用端口，避免与 8626 正式实例冲突）
$env:VDCAT_ADDR = "127.0.0.1:18626"

# SQLite / 下载目录隔离
$env:VDCAT_DB = (Join-Path $RepoRoot ".devdata\vdcat-dev.db")
$env:VDCAT_DOWNLOAD_DIR = (Join-Path $RepoRoot ".devdata\downloads")

# 调试用固定登录凭据（生产请勿这样用）
$env:VDCAT_ADMIN_USER = "admin"
$env:VDCAT_ADMIN_PASSWORD = "dev123"
# 固定签名密钥：调试期间重启不掉登录
$env:VDCAT_SECRET = "dev-only-static-secret-do-not-use-in-prod"

# 不自动弹浏览器（由前端 Vite 页面访问）
$env:VDCAT_NO_BROWSER = "1"

# 前端 dev server 与 API 代理目标
$env:VDCAT_FRONT_PORT = "5180"
$env:VDCAT_API_TARGET = "http://127.0.0.1:18626"

# Go 编译加速（纯 Go SQLite，无需 CGO）
$env:CGO_ENABLED = "0"
