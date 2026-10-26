package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"vdcat/internal/platform"
	"vdcat/internal/store"
)

// Config 运行期配置。
type Config struct {
	Cookie      string `json:"cookie"`
	DownloadDir string `json:"download_dir"`
	Concurrency int    `json:"concurrency"`
	UseAPI      bool   `json:"use_api"` // 兼容旧字段，已由 ParseMode/ForceParse 取代
	// ParseMode: auto 自动兜底 | api 接口解析 | html HTML 提取
	ParseMode  string `json:"parse_mode"`
	ForceParse bool   `json:"force_parse"` // 强制使用所选方式，失败不切换兜底
	WriteMeta  bool   `json:"write_meta"`

	Addr        string `json:"-"`
	DBPath      string `json:"-"`
	AdminUser   string `json:"-"`
	AdminPass   string `json:"-"`
	StaticToken string `json:"-"` // 若设置则直接接受该 Bearer Token
	AuthEnabled bool   `json:"auth_enabled"`
}

const (
	keyCookie      = "cookie"
	keyDownloadDir = "download_dir"
	keyConcurrency = "concurrency"
	keyUseAPI      = "use_api"
	keyParseMode   = "parse_mode"
	keyForceParse  = "force_parse"
	keyWriteMeta   = "write_meta"
)

var (
	mu      sync.RWMutex
	current Config
	db      *store.Store
)

func defaultDownloadDir() string {
	d, _ := os.UserHomeDir()
	return filepath.Join(d, "downloads", "vdcat")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// Load 从环境变量 + SQLite 加载配置。
func Load(s *store.Store) Config {
	db = s

	dl := envOr("VDCAT_DOWNLOAD_DIR", defaultDownloadDir())
	conc, _ := strconv.Atoi(envOr("VDCAT_CONCURRENCY", "2"))
	if conc <= 0 {
		conc = 2
	}

	cfg := Config{
		DownloadDir: dl,
		Concurrency: conc,
		UseAPI:      envOr("VDCAT_USE_API", "1") != "0",
		ParseMode:   platform.ModeAuto,
		ForceParse:  false,
		WriteMeta:   envOr("VDCAT_WRITE_META", "1") != "0",

		Addr:        envOr("VDCAT_ADDR", "0.0.0.0:8626"),
		DBPath:      envOr("VDCAT_DB", ""),
		AdminUser:   envOr("VDCAT_ADMIN_USER", "admin"),
		AdminPass:   os.Getenv("VDCAT_ADMIN_PASSWORD"),
		StaticToken: os.Getenv("VDCAT_TOKEN"),
	}
	cfg.AuthEnabled = cfg.AdminPass != "" || cfg.StaticToken != ""

	// DB 中保存的业务项覆盖默认值
	kv, err := s.GetAllConfig()
	if err == nil {
		if v, ok := kv[keyCookie]; ok {
			cfg.Cookie = v
		}
		if v, ok := kv[keyDownloadDir]; ok && v != "" {
			cfg.DownloadDir = v
		}
		if v, ok := kv[keyConcurrency]; ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.Concurrency = n
			}
		}
		if v, ok := kv[keyUseAPI]; ok {
			cfg.UseAPI = v == "1" || v == "true"
		}
		if v, ok := kv[keyParseMode]; ok && v != "" {
			cfg.ParseMode = platform.NormalizeMode(v)
		}
		if v, ok := kv[keyForceParse]; ok {
			cfg.ForceParse = v == "1" || v == "true"
		}
		if v, ok := kv[keyWriteMeta]; ok {
			cfg.WriteMeta = v == "1" || v == "true"
		}
	}

	mu.Lock()
	current = cfg
	mu.Unlock()
	return cfg
}

func Get() Config {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Save 保存业务配置，登录相关环境变量字段不会被改动。
func Save(c Config) (Config, error) {
	mu.Lock()
	cur := current
	mu.Unlock()

	if strings.TrimSpace(c.DownloadDir) != "" {
		cur.DownloadDir = c.DownloadDir
	}
	cur.Cookie = c.Cookie
	if c.Concurrency > 0 {
		cur.Concurrency = c.Concurrency
	}
	cur.UseAPI = c.UseAPI
	cur.ParseMode = platform.NormalizeMode(c.ParseMode)
	cur.ForceParse = c.ForceParse
	cur.WriteMeta = c.WriteMeta

	pairs := map[string]string{
		keyCookie:      cur.Cookie,
		keyDownloadDir: cur.DownloadDir,
		keyConcurrency: strconv.Itoa(cur.Concurrency),
		keyUseAPI:      strconv.FormatBool(cur.UseAPI),
		keyParseMode:   cur.ParseMode,
		keyForceParse:  strconv.FormatBool(cur.ForceParse),
		keyWriteMeta:   strconv.FormatBool(cur.WriteMeta),
	}
	for k, v := range pairs {
		if err := db.SetConfigKV(k, v); err != nil {
			return cur, err
		}
	}

	mu.Lock()
	current = cur
	mu.Unlock()
	return cur, nil
}
