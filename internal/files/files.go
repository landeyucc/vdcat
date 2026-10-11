package files

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileInfo 已下载文件信息
type FileInfo struct {
	Name       string    `json:"name"`
	RelPath    string    `json:"rel_path"`
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	IsVideo    bool      `json:"is_video"`
	IsImage    bool      `json:"is_image"`
	Ext        string    `json:"ext"`
}

var videoExt = map[string]bool{
	".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".m4v": true, ".ts": true,
}
var imageExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// List 递归列出下载目录中的媒体文件（忽略 .part 与 .json）
func List(root string) ([]*FileInfo, error) {
	var out []*FileInfo
	root = filepath.Clean(root)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".part" || ext == ".json" || ext == ".db" ||
			strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}
		if !videoExt[ext] && !imageExt[ext] {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, &FileInfo{
			Name:    info.Name(),
			RelPath: filepath.ToSlash(rel),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsVideo: videoExt[ext],
			IsImage: imageExt[ext],
			Ext:     ext,
		})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []*FileInfo{}, nil
		}
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// safeJoin 防止路径穿越
func safeJoin(root, rel string) (string, error) {
	clean := filepath.Clean(filepath.Join(root, rel))
	rootAbs, err1 := filepath.Abs(root)
	cleanAbs, err2 := filepath.Abs(clean)
	if err1 != nil || err2 != nil {
		return "", errors.New("路径非法")
	}
	if cleanAbs != rootAbs && !strings.HasPrefix(cleanAbs, rootAbs+string(os.PathSeparator)) {
		return "", errors.New("非法路径")
	}
	return cleanAbs, nil
}

// Delete 删除指定相对路径的文件
func Delete(root, rel string) error {
	p, err := safeJoin(root, rel)
	if err != nil {
		return err
	}
	if info, err := os.Stat(p); err != nil || info.IsDir() {
		return errors.New("文件不存在")
	}
	return os.Remove(p)
}

// Resolve 解析为绝对路径（供下载/播放）
func Resolve(root, rel string) (string, error) {
	return safeJoin(root, rel)
}
