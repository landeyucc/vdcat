package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"vdcat/internal/auth"
	"vdcat/internal/config"
	"vdcat/internal/downloader"
	"vdcat/internal/files"
	"vdcat/internal/platform"
	"vdcat/internal/resolver"
	"vdcat/internal/store"
)

//go:embed all:web
var webFS embed.FS

// Server HTTP 服务
type Server struct {
	mgr      *downloader.Manager
	store    *store.Store
	auther   *auth.Authenticator
	resolver *resolver.Resolver
}

func New(mgr *downloader.Manager, st *store.Store, au *auth.Authenticator) *Server {
	return &Server{
		mgr:      mgr,
		store:    st,
		auther:   au,
		resolver: resolver.New(st),
	}
}

type apiResp struct {
	OK   bool        `json:"ok"`
	Data interface{} `json:"data,omitempty"`
	Err  string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, apiResp{OK: true, Data: data})
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, apiResp{OK: false, Err: msg})
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("POST /api/config", s.saveConfig)
	mux.HandleFunc("GET /api/cookies", s.listCookies)
	mux.HandleFunc("POST /api/cookies", s.createCookie)
	mux.HandleFunc("PUT /api/cookies/{id}", s.updateCookie)
	mux.HandleFunc("POST /api/cookies/{id}/toggle", s.toggleCookie)
	mux.HandleFunc("DELETE /api/cookies/{id}", s.deleteCookie)
	mux.HandleFunc("POST /api/parse", s.parse)
	mux.HandleFunc("POST /api/download", s.download)
	mux.HandleFunc("POST /api/download-direct", s.downloadDirect)
	mux.HandleFunc("GET /api/tasks/active", s.listActive)
	mux.HandleFunc("GET /api/history", s.history)
	mux.HandleFunc("GET /api/history/groups", s.historyGroups)
	mux.HandleFunc("DELETE /api/groups/{id}", s.deleteGroup)
	mux.HandleFunc("POST /api/tasks/{id}/cancel", s.cancelTask)
	mux.HandleFunc("POST /api/tasks/{id}/retry", s.retryTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.deleteTask)

	mux.HandleFunc("GET /api/files", s.listFiles)
	mux.HandleFunc("DELETE /api/files", s.deleteFile)
	mux.HandleFunc("GET /api/files/raw", s.rawFile)

	// API Key 管理（受登录鉴权保护）
	mux.HandleFunc("GET /api/api-keys", s.listAPIKeys)
	mux.HandleFunc("POST /api/api-keys", s.createAPIKey)
	mux.HandleFunc("POST /api/api-keys/{id}/toggle", s.toggleAPIKey)
	mux.HandleFunc("POST /api/api-keys/{id}/rotate", s.rotateAPIKey)
	mux.HandleFunc("DELETE /api/api-keys/{id}", s.deleteAPIKey)

	// 浏览器扩展等外部程序调用（独立 X-Api-Key 鉴权）
	mux.HandleFunc("POST /api/external/parse", s.withAPIKey(s.extParse))
	mux.HandleFunc("POST /api/external/download", s.withAPIKey(s.extDownload))
	mux.HandleFunc("GET /api/external/tasks", s.withAPIKey(s.extTasks))

	// 在线预览媒体流代理（浏览器直连抖音/快手 CDN 会被防盗链拦截）
	mux.HandleFunc("GET /api/media", s.mediaProxy)

	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", spaHandler{sub})

	// 鉴权包裹（登录/health 内部放行）
	return withCORS(s.auther.Middleware(mux))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]interface{}{"auth_required": s.auther.Enabled()})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, 400, "请求体错误")
		return
	}
	// 允许直接用静态 token 登录
	if in.Token != "" && s.auther.Verify(strings.TrimSpace(in.Token)) {
		ok(w, map[string]string{"token": strings.TrimSpace(in.Token)})
		return
	}
	tok, err := s.auther.Login(in.Username, in.Password)
	if err != nil {
		fail(w, 401, err.Error())
		return
	}
	ok(w, map[string]string{"token": tok})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	c := config.Get()
	ok(w, map[string]interface{}{
		"cookie_set":    c.Cookie != "",
		"cookie":        c.Cookie,
		"download_dir":  c.DownloadDir,
		"concurrency":   c.Concurrency,
		"use_api":       c.UseAPI,
		"parse_mode":    c.ParseMode,
		"force_parse":   c.ForceParse,
		"write_meta":    c.WriteMeta,
		"auth_required": c.AuthEnabled,
	})
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Cookie      *string `json:"cookie"`
		DownloadDir *string `json:"download_dir"`
		Concurrency *int    `json:"concurrency"`
		UseAPI      *bool   `json:"use_api"`
		ParseMode   *string `json:"parse_mode"`
		ForceParse  *bool   `json:"force_parse"`
		WriteMeta   *bool   `json:"write_meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, 400, "请求体格式错误")
		return
	}
	cur := config.Get()
	if in.Cookie != nil {
		cur.Cookie = strings.TrimSpace(*in.Cookie)
	}
	if in.DownloadDir != nil && strings.TrimSpace(*in.DownloadDir) != "" {
		cur.DownloadDir = *in.DownloadDir
	}
	if in.Concurrency != nil && *in.Concurrency > 0 {
		cur.Concurrency = *in.Concurrency
	}
	if in.UseAPI != nil {
		cur.UseAPI = *in.UseAPI
	}
	if in.ParseMode != nil {
		cur.ParseMode = platform.NormalizeMode(*in.ParseMode)
	}
	if in.ForceParse != nil {
		cur.ForceParse = *in.ForceParse
	}
	if in.WriteMeta != nil {
		cur.WriteMeta = *in.WriteMeta
	}
	saved, err := config.Save(cur)
	if err != nil {
		fail(w, 500, "保存失败: "+err.Error())
		return
	}
	if in.Concurrency != nil {
		s.mgr.SetConcurrency(saved.Concurrency)
	}
	ok(w, map[string]interface{}{
		"cookie_set":   saved.Cookie != "",
		"cookie":       saved.Cookie,
		"download_dir": saved.DownloadDir,
		"concurrency":  saved.Concurrency,
		"use_api":      saved.UseAPI,
		"parse_mode":   saved.ParseMode,
		"force_parse":  saved.ForceParse,
		"write_meta":   saved.WriteMeta,
	})
}

func (s *Server) parse(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL        string  `json:"url"`
		Platform   string  `json:"platform"`    // auto|kuaishou|douyin
		ParseMode  *string `json:"parse_mode"`  // 可选覆盖；默认取服务端配置
		ForceParse *bool   `json:"force_parse"` // 可选覆盖；默认取服务端配置
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.URL) == "" {
		fail(w, 400, "请提供视频链接或分享文本")
		return
	}
	cfg := config.Get()
	mode := cfg.ParseMode
	force := cfg.ForceParse
	if in.ParseMode != nil {
		mode = platform.NormalizeMode(*in.ParseMode)
	}
	if in.ForceParse != nil {
		force = *in.ForceParse
	}
	pf := platform.Normalize(in.Platform) // 空/非法 => auto
	result, err := s.resolver.Resolve(strings.TrimSpace(in.URL), pf, mode, force)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	ok(w, result)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Platform  string `json:"platform"`
		VideoURL  string `json:"video_url"`
		CoverURL  string `json:"cover_url"`
		PageURL   string `json:"page_url"`
		Title     string `json:"title"`
		Author    string `json:"author"`
		AuthorID  string `json:"author_id"`
		ID        string `json:"id"`
		Cookie    string `json:"cookie"`   // 解析成功实际使用的 cookie
		GroupID   string `json:"group_id"` // 批量下载组 id（同一次提交共享）
		GroupName string `json:"group_name"`
		Cover     bool   `json:"cover"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.VideoURL == "" {
		fail(w, 400, "缺少视频地址")
		return
	}
	pf := platform.Normalize(in.Platform)
	if pf == platform.Auto || pf == "" {
		pf = platform.Detect(in.PageURL + " " + in.VideoURL)
	}
	if pf == "" {
		pf = platform.Kuaishou
	}
	dir := config.Get().DownloadDir
	info := &platform.Info{
		Platform: pf,
		ID:       in.ID,
		Title:    in.Title,
		Author:   in.Author,
		AuthorID: in.AuthorID,
		PageURL:  in.PageURL,
		CoverURL: in.CoverURL,
		VideoURL: in.VideoURL,
	}
	groupID, groupName := in.GroupID, in.GroupName
	if strings.TrimSpace(groupID) == "" {
		groupID, groupName = downloader.NewGroup(pf)
	}
	video, cover := s.mgr.Enqueue(info, in.Cookie, dir, groupID, groupName)
	resp := map[string]interface{}{
		"video_id":   video.ID,
		"platform":   string(pf),
		"group_id":   groupID,
		"group_name": groupName,
	}
	if cover != nil {
		if in.Cover {
			resp["cover_id"] = cover.ID
		} else {
			_ = s.mgr.Cancel(cover.ID)
		}
	}
	ok(w, resp)
}

// ---------- Cookie 管理 ----------

func validPlatform(p string) (string, bool) {
	pf := platform.Normalize(p)
	if pf != platform.Kuaishou && pf != platform.Douyin {
		return "", false
	}
	return string(pf), true
}

func (s *Server) listCookies(w http.ResponseWriter, r *http.Request) {
	pf := strings.TrimSpace(r.URL.Query().Get("platform"))
	list, err := s.store.ListCookies(pf)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if list == nil {
		list = []*store.CookieRecord{}
	}
	// 不回传完整 cookie 明文？本地工具仍回显便于编辑；仅在列表给出截断预览，编辑用单独内容
	type item struct {
		*store.CookieRecord
		Preview string `json:"preview"`
	}
	out := make([]item, 0, len(list))
	for _, c := range list {
		pv := c.Content
		if len([]rune(pv)) > 24 {
			pv = string([]rune(pv)[:24]) + "…"
		}
		out = append(out, item{CookieRecord: c, Preview: pv})
	}
	ok(w, out)
}

func (s *Server) createCookie(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Platform  string `json:"platform"`
		Content   string `json:"content"`
		Label     string `json:"label"`
		Enabled   *bool  `json:"enabled"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, 400, "请求体错误")
		return
	}
	pf, okP := validPlatform(in.Platform)
	if !okP {
		fail(w, 400, "platform 必须为 kuaishou 或 douyin")
		return
	}
	content := strings.TrimSpace(in.Content)
	if content == "" {
		fail(w, 400, "Cookie 内容不能为空")
		return
	}
	en := true
	if in.Enabled != nil {
		en = *in.Enabled
	}
	id, err := s.store.CreateCookie(pf, content, strings.TrimSpace(in.Label), en, in.SortOrder)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	ok(w, map[string]interface{}{"id": id})
}

func (s *Server) updateCookie(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	var in struct {
		Platform  string `json:"platform"`
		Content   string `json:"content"`
		Label     string `json:"label"`
		Enabled   *bool  `json:"enabled"`
		SortOrder int    `json:"sort_order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, 400, "请求体错误")
		return
	}
	pf, okP := validPlatform(in.Platform)
	if !okP {
		fail(w, 400, "platform 必须为 kuaishou 或 douyin")
		return
	}
	en := true
	if in.Enabled != nil {
		en = *in.Enabled
	}
	if err := s.store.UpdateCookie(id, pf, strings.TrimSpace(in.Content), strings.TrimSpace(in.Label), en, in.SortOrder); err != nil {
		fail(w, 404, err.Error())
		return
	}
	ok(w, map[string]int64{"id": id})
}

func (s *Server) toggleCookie(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if err := s.store.SetCookieEnabled(id, in.Enabled); err != nil {
		fail(w, 404, err.Error())
		return
	}
	ok(w, map[string]interface{}{"id": id, "enabled": in.Enabled})
}

func (s *Server) deleteCookie(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.store.DeleteCookie(id); err != nil {
		fail(w, 404, err.Error())
		return
	}
	ok(w, map[string]int64{"id": id})
}

func (s *Server) downloadDirect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL     string `json:"url"`
		Referer string `json:"referer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.URL) == "" {
		fail(w, 400, "请提供直链")
		return
	}
	cfg := config.Get()
	t := s.mgr.EnqueueDirect(strings.TrimSpace(in.URL), strings.TrimSpace(in.Referer), cfg.Cookie, cfg.DownloadDir)
	ok(w, map[string]string{"task_id": t.ID})
}

func (s *Server) listActive(w http.ResponseWriter, r *http.Request) {
	list := s.mgr.ListActive()
	if list == nil {
		list = []*downloader.Task{}
	}
	ok(w, list)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	pf := historyPlatformFilter(q.Get("platform"))
	groupID := strings.TrimSpace(q.Get("group_id"))
	hp, err := s.mgr.History(page, size, pf, groupID)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	ok(w, hp)
}

func (s *Server) historyGroups(w http.ResponseWriter, r *http.Request) {
	pf := historyPlatformFilter(r.URL.Query().Get("platform"))
	list, err := s.mgr.HistoryGroups(pf)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if list == nil {
		list = []*store.HistoryGroupSummary{}
	}
	ok(w, list)
}

// historyPlatformFilter 仅接受已知平台，其余（含空/auto）返回空串表示不过滤
func historyPlatformFilter(p string) string {
	if pf, ok := validPlatform(strings.TrimSpace(p)); ok {
		return pf
	}
	return ""
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.PathValue("id"))
	if groupID == "" {
		fail(w, 400, "缺少组 id")
		return
	}
	removeFile := r.URL.Query().Get("file") == "1"
	if err := s.mgr.DeleteGroup(groupID, removeFile); err != nil {
		fail(w, 500, err.Error())
		return
	}
	ok(w, map[string]string{"id": groupID})
}

func (s *Server) cancelTask(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.Cancel(r.PathValue("id")); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ok(w, map[string]string{"id": r.PathValue("id")})
}

func (s *Server) retryTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.mgr.Retry(r.PathValue("id"))
	if err != nil {
		fail(w, 404, err.Error())
		return
	}
	ok(w, map[string]string{"id": t.ID, "status": string(t.Status)})
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	removeFile := r.URL.Query().Get("file") == "1"
	if err := s.mgr.Delete(r.PathValue("id"), removeFile); err != nil {
		fail(w, 404, err.Error())
		return
	}
	ok(w, map[string]string{"id": r.PathValue("id")})
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	root := config.Get().DownloadDir
	list, err := files.List(root)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	ok(w, map[string]interface{}{"root": root, "files": list})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if strings.TrimSpace(rel) == "" {
		fail(w, 400, "缺少 path")
		return
	}
	if err := files.Delete(config.Get().DownloadDir, rel); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ok(w, map[string]string{"deleted": rel})
}

func (s *Server) rawFile(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	abs, err := files.Resolve(config.Get().DownloadDir, rel)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if _, err := os.Stat(abs); err != nil {
		fail(w, 404, "文件不存在")
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, abs)
}

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("非法 id")
	}
	return id, nil
}

// Listen 按给定 addr 监听并启动服务
func Listen(addr string, handler http.Handler) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() { _ = http.Serve(ln, handler) }()
	return ln, nil
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Api-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
