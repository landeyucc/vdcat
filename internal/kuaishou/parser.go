package kuaishou

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"vdcat/internal/platform"

	"github.com/andybalholm/brotli"
)

var (
	idReg    = regexp.MustCompile(`short-video/([A-Za-z0-9_-]+)`)
	shortReg = regexp.MustCompile(`kuaishou\.com/(?:f|fw|short-video)/[A-Za-z0-9_-]+`)
)

// ErrInvalidURL 链接无法识别
var ErrInvalidURL = errors.New("无法从链接中识别快手视频ID")

// Parse 解析视频。
//   - mode=api：走 GraphQL 接口；mode=html：走页面 HTML 提取；mode=auto：先接口后 HTML。
//   - force=true 时严格只用所选方式，失败不再兜底切换其它方案（auto 模式下 force 无意义，仍按自动兜底）。
func (c *Client) Parse(rawURL string, mode string, force bool) (*VideoInfo, error) {
	id := extractID(rawURL)
	// /f/xxx 等分享短链不含 short-video 视频ID，需先跟随重定向拿到真实页面地址
	if id == "" && isShortLink(rawURL) {
		final, err := c.expandShortLink(rawURL)
		if err == nil {
			id = extractID(final)
			rawURL = final
		}
	}
	if id == "" {
		return nil, ErrInvalidURL
	}
	pageURL := fmt.Sprintf(videoPage, id)

	switch mode {
	case platform.ModeAPI:
		return c.parseViaGraphQL2(id, pageURL, force)
	case platform.ModeHTML:
		info, err := c.parseViaHTML(pageURL)
		if err != nil {
			return nil, err
		}
		info.ID = id
		info.PageURL = pageURL
		return info, nil
	default: // auto：先接口后 HTML 兜底
		if info, err := c.parseViaGraphQL(id, pageURL); err == nil && info.VideoURL != "" {
			return info, nil
		}
		info, err := c.parseViaHTML(pageURL)
		if err != nil {
			return nil, err
		}
		info.ID = id
		info.PageURL = pageURL
		return info, nil
	}
}

// parseViaGraphQL2 仅接口解析；force=true 时即使失败也不回退 HTML。
func (c *Client) parseViaGraphQL2(id, pageURL string, force bool) (*VideoInfo, error) {
	info, err := c.parseViaGraphQL(id, pageURL)
	if err == nil && info.VideoURL != "" {
		return info, nil
	}
	if force {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("接口解析未获取到视频地址（已强制接口模式，不切换 HTML）")
	}
	// 非强制时回退 HTML
	hinfo, herr := c.parseViaHTML(pageURL)
	if herr != nil {
		if err != nil {
			return nil, err
		}
		return nil, herr
	}
	hinfo.ID = id
	hinfo.PageURL = pageURL
	return hinfo, nil
}

func extractID(raw string) string {
	if m := idReg.FindStringSubmatch(raw); len(m) == 2 {
		return m[1]
	}
	// 纯ID
	s := strings.TrimSpace(raw)
	if s != "" && !strings.Contains(s, "/") && !strings.Contains(s, " ") {
		return s
	}
	return ""
}

// isShortLink 判断是否为需要跟随重定向的快手分享短链（如 https://www.kuaishou.com/f/xxx）
func isShortLink(raw string) bool {
	return strings.Contains(raw, "/f/") || shortReg.MatchString(raw)
}

// expandShortLink 跟随短链重定向，返回最终落地的真实 URL。
// 优先从 30x 的 Location 直接取得 short-video 地址；若服务端直接 200 落地，
// 则用响应最终 Request.URL；都拿不到时兜底扫描响应体中的 short-video 链接。
func (c *Client) expandShortLink(link string) (string, error) {
	link = strings.TrimSpace(link)
	if m := linkPattern(link); m != "" {
		link = m
	}
	req, err := http.NewRequest(http.MethodGet, link, nil)
	if err != nil {
		return "", err
	}
	req.Header = c.browserHeaders("")

	var landed string
	// 用独立 client，自定义重定向策略：命中含 short-video 的 Location 即停止
	hc := &http.Client{
		Jar:     c.hc.Jar,
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			loc := req.URL.String()
			if extractID(loc) != "" {
				landed = loc
				return http.ErrUseLastResponse
			}
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	tr, _ := c.hc.Transport.(*http.Transport)
	if tr != nil {
		hc.Transport = tr
	}

	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if landed != "" {
		return landed, nil
	}
	// 30x 但未在 CheckRedirect 命中（直接返回最后响应）
	if loc := resp.Header.Get("Location"); loc != "" {
		if u, perr := resp.Request.URL.Parse(loc); perr == nil && extractID(u.String()) != "" {
			return u.String(), nil
		}
	}
	// 200 落地：从最终 URL 取
	if final := resp.Request.URL.String(); extractID(final) != "" {
		return final, nil
	}
	// 兜底：扫描响应体中的 short-video 链接
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if m := idReg.FindString(string(body)); m != "" {
		// idReg 匹配的是 short-video/xxx，补全前缀
		return "https://www.kuaishou.com/" + m, nil
	}
	return "", errors.New("短链重定向后未找到视频地址")
}

var urlReg = regexp.MustCompile(`https://[^\s，。、！？；：""''（）()]+`)

// linkPattern 从包含中文/口令的整段文本中提取第一个 https 链接
func linkPattern(raw string) string {
	if m := urlReg.FindString(raw); m != "" {
		return strings.TrimRight(m, `.,)）】]`)
	}
	return ""
}

// do 执行请求并按 Content-Encoding 解压响应体（gzip / br / deflate）
func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch enc {
	case "gzip":
		gz, gerr := gzip.NewReader(resp.Body)
		if gerr != nil {
			return nil, gerr
		}
		defer gz.Close()
		reader = gz
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "deflate":
		reader = flate.NewReader(resp.Body)
	case "", "identity":
	default:
		// 未知编码，按原始字节读取
		reader = resp.Body
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(body, 200))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---------- 方案一：GraphQL 接口 ----------

type graphqlPayload struct {
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Query         string                 `json:"query"`
}

const visionDetailQuery = `query visionVideoDetail($photoId: String, $page: String, $webPageArea: String) {
  visionVideoDetail(photoId: $photoId, page: $page, webPageArea: $webPageArea) {
    photo {
      id
      duration
      caption
      photoUrl
      coverUrl
    }
  }
}`

func (c *Client) parseViaGraphQL(id, pageURL string) (*VideoInfo, error) {
	// 先访问一次页面拿上下文 cookie（接口依赖 did 等）
	pageReq, _ := http.NewRequest(http.MethodGet, pageURL, nil)
	pageReq.Header = c.browserHeaders("")
	_, _ = c.do(pageReq) // 忽略错误，仅为播种 cookie

	payload := graphqlPayload{
		OperationName: "visionVideoDetail",
		Variables: map[string]interface{}{
			"photoId":     id,
			"page":        "detail",
			"webPageArea": "detail",
		},
		Query: visionDetailQuery,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, graphqlURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	h := c.browserHeaders(pageURL)
	h.Set("Accept", "*/*")
	h.Set("Content-Type", "application/json")
	h.Set("Origin", baseURL)
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	h.Set("Accept-Encoding", "gzip, deflate")
	req.Header = h

	respBody, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL 请求失败: %w", err)
	}

	var resp struct {
		Data struct {
			VisionVideoDetail struct {
				Photo *struct {
					ID       string `json:"id"`
					Duration int64  `json:"duration"`
					Caption  string `json:"caption"`
					PhotoURL string `json:"photoUrl"`
					CoverURL string `json:"coverUrl"`
				} `json:"photo"`
			} `json:"visionVideoDetail"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("GraphQL 响应解析失败: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL 错误: %s", resp.Errors[0].Message)
	}
	p := resp.Data.VisionVideoDetail.Photo
	if p == nil || p.PhotoURL == "" {
		return nil, errors.New("GraphQL 未返回视频地址（可能需要有效Cookie或触发风控）")
	}

	return &VideoInfo{
		PageURL:  pageURL,
		ID:       orDefault(p.ID, id),
		Title:    strings.TrimSpace(p.Caption),
		CoverURL: p.CoverURL,
		VideoURL: p.PhotoURL,
		Duration: int(p.Duration),
		Source:   "api",
	}, nil
}

func orDefault(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
