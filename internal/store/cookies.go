package store

import (
	"errors"
	"strings"
)

// ErrNotFound 记录不存在
var ErrNotFound = errors.New("记录不存在")

func errNotFound(msg string) error {
	if msg == "" {
		return ErrNotFound
	}
	return errors.New(msg)
}

// CookieRecord 一条平台 Cookie
type CookieRecord struct {
	ID        int64  `json:"id"`
	Platform  string `json:"platform"`
	Content   string `json:"content"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListCookies 按平台（空=全部）返回，启用优先、sort_order 升序、同序按 id 升序（首位优先）
func (s *Store) ListCookies(platform string) ([]*CookieRecord, error) {
	q := `SELECT id,platform,content,label,enabled,sort_order,created_at,updated_at FROM cookies`
	args := []interface{}{}
	if platform != "" {
		q += ` WHERE platform=?`
		args = append(args, platform)
	}
	q += ` ORDER BY platform, sort_order ASC, id ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CookieRecord
	for rows.Next() {
		var c CookieRecord
		var en int
		if err := rows.Scan(&c.ID, &c.Platform, &c.Content, &c.Label, &en,
			&c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = en == 1
		out = append(out, &c)
	}
	return out, rows.Err()
}

// EnabledCookieContents 返回指定平台已启用、按优先级排列的 cookie 内容
func (s *Store) EnabledCookieContents(platform string) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT content FROM cookies WHERE platform=? AND enabled=1 ORDER BY sort_order ASC, id ASC`, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		if c = trimCookie(c); c != "" {
			out = append(out, c)
		}
	}
	return out, rows.Err()
}

func trimCookie(s string) string {
	// 去除可能的 "Cookie:" 前缀与首尾空白
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, p := range []string{"cookie:", "cookie："} {
		if strings.HasPrefix(lower, p) {
			s = strings.TrimSpace(s[len(p):])
			lower = strings.ToLower(s)
			break
		}
	}
	return s
}

// CreateCookie 新增，返回自增 id
func (s *Store) CreateCookie(platform, content, label string, enabled bool, sortOrder int) (int64, error) {
	en := 0
	if enabled {
		en = 1
	}
	now := nowTS()
	res, err := s.db.Exec(
		`INSERT INTO cookies(platform,content,label,enabled,sort_order,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?)`,
		platform, content, label, en, sortOrder, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateCookie 更新指定 id
func (s *Store) UpdateCookie(id int64, platform, content, label string, enabled bool, sortOrder int) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(
		`UPDATE cookies SET platform=?,content=?,label=?,enabled=?,sort_order=?,updated_at=? WHERE id=?`,
		platform, content, label, en, sortOrder, nowTS(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound("Cookie 不存在")
	}
	return nil
}

// SetCookieEnabled 单独启停
func (s *Store) SetCookieEnabled(id int64, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE cookies SET enabled=?,updated_at=? WHERE id=?`, en, nowTS(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound("Cookie 不存在")
	}
	return nil
}

// DeleteCookie 删除
func (s *Store) DeleteCookie(id int64) error {
	res, err := s.db.Exec(`DELETE FROM cookies WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound("Cookie 不存在")
	}
	return nil
}
