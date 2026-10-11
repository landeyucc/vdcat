package kuaishou

// VideoInfo 解析得到的视频元数据
type VideoInfo struct {
	PageURL  string `json:"page_url"` // 规范化后的页面链接
	ID       string `json:"id"`
	Title    string `json:"title"` // 标题/文案
	Author   string `json:"author"`
	AuthorID string `json:"author_id"`
	CoverURL string `json:"cover_url"`
	VideoURL string `json:"video_url"` // 无水印 mp4 直链
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Duration int    `json:"duration"`
	Source   string `json:"source"` // api | html
}
