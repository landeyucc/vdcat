package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"vdcat/internal/config"
	"vdcat/internal/downloader"
	"vdcat/internal/platform"
	"vdcat/internal/store"
)

// genAPIKey 生成带前缀的随机 key（vk_ + 32 字节十六进制）
func genAPIKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "vk_" + hex.EncodeToString(b)
}

// ---------- API Key 管理（受登录鉴权保护，挂在 /api/api-keys） ----------

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListAPIKeys()
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	if list == nil {
		list = []*store.APIKeyRecord{}
	}
	// 直接回传便于页面展示/复制；这是本地自托管工具
	ok(w, list)
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	key := genAPIKey()
	id, err := s.store.CreateAPIKey(strings.TrimSpace(in.Name), key, true)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	ok(w, map[string]interface{}{"id": id, "name": in.Name, "api_key": key, "enabled": true})
}

func (s *Server) toggleAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, 400, "请求体格式错误")
		return
	}
	if err := s.store.SetAPIKeyEnabled(id, in.Enabled); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ok(w, map[string]interface{}{"id": id, "enabled": in.Enabled})
}

func (s *Server) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	key := genAPIKey()
	if err := s.store.RotateAPIKey(id, key); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ok(w, map[string]interface{}{"id": id, "api_key": key})
}

func (s *Server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.store.DeleteAPIKey(id); err != nil {
		fail(w, 400, err.Error())
		return
	}
	ok(w, map[string]int64{"id": id})
}

// ---------- 外部开放接口（X-Api-Key 鉴权，挂在 /api/external） ----------

// withAPIKey 校验 X-Api-Key（也兼容 query api_key / Authorization: Bearer <key>）
func (s *Server) withAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-Api-Key"))
		if key == "" {
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				key = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			}
		}
		if key == "" {
			key = strings.TrimSpace(r.URL.Query().Get("api_key"))
		}
		if key == "" {
			fail(w, 401, "缺少 API Key（X-Api-Key）")
			return
		}
		enabled, err := s.store.APIKeyEnabled(key)
		if err != nil || !enabled {
			fail(w, 401, "API Key 无效或已停用")
			return
		}
		s.store.TouchAPIKey(key)
		next(w, r)
	}
}

// extParse 外部解析：POST /api/external/parse {url, platform?}
func (s *Server) extParse(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL      string `json:"url"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.URL) == "" {
		fail(w, 400, "请提供视频链接或分享文本（url）")
		return
	}
	cfg := config.Get()
	pf := platform.Normalize(in.Platform)
	result, err := s.resolver.Resolve(strings.TrimSpace(in.URL), pf, cfg.ParseMode, cfg.ForceParse)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	ok(w, result)
}

// extDownload 外部解析并直接入队下载：POST /api/external/download {url, platform?, cover?}
func (s *Server) extDownload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL      string `json:"url"`
		Platform string `json:"platform"`
		Cover    bool   `json:"cover"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.URL) == "" {
		fail(w, 400, "请提供视频链接或分享文本（url）")
		return
	}
	cfg := config.Get()
	pf := platform.Normalize(in.Platform)
	result, err := s.resolver.Resolve(strings.TrimSpace(in.URL), pf, cfg.ParseMode, cfg.ForceParse)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	info := result.Info
	dir := config.Get().DownloadDir
	groupID, groupName := downloader.NewGroup(result.Platform)
	video, coverTask := s.mgr.Enqueue(info, result.CookieContent, dir, groupID, groupName)
	resp := map[string]interface{}{
		"video_id":   video.ID,
		"platform":   string(result.Platform),
		"group_id":   groupID,
		"group_name": groupName,
		"title":      info.Title,
		"author":     info.Author,
	}
	if coverTask != nil {
		if in.Cover {
			resp["cover_id"] = coverTask.ID
		} else {
			_ = s.mgr.Cancel(coverTask.ID)
		}
	}
	ok(w, resp)
}

// extTasks 外部任务状态：GET /api/external/tasks
func (s *Server) extTasks(w http.ResponseWriter, r *http.Request) {
	list := s.mgr.ListActive()
	if list == nil {
		list = []*downloader.Task{}
	}
	ok(w, list)
}
