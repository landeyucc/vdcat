package platform

import (
	"errors"
	"regexp"
	"strings"
)

// Platform 平台标识
type Platform string

const (
	Auto     Platform = "auto"
	Kuaishou Platform = "kuaishou"
	Douyin   Platform = "douyin"
)

// Info 统一的视频元数据（各平台解析结果映射到该结构）
type Info struct {
	Platform Platform `json:"platform"`
	PageURL  string   `json:"page_url"`
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	AuthorID string   `json:"author_id"`
	CoverURL string   `json:"cover_url"`
	VideoURL string   `json:"video_url"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	Duration int      `json:"duration"` // 毫秒
	Source   string   `json:"source"`
}

const (
	ModeAuto = "auto" // 自动：按平台默认顺序尝试，失败在方案间兜底
	ModeAPI  = "api"  // 仅/优先接口解析
	ModeHTML = "html" // 仅/优先 HTML 提取
)

// NormalizeMode 归一化解析方式，非法值回退 auto
func NormalizeMode(m string) string {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case ModeAPI, ModeHTML:
		return strings.ToLower(strings.TrimSpace(m))
	default:
		return ModeAuto
	}
}

// Parser 平台解析器统一接口
type Parser interface {
	Platform() Platform
	// Parse 用单个 cookie（可为空）解析；实现应只负责一次尝试，cookie 轮转由上层负责。
	// mode 指定解析方式（auto/api/html）；force=true 时严格只用所选方式，失败不兜底切换其它方案。
	Parse(rawURL, cookie string, mode string, force bool) (*Info, error)
}

// ErrAuth 需要有效 Cookie / 触发风控（用于决定是否切换下一个 cookie）
var ErrAuth = errors.New("需要有效Cookie或触发风控")

// IsAuthError 判断是否为鉴权类错误（应切换 cookie）
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if errors.Is(err, ErrAuth) {
		return true
	}
	for _, kw := range []string{"Cookie", "cookie", "登录", "风控", "验证", "403", "401", "未授权", "CDN", "拦截"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// 仅识别 https 链接（http 及其它任何非 https 协议一律忽略），兼容整段分享口令/换行文本。
// 排除空白、常见中英文标点与括号，避免把链接后的文字或标点卷入。
var httpsReg = regexp.MustCompile(`https://[^\s，。、；“”"'<>（）()\[\]{}]+`)

func trimURL(m string) string {
	return strings.TrimRight(m, ".,;:!?，。、；！？")
}

// ExtractURLs 从可能包含分享文案的输入中提取全部 https 链接（去重，保持顺序）。
// 不含 https 链接时返回 nil —— 纯口令文字、http 链接等都会被忽略。
func ExtractURLs(input string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range httpsReg.FindAllString(input, -1) {
		u := trimURL(m)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// ExtractURL 取第一个 https 链接；不存在返回空串（绝不把原文当链接）。
func ExtractURL(input string) string {
	if us := ExtractURLs(input); len(us) > 0 {
		return us[0]
	}
	return ""
}

var (
	ksHostReg  = regexp.MustCompile(`(?:^|[\./])(kuaishou|chenzhongtech|gifshow)\.(?:com|cn)`)
	ksVideoReg = regexp.MustCompile(`(?:short-video|fw/photo|photo)/([A-Za-z0-9_-]+)`)
	dyHostReg  = regexp.MustCompile(`(?:^|[\./])(douyin|iesdouyin|amemv)\.(?:com|cn)`)
	dyVideoReg = regexp.MustCompile(`/video/(\d+)`)
	dyNoteReg  = regexp.MustCompile(`/note/(\d+)`)
	dyShareReg = regexp.MustCompile(`/share/video/(\d+)`)
)

// Detect 从输入（链接或分享文本）识别平台；无法识别返回空串
func Detect(input string) Platform {
	u := strings.ToLower(ExtractURL(input))
	if u == "" {
		return ""
	}
	switch {
	case ksHostReg.MatchString(u):
		return Kuaishou
	case dyHostReg.MatchString(u):
		return Douyin
	}
	// 纯数字 ID（>=15 位通常是抖音 aweme id；快手短 id 形如 3xn4ybn... 字母数字）
	plain := strings.TrimSpace(input)
	if regexp.MustCompile(`^\d{15,}$`).MatchString(plain) {
		return Douyin
	}
	return ""
}

// Normalize 解析手动指定的平台字符串
func Normalize(s string) Platform {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "kuaishou", "ks", "快手":
		return Kuaishou
	case "douyin", "dy", "抖音":
		return Douyin
	default:
		return Auto
	}
}

func (p Platform) Label() string {
	switch p {
	case Kuaishou:
		return "快手"
	case Douyin:
		return "抖音"
	default:
		return "未知"
	}
}

// 供各平台包复用的 ID 提取辅助
func MatchID(r *regexp.Regexp, s string) string {
	if m := r.FindStringSubmatch(s); len(m) == 2 {
		return m[1]
	}
	return ""
}

// 暴露已编译正则给平台包
var (
	KSVideoRegexp = ksVideoReg
	DYVideoRegexp = dyVideoReg
	DYNoteRegexp  = dyNoteReg
	DYShareRegexp = dyShareReg
)
