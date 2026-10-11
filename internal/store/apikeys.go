package store

// APIKeyRecord 一个供浏览器扩展等外部程序调用的 API Key
type APIKeyRecord struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	APIKey     string `json:"api_key"`
	Enabled    bool   `json:"enabled"`
	LastUsedAt string `json:"last_used_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// ListAPIKeys 返回全部 API Key，按创建顺序升序
func (s *Store) ListAPIKeys() ([]*APIKeyRecord, error) {
	rows, err := s.db.Query(
		`SELECT id,name,api_key,enabled,last_used_at,created_at,updated_at
		 FROM api_keys ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*APIKeyRecord
	for rows.Next() {
		var k APIKeyRecord
		var en int
		if err := rows.Scan(&k.ID, &k.Name, &k.APIKey, &en,
			&k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt); err != nil {
			return nil, err
		}
		k.Enabled = en == 1
		out = append(out, &k)
	}
	return out, rows.Err()
}

// CreateAPIKey 新建一个已生成的 key，返回 id
func (s *Store) CreateAPIKey(name, key string, enabled bool) (int64, error) {
	en := 0
	if enabled {
		en = 1
	}
	now := nowTS()
	res, err := s.db.Exec(
		`INSERT INTO api_keys(name,api_key,enabled,created_at,updated_at)
		 VALUES(?,?,?,?,?)`,
		name, key, en, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// APIKeyEnabled 校验 key 是否存在且启用；命中返回 true
func (s *Store) APIKeyEnabled(key string) (bool, error) {
	var en int
	err := s.db.QueryRow(
		`SELECT enabled FROM api_keys WHERE api_key=?`, key).Scan(&en)
	if err != nil {
		return false, err
	}
	return en == 1, nil
}

// TouchAPIKey 更新最近使用时间
func (s *Store) TouchAPIKey(key string) {
	_, _ = s.db.Exec(
		`UPDATE api_keys SET last_used_at=? WHERE api_key=?`, nowTS(), key)
}

// SetAPIKeyEnabled 启停
func (s *Store) SetAPIKeyEnabled(id int64, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(
		`UPDATE api_keys SET enabled=?,updated_at=? WHERE id=?`, en, nowTS(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound("API Key 不存在")
	}
	return nil
}

// RotateAPIKey 重置指定 id 的 key 内容
func (s *Store) RotateAPIKey(id int64, newKey string) error {
	res, err := s.db.Exec(
		`UPDATE api_keys SET api_key=?,updated_at=? WHERE id=?`, newKey, nowTS(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound("API Key 不存在")
	}
	return nil
}

// DeleteAPIKey 删除
func (s *Store) DeleteAPIKey(id int64) error {
	res, err := s.db.Exec(`DELETE FROM api_keys WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errNotFound("API Key 不存在")
	}
	return nil
}
