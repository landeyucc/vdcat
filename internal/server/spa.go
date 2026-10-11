package server

import (
	"io/fs"
	"net/http"
	"strings"
)

// spaHandler 托管前端单页应用，未知路径回退到 index.html
type spaHandler struct {
	content fs.FS
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}

	if f, err := h.content.Open(p); err == nil {
		_ = f.Close()
		http.FileServer(http.FS(h.content)).ServeHTTP(w, r)
		return
	}

	// 资源文件（带扩展名）不存在则 404，其余路径回退 index.html（前端路由）
	if ext := strings.LastIndex(p, "."); ext > strings.LastIndex(p, "/") {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(h.content, "index.html")
	if err != nil {
		http.Error(w, "前端未构建：请先执行 frontend 构建并放置到 internal/server/web", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
