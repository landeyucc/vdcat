package douyin

import (
	"compress/flate"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"time"

	"github.com/andybalholm/brotli"

	"vdcat/internal/platform"
)

const (
	uaMobile = "Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.6 Mobile/15E148 Safari/604.1"

	// App 端 feed 接口：免登录、免 a_bogus 签名即可拉取公开视频（含无水印 play_addr）。
	// web 端 aweme/detail 接口受 Argus 设备指纹风控（403 Uifid Not Found），故改用此通道。
	feedURL = "https://api.amemv.com/aweme/v1/feed/?aweme_id=%s&aid=1128&version_name=23.5.0&device_platform=ios"
)

// Client 抖音解析客户端
type Client struct {
	hc     *http.Client
	cookie string
}

// Adapter 统一平台接口
type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (Adapter) Platform() platform.Platform { return platform.Douyin }

func (Adapter) Parse(rawURL, cookie string, mode string, force bool) (*platform.Info, error) {
	// 抖音当前仅提供 App feed 接口通道（web detail 受 Argus 风控，HTML 不下发播放地址）。
	// 若用户强制 HTML 模式，直接明确报错而不是悄悄走接口，避免违背“强制此模式”的预期。
	if force && platform.NormalizeMode(mode) == platform.ModeHTML {
		return nil, errors.New("抖音暂不支持 HTML 提取模式，请选择接口解析或自动模式")
	}
	c := newClient(cookie)
	return c.Parse(rawURL)
}

func newClient(cookie string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		cookie: cookie,
		hc: &http.Client{
			Jar: jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil
			},
			Timeout: 30 * time.Second,
		},
	}
}

var idInPageReg = regexp.MustCompile(`(?:video|note|share/video)/(\d+)`)

// Parse 解析抖音视频
func (c *Client) Parse(raw string) (*platform.Info, error) {
	input := platform.ExtractURL(raw)
	if input == "" {
		return nil, errors.New("无法识别抖音链接")
	}

	id, canonical, err := c.resolveID(input)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, errors.New("未找到视频ID")
	}

	page := canonical
	if page == "" {
		page = "https://www.douyin.com/video/" + id
	}

	info, err := c.fetchDetail(id)
	if err != nil {
		return nil, err
	}
	info.PageURL = page
	info.ID = id
	info.Platform = platform.Douyin
	return info, nil
}

// resolveID 从直链/短链/纯ID 获取 aweme id 与规范化页面URL
func (c *Client) resolveID(u string) (string, string, error) {
	// 纯数字ID（>=15 位 aweme id）
	if regexp.MustCompile(`^\d{15,}$`).MatchString(strings.TrimSpace(u)) {
		return strings.TrimSpace(u), "https://www.douyin.com/video/" + strings.TrimSpace(u), nil
	}

	if m := idInPageReg.FindStringSubmatch(u); len(m) == 2 {
		return m[1], u, nil
	}

	// 短链：跟随重定向，从最终 Location 取 id
	if strings.Contains(u, "v.douyin.com") || strings.Contains(u, "iesdouyin.com/share") {
		final, err := c.followRedirect(u)
		if err != nil {
			return "", "", err
		}
		if m := idInPageReg.FindStringSubmatch(final); len(m) == 2 {
			return m[1], final, nil
		}
		return "", "", errors.New("短链重定向后未找到视频ID")
	}
	return "", "", errors.New("无法从链接中识别抖音视频ID")
}

func (c *Client) followRedirect(u string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.setCommonHeaders(req, "https://www.douyin.com/", uaMobile)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String(), nil
	}
	return u, nil
}

// fetchDetail 通过 App 端 feed 接口拉取视频（免登录）。
// feed 会返回多条相关内容，必须按 aweme_id 精确匹配目标视频。
func (c *Client) fetchDetail(id string) (*platform.Info, error) {
	api := fmt.Sprintf(feedURL, id)
	req, err := http.NewRequest(http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", uaMobile)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	req.Header.Set("Referer", "https://www.douyin.com/")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := c.readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("抖音接口拒绝访问(%d)，可能触发风控，稍后重试或配置Cookie", resp.StatusCode)
	}

	var data struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
		AwemeList  []struct {
			AwemeID  string `json:"aweme_id"`
			Desc     string `json:"desc"`
			Duration int    `json:"duration"` // 毫秒
			Video    struct {
				PlayAddr struct {
					Width   int      `json:"width"`
					Height  int      `json:"height"`
					URLList []string `json:"url_list"`
				} `json:"play_addr"`
				Cover struct {
					URLList []string `json:"url_list"`
				} `json:"cover"`
				DynamicCover struct {
					URLList []string `json:"url_list"`
				} `json:"dynamic_cover"`
			} `json:"video"`
			Author struct {
				UID      string `json:"uid"`
				Nickname string `json:"nickname"`
			} `json:"author"`
		} `json:"aweme_list"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("详情响应解析失败: %w", err)
	}
	if data.StatusCode != 0 && len(data.AwemeList) == 0 {
		return nil, fmt.Errorf("接口未返回视频详情(status_code=%d)", data.StatusCode)
	}

	// 精确匹配目标 aweme_id；找不到则退回第一条
	var d = &data.AwemeList[0]
	for i := range data.AwemeList {
		if data.AwemeList[i].AwemeID == id {
			d = &data.AwemeList[i]
			break
		}
	}

	videoURL := firstURL(d.Video.PlayAddr.URLList)
	if videoURL == "" {
		return nil, errors.New("未获取到视频播放地址（可能为私密或已删除视频）")
	}
	// 兜底：个别链接为带水印 playwm，替换为 play 无水印地址
	videoURL = strings.Replace(videoURL, "playwm", "play", 1)

	cover := firstURL(d.Video.Cover.URLList)
	if cover == "" {
		cover = firstURL(d.Video.DynamicCover.URLList)
	}

	return &platform.Info{
		ID:       d.AwemeID,
		Title:    strings.TrimSpace(d.Desc),
		Author:   d.Author.Nickname,
		AuthorID: d.Author.UID,
		CoverURL: cover,
		VideoURL: videoURL,
		Width:    d.Video.PlayAddr.Width,
		Height:   d.Video.PlayAddr.Height,
		Duration: d.Duration,
		Source:   "api",
	}, nil
}

func firstURL(list []string) string {
	for _, u := range list {
		if strings.HasPrefix(u, "http") {
			return u
		}
	}
	if len(list) > 0 {
		return list[0]
	}
	return ""
}

func (c *Client) setCommonHeaders(req *http.Request, referer, ua string) {
	h := req.Header
	h.Set("User-Agent", ua)
	h.Set("Accept", "text/html,application/json,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9")
	h.Set("Accept-Encoding", "gzip, deflate, br")
	h.Set("Cache-Control", "no-cache")
	h.Set("Referer", referer)
	if c.cookie != "" {
		h.Set("Cookie", c.cookie)
	}
}

func (c *Client) readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "deflate":
		reader = flate.NewReader(resp.Body)
	}
	return io.ReadAll(reader)
}
