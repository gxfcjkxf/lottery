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

