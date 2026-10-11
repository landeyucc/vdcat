# vdcat · 短视频解析下载服务

自托管的抖音 / 快手短视频解析与下载工具。Go 单二进制内嵌 Vue3 前端，无需 Node.js 运行时；配置与任务历史使用纯 Go SQLite 持久化（无 CGO 依赖），支持 Windows / Linux 跨平台运行，并提供浏览器扩展对接。

## 功能特性

- **多平台解析**：自动识别链接来源平台（抖音、快手），也可手动指定
- **双解析通道**：接口解析（GraphQL / App feed）与 HTML 提取，可在设置中选择解析方式
  - `auto` 自动兜底：先接口后 HTML
  - `api` / `html` 指定首选方式，非强制时失败仍可兜底
  - 「强制此模式」开启后，所选方式失败不再切换其它方案
- **分享短链支持**：快手 `/f/`、抖音 `v.douyin.com` 等短链先跟随重定向再解析
- **多 Cookie 轮转**：同平台可配置多个已启用 Cookie，默认用首位，解析失败自动切换下一个
- **下载管理**：断点续传、进度持久化，重启后自动恢复未完成任务；下载队列与历史分离，历史支持按分组 / 平台过滤与分页
- **文件组织**：按「平台分类 → 单次任务独立文件夹」存储
- **在线预览**：后端同源媒体代理透传 Range 请求，规避浏览器直连 CDN 的防盗链拦截（域名白名单防 SSRF）
- **开放 API**：为浏览器扩展提供独立 `X-Api-Key` 鉴权的解析 / 下载 / 任务查询接口
- **鉴权**：可选账号密码登录或静态 Bearer Token，凭据仅来自环境变量，不入库、不回传前端

## 技术栈

| 层  | 技术                                                           |
| -- | ------------------------------------------------------------ |
| 后端 | Go 1.26、`net/http`、modernc.org/sqlite（纯 Go，无 CGO）、brotli 解压  |
| 前端 | Vue 3、Vite 6、Vuetify 3（Material Design 3），构建产物 `go:embed` 内嵌 |
| 扩展 | Chrome Manifest V3（原生 JS）                                    |

## 快速开始

### 环境要求

- Go 1.26+（仅构建时需要）
- Node.js 18+ 与 npm（仅构建前端时需要；运行产物不依赖 Node）

### 本地开发（实时调试，免编译）

Windows PowerShell：

```powershell
./scripts/dev.ps1
```

该脚本会分别启动后端（`go run` 源码、变更自动重启）与前端（Vite 热更新），访问脚本输出的「前端页面」地址即可。调试数据位于 `.devdata/`。

常用参数：

```powershell
./scripts/dev.ps1 -Clean         # 重置调试数据库后启动
./scripts/dev.ps1 -BackendOnly   # 仅启动后端
```

### 构建单二进制

```powershell
./build.ps1                 # 同时构建 Windows 与 Linux
./build.ps1 -Target windows # 仅 Windows
./build.ps1 -Target linux   # 仅 Linux amd64
```

脚本先执行前端 `vite build`（输出到 `internal/server/web`），再以 `CGO_ENABLED=0` 编译：

- Windows：`vdcat.exe`
- Linux：`dist/vdcat-linux-amd64`

运行：

```bash
# Windows
./vdcat.exe
# Linux
./vdcat-linux-amd64
```

启动后默认监听 `http://0.0.0.0:8626`（本机访问 `http://127.0.0.1:8626`），桌面环境会自动打开浏览器。

## 配置

配置优先级：**已存在的系统环境变量 >** **`.env`** **文件 > 内置默认值**。`.env` 缺失时静默跳过。

复制模板并修改：

```powershell
Copy-Item .env.example .env
```

| 变量                     | 说明                              | 默认值                     |
| ---------------------- | ------------------------------- | ----------------------- |
| `VDCAT_ADMIN_USER`     | 登录用户名                           | `admin`                 |
| `VDCAT_ADMIN_PASSWORD` | 登录密码；设置后即开启登录鉴权                 | 空（不鉴权）                  |
| `VDCAT_TOKEN`          | 固定 Bearer Token，设置后可直接用于 API 访问 | 空                       |
| `VDCAT_SECRET`         | 会话签名密钥，配置后重启不导致登录失效             | 自动生成                    |
| `VDCAT_ADDR`           | 监听地址                            | `0.0.0.0:8626`          |
| `VDCAT_DB`             | SQLite 数据库路径                    | 可执行文件目录 `data/vdcat.db` |
| `VDCAT_DOWNLOAD_DIR`   | 下载目录                            | 用户目录 `downloads/vdcat`  |
| `VDCAT_CONCURRENCY`    | 并发下载任务数                         | `2`                     |
| `VDCAT_NO_BROWSER`     | 设为 `1` 启动时不自动打开浏览器              | `0`                     |

> `.env` 已加入 `.gitignore`，请勿提交真实凭据。下载目录、并发数、Cookie、解析方式等业务项也可在前端「设置」中修改并持久化到数据库。

## 浏览器扩展

`extension/` 为 Manifest V3 扩展，可在 Chrome / Edge 的 `chrome://extensions` 开启「开发者模式」后「加载已解压的扩展程序」。

扩展通过开放 API 对接自托管服务，在扩展选项页配置：

- **服务地址**：如 `http://127.0.0.1:8626` 或服务器域名
- **API Key**：在 vdcat 前端「API 接入」页创建

### 开放接口

所有请求在请求头携带 `X-Api-Key: <key>`（也兼容 `Authorization: Bearer <key>` 或 `?api_key=`）。

| 方法   | 路径                       | 说明          | 请求体                            |
| ---- | ------------------------ | ----------- | ------------------------------ |
| POST | `/api/external/parse`    | 仅解析，返回无水印直链 | `{"url":"...","platform":"?"}` |
| POST | `/api/external/download` | 解析并入队下载     | `{"url":"...","cover":true}`   |
| GET  | `/api/external/tasks`    | 查询任务状态与进度   | —                              |

调用示例：

```bash
curl -X POST http://127.0.0.1:8626/api/external/parse \
  -H "X-Api-Key: vk_xxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.kuaishou.com/short-video/xxxxx"}'
```

## Linux 服务端部署

```bash
# 上传二进制与部署文件
scp dist/vdcat-linux-amd64 deploy/* user@server:/tmp/vdcat/

# 在服务器上一键初始化（创建用户、安装 systemd 服务）
cd /tmp/vdcat && sudo bash install.sh

# 编辑配置（务必修改管理员密码 / Token）
sudo -u vdcat vi /opt/vdcat/vdcat.env

# 启动
sudo systemctl start vdcat
```

- 数据库：`/opt/vdcat/data/vdcat.db`
- 下载目录：`/opt/vdcat/downloads`
- 外网访问建议参考 `deploy/nginx-vdcat.conf` 配置 Nginx + HTTPS 反向代理

## 说明与限制

- 抖音 Web 端详情接口受设备指纹风控，故采用 App 端 feed 接口免登录获取无水印资源；抖音暂不支持 HTML 提取模式，强制该模式会明确报错，浏览器扩展也暂不支持解析 HTML DOM 下载资源。
- 媒体代理白名单仅放行抖音系 / 快手系视频与图片 CDN 域名（后缀通配），不转发任意主机，避免 SSRF。
- 请遵守目标平台的服务条款与相关法律法规，本工具仅供个人学习与合理使用。

