package server

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 媒体 CDN 主机后缀白名单（防止 /api/media 被当作开放代理进行 SSRF）。
// 覆盖抖音系与快手系的视频/图片 CDN。
var mediaHostSuffixes = []string{
	// 抖音
	".douyinvod.com",
	".douyinpic.com",
	".douyincdn.com",
	".douyin.com",
	".iesdouyin.com",
	".amemv.com",
	".bytedance.com",
	".pstatp.com",
	".byteimg.com",
	".ixigua.com",
	// 快手
	".kuaishou.com",
	".kuaishouv.com",
	".gifshow.com",
	".yxixigua.com",
	".yxixiguavideo.com",
	".ksapisrv.com",
	// 快手视频/图片 CDN（直链 host 形如 *.djvod.ndcimgs.com、*.ndcimgs.com）
	".ndcimgs.com",
	".kwaicdn.com",
	".ksyuncs.com",
}

const (
	uaMediaMobile = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"
)

var mediaClient = &http.Client{
	Timeout: 60 * time.Second,
	// 视频流需要直连，不跟随异常跳转（CDN 一般不重定向）
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

func mediaAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, suf := range mediaHostSuffixes {
		if host == strings.TrimPrefix(suf, ".") || strings.HasSuffix(host, suf) {
			return true
		}
	}
	return false
}

var douyinHostKeys = []string{"douyin", "amemv", "iesdouyin", "bytedance", "pstatp", "byteimg", "ixigua"}

var kuaishouHostKeys = []string{"kuaishou", "gifshow", "ksapisrv", "ndcimgs", "kwaicdn", "ksyuncs", "yxixigua"}

func hostMatches(host string, keys []string) bool {
	h := strings.ToLower(host)
	for _, k := range keys {
		if strings.Contains(h, k) {
			return true
		}
	}
	return false
}

func isDouyinHost(host string) bool   { return hostMatches(host, douyinHostKeys) }
func isKuaishouHost(host string) bool { return hostMatches(host, kuaishouHostKeys) }

func mediaUA(host string) string {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "kuaishou"), strings.Contains(h, "gifshow"), strings.Contains(h, "ks"):
		// 快手 CDN 用移动 UA 最稳
		return uaMediaMobile
	default:
		return uaMediaMobile
	}
}

// mediaProxy 同源代理远程视频流，解决抖音/快手 CDN 在浏览器直连时
// 因防盗链/环境校验导致的 net::ERR_ABORTED、无法在线预览问题。
// 透传 Range，支持视频拖动与分段加载。
//
//	GET /api/media?url=<编码后的远程直链>
func (s *Server) mediaProxy(w http.ResponseWriter, r *http.Request) {
	// 该路径在登录中间件白名单内，鉴权在此自行完成。
	// 支持：登录 token（Authorization: Bearer / query token）、
	// 扩展 API Key（X-Api-Key / query api_key）。
	// 未开启登录鉴权时直接放行。
	if s.auther.Enabled() {
		if !s.mediaAuthorized(r) {
			fail(w, 401, "未授权或登录已过期")
			return
		}
	}

	raw := r.URL.Query().Get("url")
	if raw == "" {
		fail(w, 400, "缺少 url 参数")
		return
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		fail(w, 400, "非法 url")
		return
	}
	if !mediaAllowed(u.Host) {
		fail(w, 403, "不允许的媒体主机")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, raw, nil)
	if err != nil {
		fail(w, 400, "构建上游请求失败")
		return
	}
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	req.Header.Set("User-Agent", mediaUA(u.Host))
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	switch {
	case isDouyinHost(u.Host):
		req.Header.Set("Referer", "https://www.douyin.com/")
	case isKuaishouHost(u.Host):
		req.Header.Set("Referer", "https://www.kuaishou.com/")
	}

	resp, err := mediaClient.Do(req)
	if err != nil {
		fail(w, 502, "媒体上游请求失败")
		return
	}
	defer resp.Body.Close()

	// 透传关键响应头
	for _, h := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Last-Modified",
		"ETag",
		"Cache-Control",
	} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	// 上游可能带 Content-Disposition（如 filename="media.mp4"），
	// 会覆盖调用方期望的文件名，这里先清除，再按需自行设置。
	w.Header().Del("Content-Disposition")
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	// 下载场景：调用方通过 dl 传入期望文件名（含扩展名），
	// 输出 attachment + RFC 5987 编码，确保中文/特殊字符文件名在浏览器生效。
	if dl := strings.TrimSpace(r.URL.Query().Get("dl")); dl != "" {
		w.Header().Set("Content-Disposition", mediaContentDisposition(dl))
	}
	// 同源响应，允许播放器读取
	w.Header().Set("Access-Control-Allow-Origin", "*")

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// mediaContentDisposition 构造安全的 Content-Disposition 头值。
// filename* 使用 RFC 5987 百分号编码以支持中文；同时给一个 ASCII 兜底 filename。
func mediaContentDisposition(name string) string {
	// 去掉路径分隔符，避免文件名注入
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.TrimSpace(name)
	if name == "" {
		name = "media.mp4"
	}
	ascii := toASCIIFilename(name)
	return "attachment; filename=\"" + ascii + "\"; filename*=UTF-8''" + url.QueryEscape(name)
}

// toASCIIFilename 生成纯 ASCII 兜底文件名：非 ASCII 字符替换为下划线。
func toASCIIFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == '"' || r == 0x7f {
			b.WriteByte('_')
			continue
		}
		if r > 0x7e {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	s := strings.Trim(b.String(), ". ")
	if s == "" || s == "_" {
		return "media.mp4"
	}
	return s
}

// mediaAuthorized 校验媒体代理请求：登录会话 token 或扩展 API Key 任一通过即可。
func (s *Server) mediaAuthorized(r *http.Request) bool {
	// 1) 登录 token：Authorization: Bearer <token> 或 query token
	var token string
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token != "" && s.auther.Verify(token) {
		return true
	}

	// 2) 扩展 API Key：X-Api-Key 头或 query api_key
	var key string
	if k := strings.TrimSpace(r.Header.Get("X-Api-Key")); k != "" {
		key = k
	}
	if key == "" {
		key = strings.TrimSpace(r.URL.Query().Get("api_key"))
	}
	if key != "" {
		if enabled, err := s.store.APIKeyEnabled(key); err == nil && enabled {
			return true
		}
	}
	return false
}
