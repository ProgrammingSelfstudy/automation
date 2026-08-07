# PerfRabbit Web

本地移动设备性能测试工作台，已接入以下接口：

- `GET /api/device/list`
- `GET /api/devices/:deviceId/apps`
- `POST /api/collect/perf/start`
- `GET /api/collect/perf/:taskId`
- `POST /api/collect/perf/:taskId/stop`

## 启动

```bash
npm install
npm run dev
```

开发服务器默认运行在 `http://localhost:5173`，并将 `/api` 代理至 `http://127.0.0.1:8081`。

后端调试入口默认端口是 `8081`，打包后的一体化程序默认端口是 `8080`，两套互不影响。

开始采集成功后，前端会保存返回的 `task_id`，每秒查询任务性能快照并实时绘制 FPS、CPU、内存和 Jank 曲线。手动停止或检测到应用进程退出时，会调用对应任务的停止接口。

## 构建

```bash
npm run build
```

如需部署到独立域名，可复制 `.env.example` 为 `.env`，设置 `VITE_API_BASE_URL`。
