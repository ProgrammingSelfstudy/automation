# 项目说明：接口压测平台

## 技术方案
完整设计见 docs/design.md，关键决策：
- 账号池 = 并发上限，用 buffered channel 实现（不用 sync.Cond）
- TaskResult 表按 task_id + account_id 分组，导出时一个账号一个 Excel sheet
- 模块化设计：Module 接口 + ModuleRegistry，压测是第一个模块，性能测试模块后续再加
- 日志推送用 WebSocket（TaskLogHub），非阻塞广播，只推送 Step 完成/失败事件，
  不要把完整 request/response body 塞进 WebSocket 消息

## 当前开发顺序
1. [ ] internal/accountpool - 账号池（当前在做）
2. [ ] internal/logevent - WebSocket 日志推送
3. [ ] internal/scenario - 接口编排 + 公式计算
4. [ ] Worker Pool 压测执行引擎
5. [ ] 导出模块

## 代码规范
- 并发代码必须跑 go test -race
- 每个模块先写好单元测试再联调