package server

import (
	"net/http"

	"interface-load-test/client/common"
)

type agentInfo struct {
	Version string `json:"version"`
}

// AgentInfoHandler 上报 Agent 版本号，供中心平台前端探测 Agent 存活时顺带
// 判断版本是否兼容（web/src/api/perfAgent.ts 的 probePerfAgent）。
func AgentInfoHandler(w http.ResponseWriter, r *http.Request) {
	common.Success(w, agentInfo{Version: common.AgentVersion})
}
