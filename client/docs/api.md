# Perf Rabbit 后端接口文档

## 基础信息

- 服务地址：`http://127.0.0.1:8080`
- 数据格式：`application/json`
- Android 设备依赖本机 `adb`，目标电脑需要能正常执行 `adb devices -l`。
- iOS 设备依赖 `python3 -m pymobiledevice3 usbmux list`。

## 统一返回结构

所有业务接口统一返回：

```json
{
  "code": 0,
  "msg": "success",
  "data": {}
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `code` | number | 业务状态码，`0` 表示成功 |
| `msg` | string | 业务提示 |
| `data` | object / array / null | 业务数据，失败时通常为 `null` |

常见错误码：

| code | 说明 |
| --- | --- |
| `10001` | 未检测到 ADB 环境 |
| `10002` | ADB 执行失败 |
| `10003` | 未检测到设备，或获取设备应用列表失败 |
| `10004` | 参数错误 |
| `10006` | 指定设备不在线 |
| `10007` | 启动性能采集失败 |
| `10008` | 停止性能采集失败 |
| `10009` | 查询性能采集任务失败 |
| `10010` | 保存历史采集失败 |
| `10011` | 查询历史采集失败 |
| `10012` | 删除历史采集失败 |

## 设备接口

### 获取设备列表

```http
GET /api/device/list
```

说明：一个接口同时查询当前 Android 和 iOS 设备，并返回设备基础信息。

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "total": 2,
    "android_total": 1,
    "ios_total": 1,
    "devices": [
      {
        "serial": "10AF5J1JP1004SU",
        "device_id": "10AF5J1JP1004SU",
        "platform": "android",
        "version": "15",
        "brand": "vivo",
        "model": "V2405A",
        "status": "Online"
      },
      {
        "serial": "G0NCGGV4N73V",
        "device_id": "00008030-001945681E91802E",
        "device_name": "iPhone",
        "platform": "ios",
        "version": "26.2.1",
        "brand": "Apple",
        "model": "iPhone 11",
        "connection_type": "network",
        "status": "Online"
      }
    ]
  }
}
```

设备字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `serial` | string | Android 是 ADB 序列号；iOS 优先返回硬件序列号，没有时返回 UDID |
| `device_id` | string | 设备唯一标识，Android 为 serial，iOS 为 UDID |
| `device_name` | string | iOS 设备名；Android 通常为空 |
| `platform` | string | `android` 或 `ios` |
| `version` | string | 系统版本 |
| `brand` | string | 设备品牌 |
| `model` | string | 设备型号 |
| `product_type` | string | iOS 机型标识，例如 `iPhone12,1` |
| `connection_type` | string | iOS 连接方式，例如 `usb`、`network` |
| `status` | string | 设备状态：`Online`、`Offline`、`Unauthorized` |

### 获取设备应用列表

```http
GET /api/devices/:deviceId/apps
```

路径参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `deviceId` | string | 是 | ADB 设备序列号 |

说明：

- Android：查询指定设备当前用户下安装的第三方应用包名，保持原有 `pm list packages -3 --user 当前用户` 逻辑。
- iOS：使用 `python3 -m pymobiledevice3 apps list --udid <deviceId>` 查询应用，返回中文应用名和 Bundle ID。

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "total": 2,
    "items": [
      {
        "app_name": "虎牙直播",
        "package_name": "com.duowan.kiwi"
      },
      {
        "app_name": "微信",
        "package_name": "com.tencent.mm"
      }
    ]
  }
}
```

应用字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `app_name` | string | 应用展示名；iOS 可返回中文名，Android 当前通常为空 |
| `package_name` | string | Android 包名 / iOS Bundle ID，开始采集时传这个字段 |
| `executable` | string | iOS 可执行文件名，排查进程名时使用 |

## 性能采集接口

### 开始性能采集

```http
POST /api/collect/perf/start
```

请求体：

```json
{
  "device_id": "10AF5J1JP1004SU",
  "package_name": "com.duowan.kiwi",
  "process_name": "kiwi",
  "device_model": "iPhone 11"
}
```

请求字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `device_id` | string | 是 | Android 序列号 / iOS UDID |
| `package_name` | string | 是 | Android 包名 / iOS Bundle ID |
| `process_name` | string | 否 | iOS 进程名，例如 `VoiceRoom`；Android 可不传，默认等于 `package_name` |
| `device_model` | string | 否 | iOS 机型，例如 `iPhone12,1` 或 `iPhone 11`；用于 CPU 核心数换算 |

说明：

- 同一个设备、同一个包名同时只允许一个采集任务。
- 成功后会返回 `task_id`。
- 后端默认每秒采集一次。
- Android 采集逻辑不变：CPU、内存、FPS、Jank。
- iOS 当前采集 CPU、内存、FPS、GPU：
  - CPU/内存使用 `pymobiledevice3 developer dvt sysmon process monitor process`，采样间隔为 `1000ms`。
  - FPS/GPU 使用 `pymobiledevice3 developer dvt graphics --userspace --udid <device_id>`。
- 如果设备采集中途掉线，任务会自动变成 `interrupted`。

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "task_id": "task_20260708_113643_abcd1234",
    "device_id": "10AF5J1JP1004SU",
    "package_name": "com.duowan.kiwi",
    "process_name": "kiwi",
    "platform": "android",
    "device_model": "",
    "status": "collecting",
    "start_time": "2026-07-08 11:36:43",
    "sample_interval_ms": 1000,
    "initial_sample": {
      "collect_time": "2026-07-08 11:36:43",
      "data": {}
    }
  }
}
```

任务状态：

| 状态 | 说明 |
| --- | --- |
| `collecting` | 采集中 |
| `stopped` | 用户手动停止 |
| `interrupted` | 设备断开等异常导致自动中断 |

### 查询性能采集数据

```http
GET /api/collect/perf/:taskId
```

支持增量查询：

```http
GET /api/collect/perf/:taskId?from=10
```

路径参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `taskId` | string | 是 | 开始采集接口返回的任务 ID |

Query 参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `from` | number | 否 | 样本起始下标；前端轮询时传上一次返回的 `next_from` |

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "task_id": "task_20260708_113643_abcd1234",
    "device_id": "10AF5J1JP1004SU",
    "package_name": "com.duowan.kiwi",
    "status": "collecting",
    "start_time": "2026-07-08 11:36:43",
    "sample_interval_ms": 1000,
    "last_error": "",
    "total_samples": 12,
    "next_from": 12,
    "samples": [
      {
        "collect_time": "2026-07-08 11:36:44",
        "data": {
          "cpu": {
            "app_cpu": 17.5,
            "total_cpu": 38.9
          },
          "memory": {
            "java_heap": 120,
            "native_heap": 220,
            "stack": 8,
            "graphics": 450,
            "total_pss": 1420
          },
          "fps": {
            "fps": 59.8,
            "frames": 60,
            "method": "surfaceflinger",
            "layer": "SurfaceView[com.xxx](BLAST)",
            "refresh_rate": 60,
            "has_frame_data": true,
            "time_source": "actual_present_time"
          },
          "jank": {
            "small_jank": 0,
            "jank": 0,
            "big_jank": 0,
            "total_small_jank": 0,
            "total_jank": 0,
            "total_big_jank": 0,
            "frames": 60,
            "max_frame_time_ms": 16.67,
            "has_frame_data": true,
            "method": "gfxinfo",
            "time_source": "display_present_time"
          }
        }
      }
    ]
  }
}
```

响应字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `task_id` | string | 任务 ID |
| `device_id` | string | ADB 设备序列号 |
| `package_name` | string | 被采集应用包名 |
| `status` | string | `collecting` / `stopped` / `interrupted` |
| `start_time` | string | 开始时间 |
| `sample_interval_ms` | number | 采样间隔，单位毫秒 |
| `last_error` | string | 最近一次采集错误 |
| `total_samples` | number | 当前任务累计样本数量 |
| `next_from` | number | 前端下次增量查询可传入的 `from` |
| `samples` | array | 本次返回的样本列表 |

### 停止性能采集

```http
POST /api/collect/perf/:taskId/stop
```

路径参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `taskId` | string | 是 | 开始采集接口返回的任务 ID |

说明：

- 停止成功后会把完整采集数据保存到本地 JSON 文件，同时生成一份 CSV 表格。
- 历史文件默认保存在后端运行目录下的 `data/perf/`。

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "task_id": "task_20260708_113643_abcd1234",
    "device_id": "10AF5J1JP1004SU",
    "package_name": "com.duowan.kiwi",
    "status": "stopped",
    "stop_time": "2026-07-08 11:38:20"
  }
}
```

## 历史采集接口

### 查询历史采集列表

```http
GET /api/collect/perf-history
```

说明：查询已保存的历史采集任务列表。列表接口不返回 `samples`，避免响应过大。

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "total": 1,
    "tasks": [
      {
        "task_id": "task_20260708_113643_abcd1234",
        "device_id": "10AF5J1JP1004SU",
        "package_name": "com.duowan.kiwi",
        "status": "stopped",
        "start_time": "2026-07-08 11:36:43",
        "stop_time": "2026-07-08 11:38:20",
        "sample_interval_ms": 1000,
        "sample_count": 97,
        "last_error": ""
      }
    ]
  }
}
```

### 查询历史采集详情

```http
GET /api/collect/perf-history/:taskId
```

路径参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `taskId` | string | 是 | 历史采集任务 ID |

说明：返回完整历史数据，包含 `samples`，前端可用于历史图表展示。

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "task_id": "task_20260708_113643_abcd1234",
    "device_id": "10AF5J1JP1004SU",
    "package_name": "com.duowan.kiwi",
    "status": "stopped",
    "start_time": "2026-07-08 11:36:43",
    "stop_time": "2026-07-08 11:38:20",
    "sample_interval_ms": 1000,
    "sample_count": 97,
    "last_error": "",
    "samples": []
  }
}
```

### 下载历史采集 CSV

```http
GET /api/collect/perf-history/:taskId/csv
```

路径参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `taskId` | string | 是 | 历史采集任务 ID |

说明：

- 返回 CSV 文件下载，适合前端“导出”按钮直接跳转或发起下载。
- CSV 文件保存在后端运行目录下的 `data/perf/<taskId>.csv`。
- 老历史如果只有 JSON，没有 CSV，首次下载时会自动根据 JSON 补生成 CSV。
- CSV 第一行是指标名，后面每一行对应一秒采集数据；平台没有的指标留空。

当前 CSV 表头：

```csv
collect_time,sample_index,app_cpu_percent,total_cpu_percent,memory_total_pss_mb,memory_java_heap_mb,memory_native_heap_mb,memory_stack_mb,memory_graphics_mb,fps,gpu_device_utilization_percent,frames,refresh_rate_hz,small_jank,jank,big_jank,total_small_jank,total_jank,total_big_jank
```

### 删除历史采集记录

```http
DELETE /api/collect/perf-history/:taskId
```

路径参数：

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `taskId` | string | 是 | 历史采集任务 ID |

成功响应：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "task_id": "task_20260708_113643_abcd1234",
    "deleted": true
  }
}
```

## 采样数据字段说明

### CPU

| 字段 | 类型 | 单位 | 说明 |
| --- | --- | --- | --- |
| `app_cpu` | number | `%` | 应用 CPU 使用率，已按核心数折算 |
| `total_cpu` | number | `%` | 设备整体 CPU 使用率，已按核心数折算 |

iOS 当前只返回 `app_cpu`，来源是 `pymobiledevice3` 的 `cpuUsage`。后端会按 iOS 机型核心数映射表换算：`app_cpu = cpuUsage / coreCount`；未命中机型时默认按 6 核。

### Memory

| 字段 | 类型 | 单位 | 说明 |
| --- | --- | --- | --- |
| `java_heap` | number | MB | Java 堆 PSS |
| `native_heap` | number | MB | Native 堆 PSS |
| `stack` | number | MB | 线程栈 PSS |
| `graphics` | number | MB | 图形相关 PSS |
| `total_pss` | number | MB | 应用总 PSS |

iOS 当前只返回 `total_pss`，对应 `pymobiledevice3` 的 `physFootprint`。

### iOS FPS

| 字段 | 类型 | 单位 | 说明 |
| --- | --- | --- | --- |
| `core_animation_frames_per_second` | number | fps | 对应 `CoreAnimationFramesPerSecond` |

### iOS GPU

| 字段 | 类型 | 单位 | 说明 |
| --- | --- | --- | --- |
| `device_utilization` | number | `%` | 对应 `Device Utilization %` |

### FPS

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `fps` | number | 当前 FPS |
| `frames` | number | 本次新增有效帧数量 |
| `method` | string | `surfaceflinger` 或 `gfxinfo` |
| `layer` | string | SurfaceFlinger 命中的 Layer；`gfxinfo` 时为空 |
| `refresh_rate` | number | 屏幕刷新率 |
| `has_frame_data` | boolean | 本次是否有新增帧 |
| `time_source` | string | `actual_present_time` / `display_present_time` / `frame_completed` |

### Jank

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `small_jank` | number | 本次 SmallJank 次数 |
| `jank` | number | 本次 Jank 次数 |
| `big_jank` | number | 本次 BigJank 次数 |
| `total_small_jank` | number | 当前任务累计 SmallJank 次数 |
| `total_jank` | number | 当前任务累计 Jank 次数 |
| `total_big_jank` | number | 当前任务累计 BigJank 次数 |
| `frames` | number | 本次新增有效帧数量 |
| `max_frame_time_ms` | number | 本次最大帧耗时，单位 ms |
| `has_frame_data` | boolean | 是否有可计算帧数据 |
| `method` | string | 当前固定为 `gfxinfo` |
| `time_source` | string | `display_present_time` / `frame_completed` / `unavailable` |

## 前端轮询建议

实时采集中建议使用增量查询：

1. 首次请求：

```http
GET /api/collect/perf/:taskId?from=0
```

2. 保存响应里的 `next_from`。
3. 下一次请求：

```http
GET /api/collect/perf/:taskId?from=<next_from>
```

4. 当 `status` 为 `stopped` 或 `interrupted` 时，前端应停止轮询。
