package kuaishou

import (
	"vdcat/internal/platform"
)

// Adapter 把快手解析器适配到统一 platform.Parser 接口
type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (Adapter) Platform() platform.Platform { return platform.Kuaishou }

func (Adapter) Parse(rawURL, cookie string, mode string, force bool) (*platform.Info, error) {
	client := New(cookie) // 每次尝试独立 client，携带对应 cookie
	info, err := client.Parse(rawURL, mode, force)
	if err != nil {
		return nil, err
	}
	return &platform.Info{
		Platform: platform.Kuaishou,
		PageURL:  info.PageURL,
		ID:       info.ID,
		Title:    info.Title,
		Author:   info.Author,
		AuthorID: info.AuthorID,
		CoverURL: info.CoverURL,
		VideoURL: info.VideoURL,
		Width:    info.Width,
		Height:   info.Height,
		Duration: info.Duration,
		Source:   info.Source,
	}, nil
}
