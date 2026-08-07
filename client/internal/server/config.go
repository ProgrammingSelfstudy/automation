package server

import (
	"os"
	"strings"
)

func DevPort() string {
	return portFromEnv("PERF_RABBIT_DEV_PORT", "9527")
}

func portFromEnv(key string, defaultPort string) string {
	port := strings.TrimSpace(os.Getenv(key))
	if port == "" {
		return defaultPort
	}

	return port
}
