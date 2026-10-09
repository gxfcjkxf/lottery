# 运维可观测性交接

本阶段提供 API/worker 的独立保护端口、Prometheus 指标、有界只读数据库快照和可选 OpenTelemetry 链路。不新增业务接口、权限、迁移、资金开关或自动重试。发布与恢复见[34号手册](34-release-and-operations.md)；生产部署、告警接收人、容量、RTO/RPO 和客户人工审核仍需验收。

## 配置与保护端口

默认监听和追踪地址都为空：不启动运维监听、数据库指标刷新或外发追踪。两个进程分别读取对应端口，必须不同。

| 配置 | 合同 |
| --- | --- |
| `API_METRICS_ADDR` | API端口，仅字面回环IP，例127.0.0.1:9091 |
| `WORKER_METRICS_ADDR` | worker端口，仅字面回环IP，例127.0.0.1:9092 |
| `METRICS_TOKEN_FILE` | 任一监听启用时必填；普通非符号链接文件，仅0400/0600权限，令牌32–128字节 |
| `TRACE_OTLP_ENDPOINT` | 明确OTLP/HTTP地址；HTTPS，或仅字面回环IP的HTTP；禁止URL凭证、query、fragment |
| `TRACE_SAMPLE_RATIO` | 有限数0–1，默认0.1 |

`platform generate-metrics-token <file>` 无数据库依赖，排他创建0600文件，生成256位URL-safe令牌，不能替代 `AUTH_KEY_FILE`。不把令牌放进URL、命令参数、日志或Git。监听没有TLS，私有局域网、公网及通配地址也拒绝；远程抓取须另行审核安全代理或本机转发。

仅GET/HEAD `/metrics`、`/health/live`、`/health/ready`；均要求Bearer头，缺失/错误凭证401，非读取方法405，未知路径404，响应禁止缓存。无pprof、无公共API `/metrics`。主机访问和令牌文件权限仍须由部署方管理。

API/worker启动前核验完整嵌入迁移清单及校验和，失败退出，不自动migrate。API先验证认证密钥，再启动监听。ready检查主库及完整迁移；worker另要求period_tick、notification两循环在最近30秒完成。完成不等于业务成功、队列清空或财务已批准。监听异常取消进程，SIGTERM有界退出。

## 指标口径

| 指标族 | 标签/含义 |
| --- | --- |
| `lottery_build_info` | service=api/worker、环境、构建版本；发行构建应嵌入审核后的Git SHA |
| `lottery_http_requests_total`、`lottery_http_request_duration_seconds` | 固定注册路由、封闭HTTP方法、状态码；未匹配路径统一unmatched |
| `lottery_worker_runs_total`、`lottery_worker_committed_items_total` | 15固定组件、success/error；执行次数与实际提交数分开，部分提交后报错仍保留真实数量 |
| `lottery_worker_run_duration_seconds`、`lottery_worker_last_completed_timestamp_seconds`、`lottery_worker_inflight` | 耗时、最近完成时间、在途数；不改变调度 |
| `lottery_database_up`、`lottery_ops_snapshot_*` | 主库ping及最近快照尝试成功、时间、耗时 |
| `lottery_work_pending`、`lottery_work_oldest_pending_age_seconds`、`lottery_work_failed` | 固定9类：通知、结算、期次退款、结果更正、佣金周期、派发、更正、归档、提现 |
| `lottery_reconciliation_observed`、`lottery_reconciliation_discrepancies` | 各品牌最近完成的业务对账；无历史不是健康证明，不是持续实时检查 |
| `lottery_replication_stats_visible`、`lottery_replication_observed`、`lottery_replication_lag_bytes` | 主库实际streaming从库WAL字节差；无观察/权限不足不报告零延迟 |
| `lottery_db_pool_*_connections` | 主库及最多8个配置从库的进程内池状态，primary/replica_1…8 |

提现processing是人工队列，不是自动出款；驳回/失败退款不计技术失败。佣金待审核、余额不足暂停不是可自动执行的pending工作。指标无品牌、用户、订单、余额、数据库地址、原始路径或凭证标签。

数据库刷新立即尝试，之后每30秒一次、不重叠；ping、只读repeatable-read事务及查询合计最多2秒，SQL另限1.8秒。抓取只读取缓存。来源失败或快照超过75秒时省略业务指标、保留诊断元数据；缺失为未知，不能补0。schema来自受信任目录且显式限定，禁止临时表遮蔽。

双进程会重复看到同一数据库数量，示例用 `max by(kind)`，不能相加。多数据库部署须配置受控集群标签并按集群去重，不能跨库max隐藏积压。

## 追踪与日志

独立SDK/registry，不改全局provider。仅固定服务身份、注册路由、方法/状态及worker组件；不保存请求体、URL参数、Host、连接串或业务身份。保留合法W3C parent trace/span ID，丢弃tracestate、不提取baggage。HTTP结构化日志可关联实际trace_id/span_id，不以外部追踪值授权。

导出HTTPS校验、无环境代理、2秒超时、无重试；队列1024、单批256、批等待5秒、退出flush最多5秒。忽略通用OTEL exporter/header/resource环境值，故障日志固定安全文本。已有业务日志仍须按部署权限和保留策略管理。

## 文件与验收

- `monitoring/prometheus.yaml`：两个本机保护端口及私有抓取凭证文件，不能直接用于其他拓扑。
- `monitoring/alerts.yaml`：11条工程起始告警，无接收器、自动修账、重结算或出款动作。
- `monitoring/alerts.test.yaml`：冷启动循环缺失、双进程去重、提现排除及未知复制延迟场景。
- `monitoring/grafana-dashboard.json`：6个只读面板；查询校验，不等于Grafana导入/视觉验收。

`PROMTOOL_BIN=<绝对路径> node scripts/verify-monitoring.mjs` 在自有临时目录生成凭证副本，执行完整配置、规则、场景及面板PromQL验证。CI固定Prometheus3.15.0 Linux amd64官方下载/SHA256，下载大小和时间有界，无自动重试。

`scripts/verify-ops-runtime.mjs` 仅接受明确确认的 `lottery_ops_acceptance` 空测试库，正常CLI迁移/开发seed后运行真实serve/worker。核验保护鉴权、路由基数、核心循环ready、schema拒绝且不自动迁移、无效密钥/端口/令牌/非回环地址失败关闭及SIGTERM退出。120张表前后摘要一致，无财务重试；证据留在自有 `.local/ops-runtime-*`，不上传密钥。

本地验证不代表启用追踪后的500次/秒生产性能。macOS未执行systemd示例，Grafana未渲染，Docker/云/TLS/告警通知尚未部署；这些限制不能以测试通过替代。
