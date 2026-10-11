package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv 从指定路径加载 .env 文件到进程环境变量。
// 规则：
//   - 文件不存在时静默返回（可选配置）；
//   - 已存在的环境变量优先，.env 不覆盖（便于用系统环境变量临时覆盖）；
//   - 支持 KEY=VALUE、去除首尾空白与引号、忽略空行和 # 注释；
//   - 支持 export KEY=VALUE 写法。
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		line = strings.TrimSpace(line)
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}

// LoadDefaultDotEnv 依次尝试工作目录与可执行文件目录下的 .env。
func LoadDefaultDotEnv() {
	_ = LoadDotEnv(".env")
	if exe, err := os.Executable(); err == nil {
		_ = LoadDotEnv(filepath.Join(filepath.Dir(exe), ".env"))
	}
}
