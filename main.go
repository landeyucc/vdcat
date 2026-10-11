package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"vdcat/internal/auth"
	"vdcat/internal/config"
	"vdcat/internal/downloader"
	"vdcat/internal/server"
	"vdcat/internal/store"
)

func defaultDBPath() string {
	if v := os.Getenv("VDCAT_DB"); v != "" {
		return v
	}
	// 默认放在可执行文件同目录 data/vdcat.db，便于服务器挂载持久卷
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "data", "vdcat.db")
	}
	return "vdcat.db"
}

func openBrowser(url string) {
	if os.Getenv("VDCAT_NO_BROWSER") == "1" {
		return
	}
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}

func main() {
	// 尽早加载 .env（工作目录 / 可执行文件目录），已存在的环境变量优先
	config.LoadDefaultDotEnv()

	dbPath := defaultDBPath()
	st, err := store.Open(dbPath)
	if err != nil {
		fmt.Println("数据库初始化失败:", err)
		os.Exit(1)
	}
	defer st.Close()

	cfg := config.Load(st)

	auther := auth.New(cfg.AdminUser, cfg.AdminPass, cfg.StaticToken)

	mgr := downloader.New(st)
	mgr.SetConcurrency(cfg.Concurrency)
	mgr.Recover(true) // 重启后自动续传上次未完成任务

	srv := server.New(mgr, st, auther)
	ln, err := server.Listen(cfg.Addr, srv.Routes())
	if err != nil {
		fmt.Println("启动失败:", err)
		os.Exit(1)
	}

	url := "http://" + ln.Addr().String()
	fmt.Println("======================================")
	fmt.Println("  vdcat 短视频下载服务已启动")
	fmt.Println("  监听地址:  ", url)
	fmt.Println("  数据库:    ", dbPath)
	fmt.Println("  下载目录:  ", cfg.DownloadDir)
	fmt.Println("  并发数:    ", cfg.Concurrency)
	if auther.Enabled() {
		fmt.Println("  登录鉴权:  已启用")
	} else {
		fmt.Println("  登录鉴权:  未启用（设置 VDCAT_ADMIN_PASSWORD 或 VDCAT_TOKEN 启用）")
	}
	fmt.Println("  按 Ctrl+C 退出")
	fmt.Println("======================================")

	// 仅在本机回环地址且非服务器环境尝试开浏览器
	if !isServerHeadless() {
		openBrowser(url)
	}

	select {}
}

// isServerHeadless 判断是否为典型服务器/监听全网卡环境，避免无谓调用浏览器
func isServerHeadless() bool {
	if os.Getenv("VDCAT_NO_BROWSER") == "1" {
		return true
	}
	if runtime.GOOS == "linux" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return true
		}
	}
	return false
}
