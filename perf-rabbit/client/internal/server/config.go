package server

import (
	"os"
	"strings"
)

func DevPort() string {
	// 开发调试默认用 8081，避免和打包后的一体化程序 8080 冲突。
	return portFromEnv("PERF_RABBIT_DEV_PORT", "9527")
}

func AppPort() string {
	// 打包程序默认用 8080，用户双击后访问固定地址。
	return portFromEnv("PERF_RABBIT_PORT", "8080")
}

func portFromEnv(key string, defaultPort string) string {
	port := strings.TrimSpace(os.Getenv(key))
	if port == "" {
		return defaultPort
	}

	return port
}
