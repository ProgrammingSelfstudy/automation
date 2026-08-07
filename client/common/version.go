package common

// AgentVersion 是本地采集 Agent 的版本号，随每次对 Agent<->中心平台通信
// 协议有实质性改动的发布递增。中心平台前端（web/src/api/perfAgent.ts 的
// MIN_COMPATIBLE_AGENT_VERSION）拿它跟自己要求的最低版本比较，判断要不要
// 提示用户升级本地 Agent——两边改协议时要同时改这两个常量。
const AgentVersion = "1.0.0"
