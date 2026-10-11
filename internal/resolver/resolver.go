package resolver

import (
	"fmt"
	"strings"

	"vdcat/internal/config"
	"vdcat/internal/douyin"
	"vdcat/internal/kuaishou"
	"vdcat/internal/platform"
	"vdcat/internal/store"
)

// ResolveResult 解析结果（含使用的 cookie 序号，便于前端展示）
type ResolveResult struct {
	Info          *platform.Info    `json:"info"`
	Platform      platform.Platform `json:"platform"`
	CookieUsed    int               `json:"cookie_used"` // 0=未用cookie, 1..=第几个
	CookieCount   int               `json:"cookie_count"`
	CookieContent string            `json:"cookie_content"` // 实际成功使用的 cookie，透传给下载
}

// Resolver 平台识别 + 多 Cookie 轮转
type Resolver struct {
	store   *store.Store
	parsers map[platform.Platform]platform.Parser
}

func New(st *store.Store) *Resolver {
	return &Resolver{
		store: st,
		parsers: map[platform.Platform]platform.Parser{
			platform.Kuaishou: kuaishou.NewAdapter(),
			platform.Douyin:   douyin.NewAdapter(),
		},
	}
}

// Resolve 解析。manual 为 auto 时自动识别平台。
// 同平台多个已启用 cookie：默认用首位，失败（任意解析错误）自动尝试下一个。
func (r *Resolver) Resolve(input string, manual platform.Platform, mode string, force bool) (*ResolveResult, error) {
	mode = platform.NormalizeMode(mode)
	pf := manual
	if pf == "" || pf == platform.Auto {
		pf = platform.Detect(input)
	}
	if pf == "" || pf == platform.Auto {
		return nil, fmt.Errorf("无法识别链接所属平台，请在下方手动选择平台")
	}
	parser, ok := r.parsers[pf]
	if !ok {
		return nil, fmt.Errorf("暂不支持平台: %s", pf.Label())
	}

	// 收集该平台 cookie：DB 中已启用的多条 + 兼容旧 config 中的单条 cookie
	cookies := r.collectCookies(pf)

	var lastErr error
	if len(cookies) == 0 {
		// 无 cookie 也尝试一次（部分平台/部分视频无需登录）
		info, err := parser.Parse(input, "", mode, force)
		if err != nil {
			return nil, err
		}
		return &ResolveResult{Info: info, Platform: pf, CookieUsed: 0, CookieCount: 0}, nil
	}

	for i, ck := range cookies {
		info, err := parser.Parse(input, ck, mode, force)
		if err == nil && info != nil && info.VideoURL != "" {
			return &ResolveResult{
				Info:          info,
				Platform:      pf,
				CookieUsed:    i + 1,
				CookieCount:   len(cookies),
				CookieContent: ck,
			}, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("解析结果为空")
		}
		continue
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("该平台全部 Cookie 均解析失败")
	}
	return nil, fmt.Errorf("%s（已尝试 %d 个Cookie）: %w", pf.Label(), len(cookies), lastErr)
}

// collectCookies 返回优先级排列的 cookie：DB cookies 表 + 旧 config 单 cookie 兜底
func (r *Resolver) collectCookies(pf platform.Platform) []string {
	var out []string
	if r.store != nil {
		if list, err := r.store.EnabledCookieContents(string(pf)); err == nil && len(list) > 0 {
			out = append(out, list...)
		}
	}
	// 兼容：历史版本快手 cookie 存在 config 表；去重后置底
	if pf == platform.Kuaishou {
		if legacy := strings.TrimSpace(config.Get().Cookie); legacy != "" {
			dup := false
			for _, c := range out {
				if c == legacy {
					dup = true
					break
				}
			}
			if !dup {
				out = append(out, legacy)
			}
		}
	}
	return out
}
