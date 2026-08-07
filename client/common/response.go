package common

import (
	"encoding/json"
	"net/http"
)

// ==================== 统一返回结构 ====================

type Resp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

// Success 统一成功响应封装函数，后端所有接口正常返回必须使用此方法
// 【必须理由】全项目统一JSON返回格式，前端只需要一套解析逻辑，不用适配多种返回结构
func Success(w http.ResponseWriter, data interface{}) {
	writeResp(w, Resp{Code: 0, Msg: "success", Data: data})
}

// Fail 统一失败响应封装函数，所有业务异常、参数错误、权限失败必须调用该方法返回
// 【必须理由】统一错误输出格式，前端统一拦截错误、统一弹窗/提示，方便日志排查与错误码管理
func Fail(w http.ResponseWriter, code int, msg string) {
	writeResp(w, Resp{Code: code, Msg: msg, Data: nil})
}

// writeResp 写 JSON 响应。HTTP 状态码固定 200——区分 HTTP 网络错误和业务逻辑
// 错误是这个项目的既有约定，前端不用区分 4xx/5xx，统一解析 Resp.Code 就行。
func writeResp(w http.ResponseWriter, resp Resp) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
