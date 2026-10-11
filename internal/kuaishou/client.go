package kuaishou

import (
	"crypto/tls"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	baseURL    = "https://www.kuaishou.com"
	videoPage  = "https://www.kuaishou.com/short-video/%s"
	graphqlURL = "https://www.kuaishou.com/graphql"
	defaultUA  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
)

// Client 快手解析客户端
type Client struct {
	hc     *http.Client
	cookie string
	ua     string
}

// New 创建解析客户端。cookie 可为空。
func New(cookie string) *Client {
	jar, _ := cookiejar.New(nil)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &Client{
		hc:     &http.Client{Jar: jar, Transport: tr, Timeout: 30 * time.Second},
		cookie: cookie,
		ua:     defaultUA,
	}
}

func (c *Client) browserHeaders(referer string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", c.ua)
	h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	h.Set("Accept-Encoding", "gzip, deflate, br")
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("Sec-Ch-Ua", `"Chromium";v="130", "Not?A_Brand";v="99"`)
	h.Set("Sec-Ch-Ua-Mobile", "?0")
	h.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	h.Set("Sec-Fetch-Dest", "document")
	h.Set("Sec-Fetch-Mode", "navigate")
	h.Set("Sec-Fetch-Site", "none")
	h.Set("Upgrade-Insecure-Requests", "1")
	if referer != "" {
		h.Set("Referer", referer)
	}
	if c.cookie != "" {
		h.Set("Cookie", c.cookie)
	}
	return h
}
