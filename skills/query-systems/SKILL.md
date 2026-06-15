---
name: query_systems
description: 查询业务系统列表及各系统下的设备清单（manager 只读）
when_to_use: |
  用户问「有哪些系统」「某系统下有哪些机器」「按业务系统看设备分布」时使用。

  **典型场景：**
    • 「现在纳管了哪些业务系统？」→ `query_systems`（只看数量）
    • 「订单中心有哪些设备、在线情况？」→ `query_systems(system_name="订单中心")`
    • 「每个系统各多少台、要不要展开设备列表？」→ `query_systems(include_devices=true)`

  **不要用：**
    • 按角色/在线/名称筛设备、不关心系统分组 → `query_devices`
    • 看 Manager/Loki 等部署拓扑 → `get_topology`
    • 单台设备深度诊断 → `get_edge_summary`

metadata:
  ongrid:
    scope: manager
    activation:
      mode: keyword
      keywords: [系统, 业务系统, system_name, 订单, 纳管, 设备清单, fleet]
---

[能力: query_systems]

在 **Manager** 侧只读查询 operator 填写的 `system_name`（业务系统名）分组：

## 调用示例

```text
# 列出所有系统 + 每台数量（不含设备明细）
query_systems()

# 列出所有系统，并在每个系统下带设备摘要
query_systems(include_devices=true, devices_per_system=20)

# 查询单个系统下的设备
query_systems(system_name="订单中心")
```

## 返回字段

- `systems[]`：每个元素含 `system_name`、`device_count`、`online_count`、`offline_count`
- 可选 `devices[]`：`device_id`、`name`、`hostname`、`system_name`、`device_ip`、`online`、`roles`、`last_seen_at`
- `system_name` 为空字符串表示尚未分配业务系统的设备

## 与 query_devices 的区别

| 工具 | 用途 |
|------|------|
| `query_systems` | 按 **业务系统** 分组盘点 |
| `query_devices` | 扁平设备列表，可按 role/online/name 过滤 |
