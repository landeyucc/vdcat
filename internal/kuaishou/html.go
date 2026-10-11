package kuaishou

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// 快手短视频页面把数据内联在 window.__APOLLO_STATE__ = {...}; 中
var apolloReg = regexp.MustCompile(`window\.__APOLLO_STATE__\s*=\s*(\{.*?\})\s*(?:</script>|;\s*\(function)`)

// 兼容：部分版本字段名为 visionVideoDetailPhoto:<id>
var (
	photoURLReg = regexp.MustCompile(`"photoUrl"\s*:\s*"(https?:[^"]+?)"`)
	coverReg    = regexp.MustCompile(`"coverURL"\s*:\s*"(https?:[^"]+?)"`)
	captionReg  = regexp.MustCompile(`"caption"\s*:\s*"((?:\\.|[^"\\])*)"`)
	authorReg   = regexp.MustCompile(`"name"\s*:\s*"((?:\\.|[^"\\])*)"`)
	manifestReg = regexp.MustCompile(`"manifest"\s*:\s*\{`)
)

// parseViaHTML 方案二：直接拉取页面 HTML，从内联 JSON 提取直链
func (c *Client) parseViaHTML(pageURL string) (*VideoInfo, error) {
	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.browserHeaders(baseURL+"/")

	html, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("页面请求失败: %w", err)
	}
	s := string(html)

	info := &VideoInfo{PageURL: pageURL, Source: "html"}

	// 优先：定位 __APOLLO_STATE__ 并在其片段内搜索，减少误匹配
	var scope string
	if m := apolloReg.FindStringSubmatch(s); len(m) == 2 {
		scope = m[1]
	} else {
		// 退化为整页搜索
		scope = s
	}

	if m := photoURLReg.FindStringSubmatch(scope); len(m) == 2 {
		info.VideoURL = unescapeJSON(m[1])
	}
	if m := coverReg.FindStringSubmatch(scope); len(m) == 2 {
		info.CoverURL = unescapeJSON(m[1])
	}
	if m := captionReg.FindStringSubmatch(scope); len(m) == 2 {
		info.Title = strings.TrimSpace(unescapeJSON(m[1]))
	}
	if m := authorReg.FindStringSubmatch(scope); len(m) == 2 {
		info.Author = unescapeJSON(m[1])
	}

	// 若存在 adaptive/manifest（HLS）但无 mp4，记录错误提示
	if info.VideoURL == "" {
		if manifestReg.MatchString(scope) {
			return nil, errors.New("仅发现 HLS(manifest) 流，暂不支持该视频格式；可手动粘贴 mp4 直链")
		}
		// 识别风控/安全验证/登录墙
		low := strings.ToLower(s)
		switch {
		case strings.Contains(s, "安全验证") || strings.Contains(s, "滑块") ||
			strings.Contains(low, "captcha") || strings.Contains(s, "$kscdnwebprotector") ||
			strings.Contains(s, "cdn-web-protector"):
			return nil, errors.New("触发快手安全验证/风控：请在设置中填入浏览器 Cookie，或稍后再试；也可用手动直链")
		case strings.Contains(low, "passport") || strings.Contains(s, "登录后"):
			return nil, errors.New("页面要求登录：请在设置中填入快手 Cookie 后重试")
		case len(strings.TrimSpace(s)) < 200:
			return nil, errors.New("返回页面内容异常（疑似被 CDN 防护拦截）：请配置 Cookie 或稍后重试")
		}
		return nil, errors.New("HTML 中未提取到视频地址（页面可能需要Cookie或结构已变更）")
	}

	// 尝试在 APOLLO JSON 内补全宽高/时长（宽松）
	fillDimensions(scope, info)
	return info, nil
}

func unescapeJSON(p string) string {
	var out string
	if err := json.Unmarshal([]byte(`"`+p+`"`), &out); err == nil {
		return out
	}
	return p
}

var (
	widthReg  = regexp.MustCompile(`"width"\s*:\s*(\d+)`)
	heightReg = regexp.MustCompile(`"height"\s*:\s*(\d+)`)
	durReg    = regexp.MustCompile(`"duration"\s*:\s*(\d+)`)
)

func fillDimensions(scope string, info *VideoInfo) {
	if m := widthReg.FindStringSubmatch(scope); len(m) == 2 {
		fmt.Sscanf(m[1], "%d", &info.Width)
	}
	if m := heightReg.FindStringSubmatch(scope); len(m) == 2 {
		fmt.Sscanf(m[1], "%d", &info.Height)
	}
	if m := durReg.FindStringSubmatch(scope); len(m) == 2 {
		fmt.Sscanf(m[1], "%d", &info.Duration)
	}
}
