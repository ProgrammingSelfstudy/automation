# 接口压测平台技术方案 v1.0

## 0. 设计目标

1. 支持多账号并发压测，账号与并发数严格绑定（N个账号 = 最大N并发）
2. 支持接口关联（上一步响应 -> 下一步入参）+ 自定义公式计算
3. 每次执行生成唯一 TaskID，同一 Task 下按账号分组存储数据，导出时不同账号数据不混在一张表/一个 sheet
4. 平台长期演进，预留模块化扩展位（下一个模块：性能测试 / 后续可能还有：接口Mock、数据对比等）

---

## 1. 整体架构

采用「核心平台 + 业务模块」的分层结构，压测（LoadTest）是第一个业务模块，性能测试（Performance，先占位）是第二个。

```
┌─────────────────────────────────────────────────────────────┐
│                      Web 前端 (Vue/React)                     │
│   任务创建 / 实时进度 / 结果查看 / 导出                          │
└───────────────────────────┬─────────────────────────────────┘
                             │ HTTP/WebSocket
┌───────────────────────────▼─────────────────────────────────┐
│                      API Gateway 层                          │
│   统一鉴权 / 路由 / 参数校验                                    │
└───────────────────────────┬─────────────────────────────────┘
                             │
┌───────────────────────────▼─────────────────────────────────┐
│                      核心平台层 (Core)                         │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌───────────┐ │
│  │  任务调度器  │ │  账号池管理  │ │  结果存储   │ │  导出中心  │ │
│  │ TaskManager│ │ AccountPool│ │ ResultStore│ │ ExportSvc │ │
│  └────────────┘ └────────────┘ └────────────┘ └───────────┘ │
│              ModuleRegistry (模块注册中心 - 插件机制)             │
└──────┬──────────────────────────────────────────┬────────────┘
       │                                           │
┌──────▼──────────────┐                 ┌──────────▼───────────┐
│  模块一：压测 LoadTest │                 │ 模块二：性能测试(占位)   │
│  - 接口编排Scenario   │                 │  Performance Module   │
│  - 变量提取/公式计算   │                 │  TODO: 后续设计        │
│  - Worker Pool 执行   │                 │                       │
└──────────────────────┘                 └───────────────────────┘
```

**核心思路**：TaskManager / AccountPool / ResultStore / ExportSvc 是所有模块共用的基础设施，不属于压测模块私有。新增模块时只需实现统一的 `Module` 接口（见第6节），注册进 `ModuleRegistry`，前端加一个入口即可，不改动核心平台代码。

---

## 2. 核心数据模型

### 2.1 Task（执行任务）— 平台级通用概念

```go
type Task struct {
    ID          string    // 任务ID，UUID，全局唯一，贯穿整个执行生命周期
    ModuleType  string    // "load_test" / "performance"，标识属于哪个模块
    Name        string
    Status      string    // pending / running / success / failed / canceled
    Config      json.RawMessage // 模块自定义配置（如 Scenario 定义、并发数等），不同模块结构不同
    Concurrency int
    TotalCount  int       // 计划执行总条数
    CreatedBy   string
    CreatedAt   time.Time
    StartedAt   *time.Time
    FinishedAt  *time.Time
}
```

### 2.2 Account（账号）— 平台级通用概念

```go
type Account struct {
    ID       string
    GroupID  string // 账号分组，不同任务可以用不同账号组
    Username string
    Password string // 建议加密存储
    Extra    json.RawMessage // 扩展字段，比如附加的登录参数
    Enabled  bool
}
```

### 2.3 TaskResult（单条执行结果）— 关键：按账号隔离

**这是解决"不同账号数据不能混表"的核心字段设计**：每条结果记录必须打上 `TaskID + AccountID`，导出时按 `AccountID` 分组，一个账号对应一个 sheet（或一张表）。

```go
type TaskResult struct {
    ID          int64
    TaskID      string  // 关联 Task
    AccountID   string  // 关联 Account —— 分表/分sheet的依据
    AccountName string  // 冗余存一份，导出时直接用，不用再 join
    SeqNo       int     // 该账号下第几条（1,2,3...），保证顺序
    Steps       json.RawMessage // 每一步接口的请求/响应详情（调试用）
    FormulaResult float64  // 公式计算结果
    Success     bool
    ErrMsg      string
    CostMs      int64   // 本条整体耗时
    CreatedAt   time.Time
}
```

---

## 3. 数据库表设计（MySQL，供参考）

```sql
-- 任务表
CREATE TABLE task (
    id            VARCHAR(36) PRIMARY KEY,
    module_type   VARCHAR(32)  NOT NULL,   -- load_test / performance
    name          VARCHAR(128) NOT NULL,
    status        VARCHAR(16)  NOT NULL DEFAULT 'pending',
    config        JSON         NOT NULL,
    concurrency   INT          NOT NULL,
    total_count   INT          NOT NULL,
    success_count INT          NOT NULL DEFAULT 0,
    fail_count    INT          NOT NULL DEFAULT 0,
    created_by    VARCHAR(64),
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at    DATETIME     NULL,
    finished_at   DATETIME     NULL,
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
);

-- 账号表
CREATE TABLE account (
    id          VARCHAR(36) PRIMARY KEY,
    group_id    VARCHAR(36)  NOT NULL,
    username    VARCHAR(128) NOT NULL,
    password    VARCHAR(256) NOT NULL,   -- 加密存储
    extra       JSON,
    enabled     TINYINT      NOT NULL DEFAULT 1,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_group (group_id)
);

-- 场景/编排配置表（压测模块私有，随 Task.config 冗余一份快照，避免场景改动影响历史任务）
CREATE TABLE scenario (
    id          VARCHAR(36) PRIMARY KEY,
    name        VARCHAR(128) NOT NULL,
    steps       JSON         NOT NULL,  -- Step 数组：接口地址/提取规则/公式
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 执行结果表 —— 按 task_id + account_id 组合查询/导出
CREATE TABLE task_result (
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id        VARCHAR(36) NOT NULL,
    account_id     VARCHAR(36) NOT NULL,
    account_name   VARCHAR(128) NOT NULL,
    seq_no         INT NOT NULL,
    steps          JSON,
    formula_result DOUBLE,
    success        TINYINT NOT NULL,
    err_msg        VARCHAR(512),
    cost_ms        BIGINT,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_account (task_id, account_id, seq_no)  -- 导出时按此索引分组查询，性能关键
);
```

> 表结果量大时建议按 `task_id` 做归档/分区，避免单表无限增长；如果预期单任务数据量特别大（十万级以上），可以考虑 `task_result` 按月分表。

---

## 4. 账号池并发控制

沿用之前讨论的设计：账号数量即最大并发上限，用带缓冲 channel 实现"借出-归还"语义。

```go
type AccountPool struct {
    idle chan *Account
}

func NewAccountPool(accounts []*Account) *AccountPool {
    idle := make(chan *Account, len(accounts))
    for _, a := range accounts {
        idle <- a
    }
    return &AccountPool{idle: idle}
}

func (p *AccountPool) Acquire(ctx context.Context) (*Account, error) {
    select {
    case acc := <-p.idle:
        return acc, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (p *AccountPool) Release(acc *Account) {
    p.idle <- acc
}
```

若前端配置并发数 > 账号数，平台层面直接报错拦截或自动降级为账号数，需要在需求交互上明确提示用户，避免误解。

---

## 5. 压测模块执行流程

```
创建 Task (TaskID生成)
   │
   ▼
按账号拆分执行队列：TotalCount 条数据如何分配给 N 个账号？
   - 方式A：轮询分配（round-robin），保证每个账号执行条数均衡
   - 方式B：每个账号独立跑满 TotalCount/N 条
   │
   ▼
Worker Pool 启动（worker数 = min(concurrency, 账号数)）
   │
   ├─ 每个 worker: Acquire账号 -> 执行 Scenario 全部 Step -> 记录 SeqNo(该账号第几条)
   │                -> 写入 TaskResult(task_id, account_id, seq_no, ...) -> Release账号
   │
   ▼
Task.status 实时更新（running中定期汇总 success_count/fail_count，供前端轮询/WS推送进度）
   │
   ▼
全部完成 -> status=success -> 前端可点击"导出"
```

Scenario 执行核心（沿用之前讨论）：

```go
type Step struct {
    Name    string
    Method  string
    URL     string
    BodyTpl string
    Extract map[string]string // key -> gjson path，从响应提取变量
    Headers map[string]string
}

type Scenario struct {
    Steps   []Step
    Formula string // 用 expr-lang/expr 求值，如 "(price - discount) * qty"
}

func ExecuteScenario(ctx context.Context, acc *Account, sc Scenario) TaskResult {
    vars := map[string]interface{}{"account": acc}
    var stepLogs []StepLog

    for _, step := range sc.Steps {
        body := renderTemplate(step.BodyTpl, vars)
        resp, err := httpDo(step.Method, step.URL, body, step.Headers)
        if err != nil {
            return TaskResult{Success: false, ErrMsg: err.Error()}
        }
        for k, path := range step.Extract {
            vars[k] = gjson.GetBytes(resp, path).Value()
        }
        stepLogs = append(stepLogs, StepLog{Name: step.Name, Resp: string(resp)})
    }

    result, err := evalFormula(sc.Formula, vars) // expr-lang/expr
    return TaskResult{FormulaResult: result, Success: err == nil, Steps: marshalLogs(stepLogs)}
}
```

---

## 6. 模块化扩展设计（为性能测试等后续模块预留）

定义统一的 `Module` 接口，`ModuleRegistry` 在平台启动时注册所有模块：

```go
// Module 是所有业务模块（压测、性能测试...）必须实现的接口
type Module interface {
    Type() string                                    // "load_test" / "performance"
    ValidateConfig(cfg json.RawMessage) error         // 校验前端传入的模块配置
    Run(ctx context.Context, task *Task) error         // 执行任务，内部写 TaskResult
    ResultColumns() []ExportColumn                    // 定义导出Excel时该模块的列结构
}

type ModuleRegistry struct {
    modules map[string]Module
}

func (r *ModuleRegistry) Register(m Module) {
    r.modules[m.Type()] = m
}

func (r *ModuleRegistry) Get(moduleType string) (Module, bool) {
    m, ok := r.modules[moduleType]
    return m, ok
}
```

- **压测模块**：`loadtest.Module{}`，实现上面第5节的逻辑
- **性能测试模块（占位）**：`performance.Module{}` —— 具体设计后续再补，但接口位置现在就定好，比如可能关注的是"单接口在压力下的 RT/TPS/错误率曲线"而不是"业务公式计算"，`ResultColumns()` 会输出不同的列（如 P99、P95、TPS 而不是 FormulaResult）
- TaskManager、AccountPool、ExportSvc 对 Module 无感知，只认 Task.ModuleType 路由到对应 Module 执行

新增模块的成本 = 实现一个 Module 接口 + 前端加一个配置表单，核心链路不用动。

---

## 7. 导出设计（按账号分 Sheet，核心诉求）

```go
func ExportTask(taskID string) (*excelize.File, error) {
    f := excelize.NewFile()

    // 按 account_id 分组查询，SQL: SELECT * FROM task_result WHERE task_id=? ORDER BY account_id, seq_no
    grouped := queryResultsGroupByAccount(taskID)

    first := true
    for accountID, results := range grouped {
        sheetName := results[0].AccountName // 每个账号一个sheet，sheet名=账号名
        if first {
            f.SetSheetName("Sheet1", sheetName)
            first = false
        } else {
            f.NewSheet(sheetName)
        }

        headers := []string{"序号", "耗时(ms)", "公式结果", "是否成功", "错误信息", "执行时间"}
        for i, h := range headers {
            cell, _ := excelize.CoordinatesToCellName(i+1, 1)
            f.SetCellValue(sheetName, cell, h)
        }
        for rowIdx, r := range results {
            row := rowIdx + 2
            f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), r.SeqNo)
            f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), r.CostMs)
            f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), r.FormulaResult)
            f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), r.Success)
            f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), r.ErrMsg)
            f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), r.CreatedAt.Format(time.RFC3339))
        }
    }
    return f, nil
}
```

- 前端导出按钮 -> 调 `GET /api/tasks/{taskID}/export` -> 后端生成临时文件 -> 返回下载链接（或直接流式返回）
- 大数据量场景（单任务几万条以上）建议异步生成 + 消息通知/轮询下载，不要同步阻塞 HTTP 请求

---

## 8. API 设计（核心接口一览）

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/tasks | 创建任务（含 module_type, scenario配置, 账号组, 并发数, 总条数） |
| GET | /api/tasks/{id} | 查询任务状态/进度 |
| GET | /api/tasks/{id}/results?account_id=xxx | 分页查看某账号下的明细结果 |
| GET | /api/tasks/{id}/export | 导出Excel（按账号分sheet） |
| POST | /api/tasks/{id}/cancel | 取消运行中任务 |
| GET | /api/accounts?group_id=xxx | 查询账号组 |
| POST | /api/accounts | 新增账号 |
| GET | /api/scenarios | 场景列表 |
| POST | /api/scenarios | 创建/编辑场景（接口编排+公式配置） |
| WS | /ws/tasks/{id}/progress | 任务执行实时进度推送 |

---

## 9. 待你确认/后续要拍板的点

1. **总条数分配策略**：TotalCount 是按账号轮询平均分配，还是每个账号各跑满 TotalCount 条（那么总条数=TotalCount×账号数）？这会影响 Worker 调度逻辑，需要先定。
2. **账号复用登录态**：账号的登录 Token 是否需要在 Task 开始前统一登录一次并缓存，还是每次执行都重新登录（取决于业务接口是否要求）？
3. **实时进度推送**：轮询还是 WebSocket？如果任务量不大（几百到几千条），轮询足够简单；量大且要求体验好，建议 WebSocket。
4. **性能测试模块的具体指标**：目前只占了位置，后续需要单独设计（TPS/RT分布/错误率曲线等），这块可以下一轮细化。

方案先到这，你看看第9点里哪些需要先定下来，我们可以针对某一块（比如账号池、Scenario编排、导出）先出可运行的代码。
