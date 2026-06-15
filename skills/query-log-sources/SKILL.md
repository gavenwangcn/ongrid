---
name: query_log_sources
description: 按设备或业务系统查询 Loki 中已索引的应用日志源（unit / 容器 / 文件），并返回可直接用于 query_logql 的 selector
when_to_use: |
  用户要排查某系统或某设备的应用日志，但还不知道具体 systemd unit、Docker 容器名或文件路径时使用。

  **典型流程（系统级故障）：**
    1. `query_systems(system_name="订单中心")` → 拿到 device_id 列表
    2. `query_log_sources(system_name="订单中心")` → 列出各设备的 unit / 容器 / 文件 + `logql_selector`
    3. 对用户关心的源（如容器 `order-api`）调用 `query_logql(query=<返回的 logql_selector> |= "error")`

  **典型场景：**
    • 「订单系统有哪些容器日志？」→ `query_log_sources(system_name="订单中心")`
    • 「设备 42 上 nginx 的 unit 日志怎么查？」→ 先 `query_log_sources(device_ids=[42])` 找 unit，再 `query_logql`
    • 「某某应用容器最近有没有 error？」→ 先本工具拿 `container` 的 `logql_selector`，再 `query_logql` 加 `|~ "(?i)error"`

  **不要用：**
    • 直接读日志行内容 → `query_logql`
    • 只列设备、不关心日志源 → `query_devices` / `query_systems`
    • 主机上实时 `docker ps`（不一定已进 Loki）→ `host_bash`（本工具优先看 Loki 索引 + 可选插件配置）

metadata:
  ongrid:
    scope: manager
    activation:
      mode: keyword
      keywords: [日志源, 容器日志, unit日志, 文件日志, logql, loki, 应用日志, container, journald, 排查日志]
---

[能力: query_log_sources]

在 **Manager** 侧查询 Loki 已索引的日志源元数据（非 MySQL 表），并按设备或业务系统聚合。

## 调用示例

```text
# 某业务系统下所有设备的日志源
query_log_sources(system_name="订单中心", lookback="24h")

# 单台或少量设备
query_log_sources(device_ids=[42, 43])

# 系统内指定几台设备
query_log_sources(system_name="订单中心", device_ids=[42])

# 只要 Loki 索引，不要 edge 插件配置里的「计划采集」项
query_log_sources(device_ids=[42], include_configured=false)
```

## 返回字段

- `window`：`start` / `end`（RFC3339）
- `devices[]`：每台设备一块
  - `device_id`, `name`, `system_name`
  - `log_label_ids`：Loki 里可能出现的 `device_id` 标签（含 legacy edge_id）
  - `units[]`：`unit`, `logql_selector`, `indexed`
  - `containers[]`：`container`, `container_id`, `logql_selector`, `indexed`
  - `files[]`：`path`, `kind`（file_path / journald / filename / ongrid_source）, `logql_selector`
  - `configured`（可选）：edge logs 插件配置 `file_paths` / `journald_units` / `enable_docker_api`
  - `hints`：`recent_errors_example`, `error_count_5m_example`

`indexed=false` 表示仅在插件配置里出现、时间窗口内 Loki 尚无数据。

## 与 query_logql 配合

```text
# 1. 发现容器名
query_log_sources(system_name="订单中心")

# 2. 用返回的 logql_selector 查错误（示例）
query_logql(query="{device_id=\"42\",container=\"order-api\"} |~ \"(?i)error\"", limit=200)

# 3. 统计 5 分钟内 error 条数（示例）
query_logql(query="sum(count_over_time({device_id=\"42\",container=\"order-api\"} |~ \"(?i)error\"[5m]))")
```

## 与 query_systems 的分工

| 工具 | 用途 |
|------|------|
| `query_systems` | 业务系统 → 设备清单 / 在线数 |
| `query_log_sources` | 设备 / 系统 → Loki 日志源 + LogQL selector |
| `query_logql` | 执行 LogQL，读日志内容或聚合 |
