package store

import "database/sql"

// 配置以 key-value 存储；敏感的 cookie 等也存于此，但登录凭据只来自环境变量，不落库
func (s *Store) GetConfigKV(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) GetAllConfig() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetConfigKV(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO config(key,value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value)
	return err
}
