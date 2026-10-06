# 技术架构与运维规格

## 1. 架构基线

第一阶段采用模块化单体，不拆分独立微服务；所有模块通过清晰接口和领域事件解耦，未来可独立拆分。

```text
user-web / admin-web
          |
      CDN / WAF / Load Balancer
          |
       Go API
          |
  -----------------------------
  | Identity / Brand / RBAC     |
  | Game / Rule / Period        |
  | Bet / Settlement            |
  | Ledger / Recharge / Withdraw|
  | Agent / Commission / Reward |
  | Notification / Report / Audit|
  -----------------------------
       |             |
 PostgreSQL       Redis
 primary/read     cache/limit
 replicas         idempotency aid
       |
  Outbox -> NATS JetStream/worker
       |
 external result sources / object storage
```

## 2. 技术选型

### 后端

- Go；HTTP API 使用 REST/OpenAPI。
- 代码按 domain/application/adapter/infrastructure 分层。
- 数据库访问使用显式事务和参数化查询；禁止业务层拼接 SQL。
- 规则引擎作为独立 package，有纯函数式校验、展开和结算入口，便于单元测试。

### 前端

- `user-web` 和 `admin-web` 两个独立 Vue 3 + TypeScript + Vite 项目。
- PWA 使用 service worker，但不缓存用户余额、注单提交和提现状态等敏感动态数据。
- 共享包只放 UI tokens、基础组件、API 类型、国际化工具和品牌主题，不共享权限绕过逻辑。

### 数据和任务

- PostgreSQL：唯一业务事实来源。
- Redis：会话、热点配置、限流、短期锁和幂等辅助；不作为账本唯一来源。
- NATS JetStream 或等价持久化队列：异步任务和领域事件。
- 对象存储：外部开奖原始响应引用、品牌素材、导出文件和备份。
- PostgreSQL Outbox：业务事务提交后再投递事件，避免“数据库成功但消息丢失”。

## 3. 模块边界

- `identity`：全局用户、密码、Telegram、会话。
- `tenant`：品牌、域名、主题、配置继承。
- `membership`：品牌成员、加入来源、用户状态。
- `access`：后台账号、角色、权限、品牌范围。
- `game`：彩种、玩法、规则版本和限额。
- `period`：期次计划、状态和暂停规则。
- `draw`：来源适配器、校验、人工开奖和结果纠正。
- `bet`：选号、预览、注单、取消和幂等。
- `ledger`：账户、来源余额、冻结、解冻、冲正和对账。
- `settlement`：中奖判断、奖级、派奖和重算。
- `withdrawal`：提现资格、申请、审核和内部出款模拟。
- `agent`：代理树、佣金、奖励和周期结算。
- `notification`：站内通知和可插拔外部渠道。
- `report`：只读报表、汇总和导出。
- `audit`：不可变操作审计。

模块只能通过公开 application service 或事件调用其他模块；不能直接修改其他模块的表。

## 4. 一主多从和一致性

- 主库负责所有写入和关键读：余额、注单刚提交结果、提现状态、规则发布、开奖确认和结算状态。
- 从库负责历史列表、报表、统计和非实时查询。
- 关键写操作返回主库生成的结果；前端在短时间内使用主库粘滞读取。
- 每个读请求标注 consistency：strong 或 eventual；资金相关默认 strong。
- 连接池、PgBouncer、慢查询监控和复制延迟监控必须配置。
- 主库至少有可自动切换的备用节点；“一主多从”不等同于没有故障切换。

## 5. 高并发投注路径

目标：峰值约 500 在线用户、每秒约 500 次投注请求。

投注请求：

1. 网关限流和身份校验。
2. 从缓存读取品牌/彩种/玩法配置并校验版本。
3. 期次和限额读取主库或带版本缓存。
4. 以用户品牌账户行作为锁粒度开启事务。
5. 校验幂等键和请求摘要。
6. 计算复式、倍投、总积分和来源扣款。
7. 扣减账户、写账本、写注单、写 Outbox。
8. 提交事务后返回订单结果。

禁止：先扣 Redis 再异步写账本、在从库上扣款、只依赖前端防重复、提交后再补写订单。

## 6. 异步任务

### S4-c1 期次 worker

`go run ./cmd/platform worker` 是与 HTTP API 分开的可选进程，使用相同配置连接主 PostgreSQL；API 不会隐式启动 worker。`cmd/platform` 内嵌 tzdata，精简镜像也可加载 IANA 时区。worker 每秒执行 Tick，每轮最多挑选 25 个待处理彩种、每彩种最多锁定处理 100 条到期 periods；每个彩种一个事务，锁顺序为 game → period → play。多个实例可共享同一主库运行，行锁和状态转移避免重复开期。Tick 使用 PostgreSQL 主库时钟；只有实际开期才递增 `started_sequence`。漏过投注窗口的 pending 期次判定取消，不补开。每分钟 FillCalendar 为 active 且有日历的彩种保留未来 24 小时期次；相同时间窗复用既有记录及原 schedule 快照。

S5-a4 整期取消先按 game → period 排他锁序，与在途下注/开期/结果提交串行化。关闭期次和完整退款目标同一事务提交，旧开奖结果/尝试不删除，采集租约清除。独立退款循环每秒运行一次，每轮至多处理 20 个目标（服务硬上限 100），不阻塞期次时钟或采集循环；每个目标独立事务，按 game/period 共享锁 → cancellation task → target → account → order 处理，一次只持有一个钱包。

多 worker 通过任务行 `FOR UPDATE SKIP LOCKED` 协调；已由另一路取消的目标验证既有原借记冲正，不重复退款。中断留下 pending，后续 worker 可恢复；交易/业务失败回滚本笔后追加不可变失败记录并暂停任务，只有显式人工重试才重新开放 failed 目标。父任务和目标状态的延迟约束防止虚假完成，成功过的目标不会重做。HTTP 202 只代表取消已受理，客户端必须读取主库摘要确认 completed；大量目标的最终退款延迟与目标吞吐量仍需压测，不把有界循环当作 500 次/秒负载证明。

单个彩种失败会累积为本轮错误并记录日志，其他彩种继续处理。期次循环保留日历并推进状态，独立开奖循环按下节处理来源；均不执行支付、投注或结算。当前适配器不发起外部网络请求。API 与 worker 都必须连接同一主库；本地浏览器回归也要为 API 和 worker 配置同一 project 的临时测试库。

### 开奖 feed 当前边界

S4-c2 已接入不可变来源修订、人工结果、当前结果指针和采集证据。API/DOM 仍是无网络 stub，不实现真实 API 或网页抓取。worker 有独立两秒采集循环，不阻塞一秒期次时钟；每轮最多五个待开奖期次、总上下文十秒，每来源合作式超时五秒（适配器必须遵守取消，不用无限 goroutine 强杀）。主库短事务按 game→period 锁定并取得三十秒租约，释放锁后采集，再锁定复核版本、来源修订、租约和当前结果。失败退避三十秒；在途人工/配置变更使旧采集 discarded，过期旧 worker 不得清除新租约。每个备用来源调用前检查租约，候选时间用主库时钟验证；最终再次验证最新上一期结果。尝试只保存安全错误码，结果和审计原子追加。当前不执行投注、结算或派奖；已结算结果纠正待 S5。

事件至少包括：

- `period.created`
- `period.closed`
- `draw.source.failed`
- `draw.result.confirmed`
- `draw.result.corrected`
- `bet.order.placed`
- `bet.order.cancelled`
- `period.settlement.requested`
- `settlement.completed`
- `ledger.changed`
- `withdrawal.created`
- `withdrawal.status_changed`
- `commission.cycle_requested`
- `notification.requested`

消费者必须保存 event_id，重复消费不得重复扣款、派奖、佣金或通知。

S6-b 站内通知由独立的 PostgreSQL worker 循环消费已提交 outbox（不要求本阶段接入 NATS）。`0019` 在 outbox 插入事务内将六类事件加入投递队列，一次性补入已有真实事件，不周期扫描全部历史 outbox。每 100ms 唤醒，单轮最多 100 条、上下文最多 10 秒；这是有界突发额度，不是已证明的吞吐，多实例数量/延迟须由 S7 压测确定。多 worker 以 SKIP LOCKED 分工，每事件独立事务。消息落库、`consumed_events(in-app-notification-v1,event_id)` 和投递 sent 状态一起提交。业务事实由源表验证，只映射安全公共字段，不把原始事件/审计直接发给用户。事务失败用 savepoint 全部撤销消息/确认，再记录安全错误码和退避；无效事件隔离为 failed，其余不受阻。投递状态独立于 outbox.published_at，为未来发布器或其他消费者保留各自进度。此任务与开奖 I/O、退款 worker 和期次时钟独立；不发邮件、短信、Telegram，不产生账本变更。

## 7. 可靠性和安全运维

- API 超时、限流、重试和熔断；外部开奖源按来源单独隔离。
- 结构化日志包含 request_id、brand_id、actor_id、resource_id 和 event_id，不记录密码、Telegram 授权原文或完整敏感凭证。
- OpenTelemetry 链路追踪；Prometheus/Grafana 指标；集中日志和告警。
- 关键告警：主库不可用、复制延迟、账本对账差异、队列堆积、结算失败、开奖源连续失败、提现异常增长。
- 数据库每日备份、定期恢复演练；对象存储启用版本和生命周期策略。
- 配置和密钥使用环境变量/密钥管理服务，不进入 Git。
- 测试、预发布、生产环境隔离；生产数据不能复制到开发环境。

## 8. 性能和验收

压测至少覆盖：

- 500 次/秒投注创建持续负载；
- 同一用户并发投注和重复幂等请求；
- 多品牌并发投注；
- 期次截止瞬间投注；
- 结算任务与投注并发；
- 主从复制延迟；
- 队列消费者变慢或重复消费。

验收还需确定 P95/P99 延迟、峰值持续时间、可用性、恢复时间和数据恢复点目标。

### 可复现的受控投注负载

`backend/internal/capacity` 提供固定到达速率、受限在途数量的开放式压测驱动；请求完成不会决定下一次发起时间。在途已满时记录压测端丢弃，而非无限创建协程。时延同时保存执行、计划发起延迟与从计划时间起算的端到端分位数；到达速率、回执完成速率和实际新增注单速率分别报告，重复回执不计为新投注。

容量入口位于 `capacity` 构建标签下，仅支持Linux/macOS。测试必须确认专用回环PostgreSQL，拒绝原开发库55432；每轮另建并清理自己的随机schema。500个合成会员/真实会话预先生成，测试积分经账本服务入账，真实规则须独立审核；下注、会话检查、幂等、扣分、审计及默认每100ms/100条通知消费者均使用现有服务。登录/注册压力、TLS/代理/生产日志成本和真实PWA终端不在这条基线里。不能把合成会话数等同于500台真实在线设备。

先在独立测试数据库验证基础fixture，再从`backend`执行；报告路径必须新建，不能覆盖既有证据：

```sh
export TEST_DATABASE_URL='postgres://lottery_test@127.0.0.1:55529/postgres?sslmode=disable'
export LOTTERY_CAPACITY_CONFIRM_ISOLATED=yes
CGO_ENABLED=0 go test -tags capacity ./internal/httpapi -run '^TestCapacityFixtureSmoke$' -count=1
LOTTERY_CAPACITY_USERS=500 LOTTERY_CAPACITY_RATE=500 LOTTERY_CAPACITY_SECONDS=30 \
LOTTERY_CAPACITY_REPORT='/absolute/test-output/fresh-run.json' \
CGO_ENABLED=0 go test -tags capacity ./internal/httpapi -run '^TestBettingCapacity$' -count=1 -v
```

专用数据库及输出目录必须由操作人先创建；示例端口不是既存服务承诺，禁止指定客户生产库。`LOTTERY_CAPACITY_POOL`默认20、在途上限默认500；`BRANDS=2`用完整变量名`LOTTERY_CAPACITY_BRANDS`启用两个品牌。单用户重试使用`LOTTERY_CAPACITY_USERS=1`及`LOTTERY_CAPACITY_DUPLICATE_GROUP=5`，每五个请求保留同一账号/正文/键。`LOTTERY_CAPACITY_CUTOFF_SECONDS=6`在创建期次六秒后截止，应配合足够长的测试窗口；准备阶段已过截止会明确失败，不偷偷延长业务时间。通知消费者默认启用，`LOTTERY_CAPACITY_NOTIFICATIONS=off`只适用于明确标注的不带消费者对照。

每轮在真实HTTP处理器停止后核对已提交账本：注单与扣分笔数/积分相符，每笔引用在同品牌/会员/账户，账户余额与全部流水守恒，重复意图不多建订单，期次窗口外没有注单。读取数据库连接/锁等待、持久队列、事务及块缓存计数；计数重置时标记并禁止使用原窗口命中率。复制未配置或延迟NULL明确不可用，不填成0。PostgreSQL块缓存命中率不是Redis命中率；数据库CPU仅统计前后可见的同一批客户端进程累计CPU，不覆盖WAL/检查点/新退出进程；Go CPU包含同进程API、驱动和通知消费者，不能冒充独立API实例CPU。
