package downloader

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vdcat/internal/config"
	"vdcat/internal/platform"
	"vdcat/internal/store"
)

// Status 任务状态
type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusDone        Status = "done"
	StatusError       Status = "error"
	StatusCanceled    Status = "canceled"
	StatusPaused      Status = "paused"
	StatusInterrupted Status = "interrupted" // 服务重启导致中断，可重试
)

// Task 下载任务（运行态）
type Task struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	PageURL   string    `json:"page_url"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	CoverURL  string    `json:"cover_url"`
	FilePath  string    `json:"file_path"`
	Status    Status    `json:"status"`
	Progress  float64   `json:"progress"`
	Total     int64     `json:"total"`
	Loaded    int64     `json:"loaded"`
	Speed     int64     `json:"speed"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Kind      string    `json:"kind"`
	Platform  string    `json:"platform"` // kuaishou | douyin
	GroupID   string    `json:"group_id"`
	GroupName string    `json:"group_name"` // 时间戳+平台
	Cookie    string    `json:"-"`          // 解析成功时实际使用的 cookie，下载复用

	cancel chan struct{}
}

var invalidFileChar = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]`)

func sanitize(name string) string {
	name = invalidFileChar.ReplaceAllString(name, "_")
	name = strings.TrimSpace(name)
	if name == "" {
		name = "video"
	}
	r := []rune(name)
	if len(r) > 80 {
		r = r[:80]
	}
	return string(r)
}

// Manager 下载队列管理器
type Manager struct {
	mu      sync.RWMutex
	tasks   map[string]*Task
	counter int64
	sem     chan struct{}
	hc      *http.Client
	store   *store.Store

	lastPersist map[string]time.Time
}

func New(st *store.Store) *Manager {
	m := &Manager{
		tasks:       make(map[string]*Task),
		sem:         make(chan struct{}, 8),
		store:       st,
		lastPersist: map[string]time.Time{},
	}
	m.hc = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:    16,
			IdleConnTimeout: 30 * time.Second,
		},
		Timeout: 0,
	}
	return m
}

// SetConcurrency 调整并发信号量容量
func (m *Manager) SetConcurrency(n int) {
	if n <= 0 {
		n = 2
	}
	m.sem = make(chan struct{}, n)
}

func (m *Manager) nextID() string {
	n := atomic.AddInt64(&m.counter, 1)
	return fmt.Sprintf("t%d-%d", time.Now().UnixNano(), n)
}

// NewGroup 创建一次批量下载的组标识与组名。
// 组名格式：时间戳(20060102-150405) + 平台中文，如 20261010-153012-抖音。
func NewGroup(pf platform.Platform) (groupID, groupName string) {
	now := time.Now()
	return fmt.Sprintf("g%d", now.UnixNano()),
		fmt.Sprintf("%s-%s", now.Format("20060102-150405"), pf.Label())
}

// Enqueue 由统一平台解析结果创建视频(+封面)下载任务。
// cookie 为解析成功时实际使用的该平台 Cookie，下载时复用。
func (m *Manager) Enqueue(info *platform.Info, cookie, dir, groupID, groupName string) (*Task, *Task) {
	m.mu.Lock()
	now := time.Now()
	slug := firstNonEmpty(strings.TrimSpace(info.Author+"-"+firstNonEmpty(info.Title, info.ID)), info.ID)
	taskDir := m.uniqueTaskDir(dir, string(info.Platform), now, slug)
	video := &Task{
		ID:        m.nextID(),
		URL:       info.VideoURL,
		PageURL:   info.PageURL,
		Title:     info.Title,
		Author:    info.Author,
		CoverURL:  info.CoverURL,
		Status:    StatusQueued,
		CreatedAt: now,
		Kind:      "video",
		Platform:  string(info.Platform),
		GroupID:   groupID,
		GroupName: groupName,
		Cookie:    cookie,
		cancel:    make(chan struct{}, 1),
		FilePath:  filepath.Join(taskDir, "video.mp4"),
	}

	var cover *Task
	if info.CoverURL != "" {
		cover = &Task{
			ID:        m.nextID(),
			URL:       info.CoverURL,
			PageURL:   info.PageURL,
			Title:     info.Title,
			Author:    info.Author,
			Status:    StatusQueued,
			CreatedAt: now,
			Kind:      "cover",
			Platform:  string(info.Platform),
			GroupID:   groupID,
			GroupName: groupName,
			Cookie:    cookie,
			cancel:    make(chan struct{}, 1),
			FilePath:  filepath.Join(taskDir, "cover"+extFromURL(info.CoverURL, ".jpg")),
		}
	}
	m.tasks[video.ID] = video
	if cover != nil {
		m.tasks[cover.ID] = cover
	}
	m.mu.Unlock()

	m.persist(video)
	if cover != nil {
		m.persist(cover)
	}

	go m.run(video, info)
	if cover != nil {
		go m.run(cover, nil)
	}
	return video, cover
}

// EnqueueDirect 手动直链兜底
func (m *Manager) EnqueueDirect(videoURL, referer, cookie, dir string) *Task {
	m.mu.Lock()
	now := time.Now()
	taskDir := m.uniqueTaskDir(dir, "", now, "direct")
	t := &Task{
		ID:        m.nextID(),
		URL:       videoURL,
		PageURL:   referer,
		Title:     "direct-" + now.Format("0102-150405"),
		Status:    StatusQueued,
		CreatedAt: now,
		Kind:      "video",
		cancel:    make(chan struct{}, 1),
		FilePath:  filepath.Join(taskDir, "video.mp4"),
	}
	m.tasks[t.ID] = t
	m.mu.Unlock()
	m.persist(t)
	t.Cookie = cookie
	go m.run(t, nil)
	return t
}

// Retry 重试失败/中断的任务
func (m *Manager) Retry(id string) (*Task, error) {
	m.mu.Lock()
	if t, ok := m.tasks[id]; ok && (t.Status == StatusQueued || t.Status == StatusRunning) {
		m.mu.Unlock()
		return t, nil
	}
	m.mu.Unlock()

	rec, err := m.store.GetTask(id)
	if err != nil || rec == nil {
		return nil, errors.New("任务不存在")
	}
	t := m.recordToTask(rec)
	t.Status = StatusQueued
	t.Error = ""
	t.cancel = make(chan struct{}, 1)

	m.mu.Lock()
	m.tasks[t.ID] = t
	m.mu.Unlock()
	m.persist(t)

	go m.run(t, nil)
	return t, nil
}

// platformFolder 返回平台对应的一级存储目录名（中文，便于用户管理）
func platformFolder(pf string) string {
	switch platform.Platform(pf) {
	case platform.Kuaishou:
		return "快手"
	case platform.Douyin:
		return "抖音"
	default:
		return "手动直链"
	}
}

// uniqueTaskDir 计算「下载目录/平台/时间戳-作者-标题」形式的任务文件夹，
// 若同名目录已存在（同秒同作者标题）则追加 -2、-3 去重，保证每次任务独立成夹。
func (m *Manager) uniqueTaskDir(root, pf string, now time.Time, slug string) string {
	stem := now.Format("20060102-150405")
	if s := sanitize(slug); s != "" && s != "video" {
		stem += "-" + s
	}
	base := filepath.Join(root, platformFolder(pf), stem)
	d := base
	for i := 2; ; i++ {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			return d
		}
		d = filepath.Join(root, platformFolder(pf), fmt.Sprintf("%s-%d", stem, i))
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func extFromURL(u, fallback string) string {
	seg := u
	if q := strings.IndexAny(u, "?#"); q >= 0 {
		seg = u[:q]
	}
	if dot := strings.LastIndex(seg, "."); dot >= 0 {
		e := seg[dot:]
		if len(e) <= 5 {
			return e
		}
	}
	return fallback
}

// IsActive 状态是否仍属于下载队列（未进入历史）
func IsActive(s Status) bool {
	switch s {
	case StatusQueued, StatusRunning, StatusPaused, StatusInterrupted:
		return true
	}
	return false
}

// mergeLive 用 DB 记录兜底，内存活跃态覆盖，返回给定记录集对应的任务
func (m *Manager) mergeLive(recs []*store.TaskRecord) []*Task {
	m.mu.RLock()
	live := map[string]*Task{}
	for id, t := range m.tasks {
		cp := *t
		cp.cancel = nil
		live[id] = &cp
	}
	m.mu.RUnlock()

	out := make([]*Task, 0, len(recs))
	for _, r := range recs {
		if t, ok := live[r.ID]; ok {
			out = append(out, t)
		} else {
			out = append(out, m.recordToTask(r))
		}
	}
	return out
}

// ListActive 返回下载队列中的全部任务（含视频与封面），内存活跃态优先
func (m *Manager) ListActive() []*Task {
	recs, err := m.store.ActiveTasks(1000)
	if err != nil {
		recs = nil
	}
	out := m.mergeLive(recs)
	// 内存中可能存在 DB 尚未标记为活跃的极端情况：补充任何内存活跃任务
	seen := map[string]bool{}
	for _, t := range out {
		seen[t.ID] = true
	}
	m.mu.RLock()
	for id, t := range m.tasks {
		if !seen[id] && IsActive(t.Status) {
			cp := *t
			cp.cancel = nil
			out = append(out, &cp)
		}
	}
	m.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// HistoryGroup 历史中的一个下载组
type HistoryGroup struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Platform  string  `json:"platform"`
	CreatedAt string  `json:"created_at"`
	Total     int     `json:"total"`
	Done      int     `json:"done"`
	Error     int     `json:"error"`
	Canceled  int     `json:"canceled"`
	Items     []*Task `json:"items"`
}

// HistoryPage 分页后的历史组
type HistoryPage struct {
	Groups []*HistoryGroup `json:"groups"`
	Total  int             `json:"total"`
	Page   int             `json:"page"`
	Size   int             `json:"size"`
}

// History 返回一页历史，按组聚合。分页以视频任务为基准（limit/offset 作用于视频行），
// platform/groupID 非空时做对应过滤。
func (m *Manager) History(page, size int, platform, groupID string) (*HistoryPage, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 10
	}
	offset := (page - 1) * size
	videos, err := m.store.HistoryTasks(size, offset, platform, groupID)
	if err != nil {
		return nil, err
	}
	total, err := m.store.CountHistoryVideos(platform, groupID)
	if err != nil {
		return nil, err
	}
	hp := &HistoryPage{Groups: []*HistoryGroup{}, Total: total, Page: page, Size: size}

	// 收集本页视频对应的组 id（保持视频时间倒序）
	groupIDs := []string{}
	seen := map[string]bool{}
	for _, v := range videos {
		if v.GroupID != "" && !seen[v.GroupID] {
			seen[v.GroupID] = true
			groupIDs = append(groupIDs, v.GroupID)
		}
	}
	for _, gid := range groupIDs {
		recs, err := m.store.TasksByGroup(gid)
		if err != nil || len(recs) == 0 {
			continue
		}
		g := &HistoryGroup{ID: gid}
		var videoItems []*Task
		var earliest string
		for _, r := range recs {
			t := m.recordToTaskMerge(r)
			if r.Kind == "video" {
				g.Total++
				switch t.Status {
				case StatusDone:
					g.Done++
				case StatusError, StatusInterrupted:
					g.Error++
				case StatusCanceled:
					g.Canceled++
				}
				videoItems = append(videoItems, t)
			}
			if g.Name == "" {
				g.Name = r.GroupName
				g.Platform = r.Platform
			}
			if earliest == "" || r.CreatedAt < earliest {
				earliest = r.CreatedAt
			}
		}
		g.CreatedAt = earliest
		g.Items = videoItems
		hp.Groups = append(hp.Groups, g)
	}
	return hp, nil
}

// HistoryGroups 返回历史分组摘要（供前端分组过滤下拉）
func (m *Manager) HistoryGroups(platform string) ([]*store.HistoryGroupSummary, error) {
	return m.store.ListHistoryGroups(platform)
}

// recordToTaskMerge 内存活跃态优先，否则用记录
func (m *Manager) recordToTaskMerge(r *store.TaskRecord) *Task {
	m.mu.RLock()
	if t, ok := m.tasks[r.ID]; ok {
		cp := *t
		cp.cancel = nil
		m.mu.RUnlock()
		return &cp
	}
	m.mu.RUnlock()
	return m.recordToTask(r)
}

// DeleteGroup 删除整组记录，removeFile=true 时同时删除组内磁盘文件
func (m *Manager) DeleteGroup(groupID string, removeFile bool) error {
	recs, _ := m.store.TasksByGroup(groupID)
	for _, r := range recs {
		if removeFile && r.FilePath != "" {
			_ = os.Remove(r.FilePath)
			_ = os.Remove(r.FilePath + ".part")
			_ = os.Remove(strings.TrimSuffix(r.FilePath, filepath.Ext(r.FilePath)) + ".json")
		}
		m.mu.Lock()
		if t, ok := m.tasks[r.ID]; ok {
			select {
			case t.cancel <- struct{}{}:
			default:
			}
			delete(m.tasks, r.ID)
		}
		m.mu.Unlock()
	}
	if _, err := m.store.DeleteGroup(groupID); err != nil {
		return err
	}
	return nil
}

func (m *Manager) recordToTask(r *store.TaskRecord) *Task {
	return &Task{
		ID:        r.ID,
		URL:       r.URL,
		PageURL:   r.PageURL,
		Title:     r.Title,
		Author:    r.Author,
		CoverURL:  r.CoverURL,
		FilePath:  r.FilePath,
		Status:    Status(r.Status),
		Progress:  r.Progress,
		Total:     r.Total,
		Loaded:    r.Loaded,
		Error:     r.Error,
		CreatedAt: store.ParseTime(r.CreatedAt),
		Kind:      r.Kind,
		Platform:  r.Platform,
		GroupID:   r.GroupID,
		GroupName: r.GroupName,
	}
}

func (m *Manager) toRecord(t *Task) *store.TaskRecord {
	return &store.TaskRecord{
		ID:        t.ID,
		URL:       t.URL,
		PageURL:   t.PageURL,
		Title:     t.Title,
		Author:    t.Author,
		CoverURL:  t.CoverURL,
		FilePath:  t.FilePath,
		Kind:      t.Kind,
		Status:    string(t.Status),
		Progress:  t.Progress,
		Total:     t.Total,
		Loaded:    t.Loaded,
		Error:     t.Error,
		Platform:  t.Platform,
		GroupID:   t.GroupID,
		GroupName: t.GroupName,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (m *Manager) persist(t *Task) {
	if m.store == nil {
		return
	}
	_ = m.store.UpsertTask(m.toRecord(t))
}

// Cancel 取消任务
func (m *Manager) Cancel(id string) error {
	m.mu.RLock()
	t, ok := m.tasks[id]
	m.mu.RUnlock()
	if !ok {
		return errors.New("任务不在下载中，无法取消")
	}
	select {
	case t.cancel <- struct{}{}:
	default:
	}
	m.setStatus(t, StatusCanceled, "")
	m.persist(t)
	return nil
}

// Delete 删除任务记录，removeFile=true 时同时删除磁盘文件
func (m *Manager) Delete(id string, removeFile bool) error {
	var path string
	if rec, _ := m.store.GetTask(id); rec != nil {
		path = rec.FilePath
	}
	if err := m.store.DeleteTask(id); err != nil {
		return err
	}
	if removeFile && path != "" {
		_ = os.Remove(path)
		_ = os.Remove(path + ".part")
		_ = os.Remove(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
		// 任务文件夹已空则一并移除（封面等其他文件仍在时 Remove 会失败，自动保留）
		_ = os.Remove(filepath.Dir(path))
	}
	m.mu.Lock()
	if t, ok := m.tasks[id]; ok {
		select {
		case t.cancel <- struct{}{}:
		default:
		}
		delete(m.tasks, id)
	}
	m.mu.Unlock()
	return nil
}

func (m *Manager) setStatus(t *Task, s Status, errMsg string) {
	m.mu.Lock()
	t.Status = s
	if errMsg != "" {
		t.Error = errMsg
	}
	if s == StatusDone {
		t.Progress = 100
	}
	m.mu.Unlock()
}

// Recover 启动时把上次未完成任务标记为 interrupted；
// autoRetry=true 时将它们重新排队下载（小文件直接重下）。
func (m *Manager) Recover(autoRetry bool) {
	if m.store == nil {
		return
	}
	// 先取出未完成记录，再统一标记
	pending, err := m.store.RecoverableTasks()
	if err != nil {
		return
	}
	_ = m.store.MarkInterrupted()
	if !autoRetry || len(pending) == 0 {
		return
	}
	for _, rec := range pending {
		t := m.recordToTask(rec)
		t.Status = StatusQueued
		t.Progress = 0
		t.Loaded = 0
		t.Speed = 0
		t.Error = ""
		t.cancel = make(chan struct{}, 1)
		m.mu.Lock()
		m.tasks[t.ID] = t
		m.mu.Unlock()
		m.persist(t)
		go m.run(t, nil)
	}
}

func (m *Manager) run(t *Task, meta *platform.Info) {
	select {
	case <-t.cancel:
		m.persist(t)
		return
	default:
	}

	m.sem <- struct{}{}
	defer func() { <-m.sem }()

	m.setStatus(t, StatusRunning, "")
	m.persist(t)
	if err := m.download(t); err != nil {
		if t.Status == StatusCanceled {
			m.persist(t)
			return
		}
		m.setStatus(t, StatusError, err.Error())
		m.persist(t)
		return
	}
	if t.Kind == "video" && meta != nil && config.Get().WriteMeta {
		_ = writeMeta(t.FilePath, meta)
	}
	m.setStatus(t, StatusDone, "")
	m.persist(t)
}

// 各平台下载时的防盗链来源
var platformOrigin = map[string]string{
	"kuaishou": "https://www.kuaishou.com",
	"douyin":   "https://www.douyin.com",
}

func (m *Manager) download(t *Task) error {
	req, err := http.NewRequest(http.MethodGet, t.URL, nil)
	if err != nil {
		return err
	}
	origin := platformOrigin[t.Platform]
	if origin == "" {
		origin = "https://www.kuaishou.com"
	}
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	if t.Platform == "douyin" {
		ua = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"
	}
	req.Header.Set("User-Agent", ua)
	if t.PageURL != "" {
		req.Header.Set("Referer", t.PageURL)
	} else {
		req.Header.Set("Referer", origin+"/")
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Range", "bytes=0-")
	if t.Cookie != "" {
		req.Header.Set("Cookie", t.Cookie)
	}

	resp, err := m.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("下载返回 HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength
	m.mu.Lock()
	t.Total = total
	m.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(t.FilePath), 0o755); err != nil {
		return err
	}
	tmp := t.FilePath + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	pr := &progressReader{
		rc:     resp.Body,
		task:   t,
		mgr:    m,
		cancel: t.cancel,
	}
	_, copyErr := io.Copy(f, pr)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}

	final := t.FilePath
	if _, err := os.Stat(final); err == nil {
		final = strings.TrimSuffix(final, filepath.Ext(final)) +
			"-" + time.Now().Format("150405") + filepath.Ext(final)
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	m.mu.Lock()
	t.FilePath = final
	m.mu.Unlock()
	return nil
}

type progressReader struct {
	rc       io.Reader
	task     *Task
	mgr      *Manager
	cancel   chan struct{}
	read     int64
	lastTick int64
	lastTime time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	select {
	case <-p.cancel:
		return 0, errors.New("已取消")
	default:
	}
	n, err := p.rc.Read(b)
	if n > 0 {
		p.read += int64(n)
		now := time.Now()
		shouldPersist := false
		p.mgr.mu.Lock()
		p.task.Loaded = p.read
		if p.task.Total > 0 {
			p.task.Progress = float64(p.read) / float64(p.task.Total) * 100
		}
		if !p.lastTime.IsZero() {
			dt := now.Sub(p.lastTime).Seconds()
			if dt >= 0.5 {
				p.task.Speed = int64(float64(p.read-p.lastTick) / dt)
				p.lastTick = p.read
				p.lastTime = now
			}
		} else {
			p.lastTime = now
			p.lastTick = p.read
		}
		if last, ok := p.mgr.lastPersist[p.task.ID]; !ok || now.Sub(last) >= 2*time.Second {
			p.mgr.lastPersist[p.task.ID] = now
			shouldPersist = true
		}
		p.mgr.mu.Unlock()
		if shouldPersist {
			p.mgr.persist(p.task)
		}
	}
	return n, err
}

func writeMeta(videoPath string, info *platform.Info) error {
	metaPath := strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + ".json"
	data := map[string]interface{}{
		"platform":  info.Platform,
		"id":        info.ID,
		"title":     info.Title,
		"author":    info.Author,
		"author_id": info.AuthorID,
		"page_url":  info.PageURL,
		"cover_url": info.CoverURL,
		"video_url": info.VideoURL,
		"width":     info.Width,
		"height":    info.Height,
		"duration":  info.Duration,
		"source":    info.Source,
		"saved_at":  time.Now().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, b, 0o644)
}
