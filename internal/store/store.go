package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store SQLite 持久层
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS config (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  id          TEXT PRIMARY KEY,
  url         TEXT NOT NULL,
  page_url    TEXT,
  title       TEXT,
  author      TEXT,
  cover_url   TEXT,
  file_path   TEXT,
  kind        TEXT DEFAULT 'video',
  status      TEXT NOT NULL,
  progress    REAL DEFAULT 0,
  total       INTEGER DEFAULT 0,
  loaded      INTEGER DEFAULT 0,
  error       TEXT DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);

CREATE TABLE IF NOT EXISTS cookies (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  platform    TEXT NOT NULL,
  content     TEXT NOT NULL,
  label       TEXT DEFAULT '',
  enabled     INTEGER NOT NULL DEFAULT 1,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cookies_platform ON cookies(platform, enabled, sort_order);

CREATE TABLE IF NOT EXISTS api_keys (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL DEFAULT '',
  api_key     TEXT NOT NULL UNIQUE,
  enabled     INTEGER NOT NULL DEFAULT 1,
  last_used_at TEXT DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_api_keys_key ON api_keys(api_key, enabled);
`

// schemaMigrations 对旧库做幂等的列补齐
var schemaMigrations = []string{
	`ALTER TABLE tasks ADD COLUMN platform TEXT NOT NULL DEFAULT 'kuaishou';`,
	`ALTER TABLE tasks ADD COLUMN group_id TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE tasks ADD COLUMN group_name TEXT NOT NULL DEFAULT '';`,
	`CREATE INDEX IF NOT EXISTS idx_tasks_group ON tasks(group_id);`,
}

// Open 打开/创建数据库并执行迁移
func Open(dbPath string) (*Store, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %w", err)
		}
	}
	// _txlock=immediate 降低写锁竞争；_pragma 做常规优化
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite 单写连接，避免 database is locked
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("数据库迁移失败: %w", err)
	}
	// 旧库补列：列已存在时忽略错误（SQLite 不支持 ADD COLUMN IF NOT EXISTS）
	for _, m := range schemaMigrations {
		if _, err := db.Exec(m); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				_ = db.Close()
				return nil, fmt.Errorf("数据库增量迁移失败: %w", err)
			}
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func nowTS() string { return time.Now().UTC().Format(time.RFC3339Nano) }
