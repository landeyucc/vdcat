package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// TaskRecord 任务持久化记录
type TaskRecord struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	PageURL   string  `json:"page_url"`
	Title     string  `json:"title"`
	Author    string  `json:"author"`
	CoverURL  string  `json:"cover_url"`
	FilePath  string  `json:"file_path"`
	Kind      string  `json:"kind"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"`
	Total     int64   `json:"total"`
	Loaded    int64   `json:"loaded"`
	Error     string  `json:"error"`
	Platform  string  `json:"platform"`
	GroupID   string  `json:"group_id"`
	GroupName string  `json:"group_name"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// UpsertTask 插入或更新任务
func (s *Store) UpsertTask(t *TaskRecord) error {
	t.UpdatedAt = nowTS()
	if t.CreatedAt == "" {
		t.CreatedAt = t.UpdatedAt
	}
	if t.Platform == "" {
		t.Platform = "kuaishou"
	}
	_, err := s.db.Exec(`
INSERT INTO tasks(id,url,page_url,title,author,cover_url,file_path,kind,status,progress,total,loaded,error,platform,group_id,group_name,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
 url=excluded.url, page_url=excluded.page_url, title=excluded.title, author=excluded.author,
 cover_url=excluded.cover_url, file_path=excluded.file_path, kind=excluded.kind,
 status=excluded.status, progress=excluded.progress, total=excluded.total, loaded=excluded.loaded,
 error=excluded.error, platform=excluded.platform, updated_at=excluded.updated_at`,
		t.ID, t.URL, t.PageURL, t.Title, t.Author, t.CoverURL, t.FilePath, t.Kind,
		t.Status, t.Progress, t.Total, t.Loaded, t.Error, t.Platform, t.GroupID, t.GroupName,
		t.CreatedAt, t.UpdatedAt)
	return err
}

const taskSelectCols = `id,url,page_url,title,author,cover_url,file_path,kind,status,progress,total,loaded,error,platform,group_id,group_name,created_at,updated_at`

// ActiveStatuses 仍在下载队列中的状态
const ActiveStatuses = `('queued','running','paused','interrupted')`

// ActiveTasks 返回仍在下载队列中的任务记录
func (s *Store) ActiveTasks(limit int) ([]*TaskRecord, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.Query(`
SELECT `+taskSelectCols+`
FROM tasks WHERE status IN `+ActiveStatuses+`
ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// HistoryTasks 分页返回已结束任务（成功/失败/取消），仅视频任务计入，按创建时间倒序。
// platform 非空时按平台过滤；groupID 非空时仅返回该组。
func (s *Store) HistoryTasks(limit, offset int, platform, groupID string) ([]*TaskRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	where, args := historyWhere(platform, groupID)
	args = append(args, limit, offset)
	rows, err := s.db.Query(`
SELECT `+taskSelectCols+`
FROM tasks
WHERE `+where+`
ORDER BY created_at DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// CountHistoryVideos 历史视频任务总数（可按平台/分组过滤）
func (s *Store) CountHistoryVideos(platform, groupID string) (int, error) {
	where, args := historyWhere(platform, groupID)
	var n int
	err := s.db.QueryRow(`
SELECT COUNT(*) FROM tasks WHERE `+where, args...).Scan(&n)
	return n, err
}

// historyWhere 构造历史视频任务的公共过滤条件（始终含「已结束 + 视频」）
func historyWhere(platform, groupID string) (string, []interface{}) {
	cond := "status NOT IN " + ActiveStatuses + " AND kind='video'"
	args := []interface{}{}
	if p := strings.TrimSpace(platform); p != "" {
		cond += " AND platform=?"
		args = append(args, p)
	}
	if g := strings.TrimSpace(groupID); g != "" {
		cond += " AND group_id=?"
		args = append(args, g)
	}
	return cond, args
}

// HistoryGroupSummary 历史分组摘要（用于分组过滤下拉）
type HistoryGroupSummary struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
	Platform  string `json:"platform"`
	Count     int    `json:"count"`
	LatestAt  string `json:"latest_at"`
}

// ListHistoryGroups 返回有已结束视频任务的分组，按最近时间倒序
func (s *Store) ListHistoryGroups(platform string) ([]*HistoryGroupSummary, error) {
	q := `
SELECT group_id, MAX(group_name), MAX(platform), COUNT(*), MAX(created_at)
FROM tasks
WHERE status NOT IN ` + ActiveStatuses + ` AND kind='video' AND group_id<>''`
	args := []interface{}{}
	if p := strings.TrimSpace(platform); p != "" {
		q += " AND platform=?"
		args = append(args, p)
	}
	q += ` GROUP BY group_id ORDER BY MAX(created_at) DESC LIMIT 500`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*HistoryGroupSummary
	for rows.Next() {
		g := &HistoryGroupSummary{}
		if err := rows.Scan(&g.GroupID, &g.GroupName, &g.Platform, &g.Count, &g.LatestAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ListTasks 按创建时间倒序返回任务
func (s *Store) ListTasks(limit int) ([]*TaskRecord, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(`
SELECT `+taskSelectCols+`
FROM tasks ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// GetTask 取单条
func (s *Store) GetTask(id string) (*TaskRecord, error) {
	row := s.db.QueryRow(`SELECT `+taskSelectCols+` FROM tasks WHERE id=?`, id)
	t := &TaskRecord{}
	err := row.Scan(&t.ID, &t.URL, &t.PageURL, &t.Title, &t.Author, &t.CoverURL, &t.FilePath,
		&t.Kind, &t.Status, &t.Progress, &t.Total, &t.Loaded, &t.Error, &t.Platform,
		&t.GroupID, &t.GroupName, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// RecoverableTasks 返回重启后需要重新调度的任务（排队中/下载中/暂停）
func (s *Store) RecoverableTasks() ([]*TaskRecord, error) {
	rows, err := s.db.Query(`
SELECT ` + taskSelectCols + `
FROM tasks WHERE status IN ('queued','running','paused') ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// MarkInterrupted 把上次未完成任务统一标记为 interrupted（供启动时调用）
func (s *Store) MarkInterrupted() error {
	_, err := s.db.Exec(`UPDATE tasks SET status='interrupted', error='服务重启中断', updated_at=?
WHERE status IN ('running','queued','paused')`, nowTS())
	return err
}

// DeleteTask 删除任务记录（磁盘文件不在此处理）
func (s *Store) DeleteTask(id string) error {
	res, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("任务不存在")
	}
	return nil
}

// TasksByGroup 返回某下载组的全部任务（含封面），按创建时间升序
func (s *Store) TasksByGroup(groupID string) ([]*TaskRecord, error) {
	rows, err := s.db.Query(`
SELECT `+taskSelectCols+` FROM tasks WHERE group_id=? ORDER BY created_at ASC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

// DeleteGroup 删除某下载组的全部任务记录，返回删除条数
func (s *Store) DeleteGroup(groupID string) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM tasks WHERE group_id=?`, groupID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func scanTasks(rows *sql.Rows) ([]*TaskRecord, error) {
	var out []*TaskRecord
	for rows.Next() {
		t := &TaskRecord{}
		if err := rows.Scan(&t.ID, &t.URL, &t.PageURL, &t.Title, &t.Author, &t.CoverURL, &t.FilePath,
			&t.Kind, &t.Status, &t.Progress, &t.Total, &t.Loaded, &t.Error, &t.Platform,
			&t.GroupID, &t.GroupName, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ParseTime 解析 RFC3339 时间
func ParseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
