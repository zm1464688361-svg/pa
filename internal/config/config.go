// Package config 读取本地配置：环境变量优先，其次 config.local.yaml（不入库）。
package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDSN 返回数据库 DSN：先读环境变量 DB_DSN，再读 config.local.yaml 的 db_dsn 行。
func LoadDSN() string {
	if v := os.Getenv("DB_DSN"); v != "" {
		return v
	}
	return LoadEnvKey("db_dsn")
}

// LoadEnvKey 从 config.local.yaml 读取单行 key: value（兼容从子目录运行，向上查找两级）。
func LoadEnvKey(key string) string {
	for _, path := range []string{"config.local.yaml", "../config.local.yaml", "../../config.local.yaml"} {
		if v := readKey(path, key); v != "" {
			return v
		}
	}
	return ""
}

func readKey(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	first := true
	prefix := key + ":"
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			line = strings.TrimPrefix(line, "\ufeff") // 去掉 UTF-8 BOM
			first = false
		}
		if strings.HasPrefix(line, prefix) {
			return unquote(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		}
	}
	return ""
}

// unquote 去掉包裹的引号（yaml 风格）。
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
