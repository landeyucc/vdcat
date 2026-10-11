#!/usr/bin/env bash
# vdcat Linux 服务端一键初始化（在服务器上以 root 运行）
# 用法:
#   1. 上传二进制与本目录：scp dist/vdcat-linux-amd64 deploy/*  user@server:/tmp/vdcat/
#   2. sudo bash install.sh
set -euo pipefail

APP_DIR=/opt/vdcat
BIN_NAME=vdcat
SRC_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> 创建用户与目录"
id -u vdcat &>/dev/null || useradd --system --home-dir "$APP_DIR" --shell /usr/sbin/nologin vdcat
mkdir -p "$APP_DIR/data" "$APP_DIR/downloads"

echo "==> 安装二进制"
install -m 0755 "$SRC_DIR/vdcat-linux-amd64" "$APP_DIR/$BIN_NAME"

if [ ! -f "$APP_DIR/vdcat.env" ]; then
  echo "==> 生成环境变量配置（请编辑后再启动）"
  install -m 0640 "$SRC_DIR/vdcat.env.example" "$APP_DIR/vdcat.env"
else
  echo "==> 已存在 $APP_DIR/vdcat.env，保留"
fi

chown -R vdcat:vdcat "$APP_DIR"

echo "==> 安装 systemd 服务"
install -m 0644 "$SRC_DIR/vdcat.service" /etc/systemd/system/vdcat.service
systemctl daemon-reload
systemctl enable vdcat

cat <<EOF

============================================================
 安装完成。下一步：

   1) 编辑配置（务必修改管理员密码 / Token）：
        sudo -u vdcat vi $APP_DIR/vdcat.env

   2) 启动：
        sudo systemctl start vdcat
        sudo systemctl status vdcat
        journalctl -u vdcat -f

   3) 外网访问建议配置 nginx + HTTPS（见 nginx-vdcat.conf）

   数据：数据库 $APP_DIR/data/vdcat.db，下载目录 $APP_DIR/downloads
============================================================
EOF
