package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// perfAgentDownloads 是本地采集 Agent 支持下载的平台列表。Agent 编译自
// perf-rabbit/client/cmd/main.go（开发调试入口：只跑后端接口、不内嵌前端、
// 不自动开浏览器），不是 cmd/app——下载下来的是给中心平台当"本地采集桥"用
// 的无头程序，不是 perf-rabbit 自己那套风格不一致的独立 UI。
//
// 文件名固定写死在这个白名单里，下载接口只按文件名精确匹配，不会把
// r.PathValue 直接拼进文件系统路径，避免路径穿越。
var perfAgentDownloads = []struct {
	Platform string
	Label    string
	Filename string
}{
	{Platform: "darwin-arm64", Label: "macOS（Apple 芯片）", Filename: "perf-agent-darwin-arm64"},
	{Platform: "darwin-amd64", Label: "macOS（Intel 芯片）", Filename: "perf-agent-darwin-amd64"},
	{Platform: "windows-amd64", Label: "Windows（64 位）", Filename: "perf-agent-windows-amd64.exe"},
}

type perfAgentDownloadOption struct {
	Platform  string `json:"platform"`
	Label     string `json:"label"`
	Filename  string `json:"filename"`
	Available bool   `json:"available"`
}

// listPerfAgentDownloads 返回每个平台的下载选项，以及这次部署实际有没有
// 放对应的二进制文件——前端靠 available 灰掉还没上传的平台，而不是让用户
// 点了之后才看到一个 404。
func (h *handler) listPerfAgentDownloads(w http.ResponseWriter, r *http.Request) {
	options := make([]perfAgentDownloadOption, 0, len(perfAgentDownloads))
	for _, item := range perfAgentDownloads {
		_, err := os.Stat(filepath.Join(h.deps.PerfAgentAssetsDir, item.Filename))
		options = append(options, perfAgentDownloadOption{
			Platform:  item.Platform,
			Label:     item.Label,
			Filename:  item.Filename,
			Available: err == nil,
		})
	}
	writeJSON(w, http.StatusOK, options)
}

func (h *handler) downloadPerfAgent(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if !isKnownPerfAgentFilename(filename) {
		http.NotFound(w, r)
		return
	}

	path := filepath.Join(h.deps.PerfAgentAssetsDir, filename)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, path)
}

func isKnownPerfAgentFilename(filename string) bool {
	for _, item := range perfAgentDownloads {
		if item.Filename == filename {
			return true
		}
	}
	return false
}
