# Lottery 平台

当前项目尚未发布。数据库只保留一份完整基线 `backend/migrations/0001_baseline.up.sql`，使用空库执行 `platform migrate` 后启动。旧开发库不会被自动升级、清空或修复。

开发遵循 [AGENTS.md](AGENTS.md)：不写旧版本兼容或错误兜底代码，保持 KISS，不为臆想的极限场景堆砌分支。当前钱包及流水统一四来源16分项；接口只接受当前格式。业务账本、审计、结果更正和归档版本仍保留，它们不是版本兼容。

[佣金周期分析](docs/38-commission-cycle-analysis.md)区分保存周期的核算、有效目标、实际净入账及数学差额，包含周期窗口外的后续资金记录；未知不是零，读取与导出不授权派发。[零原派发人工入账保护](docs/40-zero-original-commission-evidence.md)阻止已有人工入账的周期被全额重复派发。

[品牌资金业务引用检查](docs/37-brand-business-inventory.md)补查逐账户对账可能遗漏的孤立记录；只观察声明范围，不自动补账，也不把结构一致当作金额链或完整财务证明。

[工作台佣金任务汇总](docs/36-commission-workbench.md)显示独立授权的发现、核算、派发和更正任务状态；不表示欠付金额、收益或资金操作授权。

[佣金历史核算明细](docs/35-commission-allocation-history.md)可按明确run查询整周期收益及逐单未舍入差额分配；不以今天的配置覆盖旧证据，不代表已派发或钱包余额。

运维工程交接见[可观测性合同](docs/33-operational-observability.md)和[发布恢复手册](docs/34-release-and-operations.md)。保护端口及追踪默认关闭，监控只读，不授权自动资金重试；生产部署和客户人工审核仍需独立验收。

多品牌彩票运营平台。实现依据位于 [docs/README.md](docs/README.md)，阶段验收与当前覆盖范围位于 [docs/implementation-progress.md](docs/implementation-progress.md)。

[审计管理](docs/32-audit-query-and-export.md)支持品牌、UTC时间及操作/人员/资源筛选，独立授权的完整CSV下载、已提交审计证据和摘要核验。0071只扩展bootstrap导出权限及查询索引；自定义角色不扩权，查询和下载不移动积分或改写历史。

后台“人工奖励”已接入独立权限、真实赠送积分发放、全额原路撤销及不足待处理，支持中英双语、PC与移动布局。提交须明确核对，结果未知只能恢复原请求，当前查询与原回执分开；余额补足不自动继续。0056接入三种不可变奖励站内消息及固定历史说明，通知消费不改变积分；0057接入实际入账及当前订单队列两种报表、完整CSV和工作台状态计数。交接及专项测试环境见[人工奖励合同](docs/20-manual-reward-orders.md)与[奖励报表合同](docs/21-reward-reports.md)。

0058为开奖结果更正后已有真实佣金入账的周期准备[不可变差额计划](docs/22-commission-correction-plans.md)，0059提供独立默认关闭的[差额执行](docs/23-commission-correction-execution.md)：新批准、仅佣金来源补发/追回、整周期暂停及显式继续。后台“佣金更正”和十二条正式管理接口已接入，原回执与当前查询分开，同会话未知操作只能恢复原请求，见[管理交接合同](docs/24-commission-correction-management.md)。0060接入实际非零差额的不可变站内消息及佣金报表补发/追回分项，CSV为版本2、22列，详见[通知与报表合同](docs/25-commission-correction-observability.md)。OPEN-117已确认新核算覆盖人工修正净额；既有人工修正、审计与账本保留，原blocked派发不解锁。历史政策未决计划只能按[人工修正后重新核算合同](docs/39-commission-manual-recalculation-policy.md)显式审计重试，不自动补偿或恢复。0073仅替换函数；两个资金开关仍默认关闭，升级不迁移资金或自动重试。0073开发验证及对应远程CI已通过；客户生产发布与人工审核另行完成。

第三方接入请使用 [已实现 OpenAPI](docs/openapi.json) 与 [接口交接说明](docs/09-openapi-handover.md)。运行 `pnpm api:generate` 更新，`pnpm api:check` 和 `pnpm test:contracts` 检查实际路由及代表性数据模型；完整业务设计中的未来接口不代表已经可调用。

日/月采用品牌时区不可变快照及追加版本，财务按实际入账，默认不自动删除。后台人工查询、创建/追加与原JSON下载已接入；0067提供默认关闭的[自动核心与worker](docs/27-automatic-report-archive-core.md)，[任务查询/人工重试](docs/28-report-archive-task-management.md)已接入双端后台。[正式配置PUT和双端编辑](docs/29-report-archive-activation.md)按确认的首次当天/当月起点保存，原回执与最新配置独立显示，未知请求只能显式原键恢复。系统归档明确显示任务来源，不冒充管理员操作；升级不回补旧档或改变积分。

[投注历史归属报表](docs/30-attribution-reports.md)提供跨彩种、会员、保存的直属代理及加入方式统计，支持直属/下级链过滤和完整CSV；同一注单不在祖先分组重复计数，不按当前关系重建历史，也不将运营投影当作佣金派发授权。

[开奖历史站内通知](docs/31-draw-result-notifications.md)按本期投注会员去重，正式公布和更正各留一条历史结果，取消/异常注单会员仍属于受众。中英页面保留原号码、前导零及独立历史说明，不将通知当作中奖或派奖成功；模板和消息不改变积分，也不接外部发送渠道。

后台“品牌和域名”中的新建品牌、运行状态、域名和展示配置已接入真实服务及审计。创建需精确的 `brand.create.platform` 权限，新品牌固定暂停、版本1；不会自动创建管理员、会员、域名、游戏或积分。暂停只阻止新投注，不撤销登录或停止已有订单退款、开奖和结算。配置使用品牌共享版本，提交前应重新读取并核对。

同品牌、同彩种上期必须已结算，或整期取消且退款全部完成，才开放下一期投注；错过原窗口的期次直接跳过。隔离一主两从及备份恢复验收见[复制与恢复手册](docs/10-replication-and-recovery.md)，不代表自动故障切换或生产灾备完成。升级需先执行包含0032函数查找路径修复、0033品牌创建及0034合规配置的迁移，再重启API/worker；服务不会自动迁移。

后台“风控与合规”提供年龄、地区、身份检查配置、不可变历史、显式伪检查及真实业务拒绝记录。初始全部关闭；allow/CHECK_DISABLED仅表示跳过，并非用户已验证。任一检查开启且无真实适配器时，服务端拒绝新注册、首次加入品牌、运营新增成员、待确认成员首次接受条款，以及投注预览/提交，返回409 COMPLIANCE_REVIEW_REQUIRED；已有正常登录、查询、取消和原路退款不被拦截。拒绝不建身份、不扣分、不建单，证据与加密负回执在业务回滚后同事务保存，原键重放不重做业务或重复证据。当前不收集证件/生日/用户地区，不建立复核队列、不冻结资金；提现已接入内部积分流程，不执行外部付款。真实验证与责任博彩仍需继续验收，不能作为生产合规完成的依据；当前版本按最新迁移协调升级。

## 开发环境

- Go 1.26+、Node 24+、pnpm 11.19.0。
- PostgreSQL 17；Redis、NATS JetStream、MinIO 可通过 Docker Compose 启动。
- 本地环境不处理真实支付；前端的原型演示流程有明确模拟标记。

启动步骤：

~~~sh
cp .env.example .env
docker compose up -d
set -a
source .env
set +a
pnpm install --frozen-lockfile
cd backend
go run ./cmd/platform generate-auth-key .local/auth.key
cd ..
make migrate
make seed
make api
~~~

期次调度需要额外启动一个共享同一主数据库的 worker；API 不会自动启动它：

~~~sh
cd backend
go run ./cmd/platform worker
~~~

分别在另两个终端启动前端：

~~~sh
pnpm dev:user
pnpm dev:admin
~~~

- 用户端：http://localhost:5173
- 管理端：http://localhost:5174
- API：http://localhost:8080
- 存活：http://localhost:8080/health/live
- 就绪：http://localhost:8080/health/ready
- 品牌上下文：http://localhost:8080/api/v1/context
- 平台路径品牌：http://localhost:8080/api/v1/b/harbor/context

本地种子提供 Aurora、Harbor 两个品牌。Aurora 绑定 localhost，Harbor 绑定 harbor.localhost；平台入口 localhost 可以通过 /api/v1/b/{brandCode} 解析品牌。种子命令不创建内置密码，且在 production 环境拒绝执行。

认证密钥只生成一次，保存在 `backend/.local/auth.key`（文件权限 0600，Git 忽略）。`.env` 中的 `AUTH_KEY_FILE` 相对 backend 工作目录解析。也可从密钥管理器注入 base64 格式 32 字节 `AUTH_KEY`；多实例必须共享同一密钥。密钥用于加密幂等响应及请求摘要，不能丢失或直接替换，否则已有幂等记录无法解密。生产环境开启 Secure Cookie，需 HTTPS 与保留原始 Host 的可信反向代理。

认证服务要求 `DB_MAX_CONNS >= 4`，预留连接供独立的限流/挑战事务，避免认证并发耗尽连接而互相等待。反向代理部署时设置 `TRUSTED_PROXY_CIDRS`，只信任实际代理网段；默认忽略转发 IP，始终不信任转发 Host。初始限流是每 IP 30 次、每账号 10 次/5 分钟，生产部署须根据 NAT、代理和流量规划调整，不是 500 次/秒投注限额。

后台没有默认密码。由服务器拥有者显式创建账号：先从密钥管理器注入 `BOOTSTRAP_ADMIN_PASSWORD`（16–128 字节；不要放在命令参数、仓库或日志中），再从 backend 执行：

~~~sh
go run ./cmd/platform create-admin --username operator --brand aurora
# 平台账号可跨品牌查看用户，但不能修改用户或踢人。
go run ./cmd/platform create-admin --username platform_reader --super
~~~

创建后清除引导密码环境变量。用户与品牌凭证全局共享，品牌成员资料及会话按品牌隔离。用户注册和新品牌加入必须接受该品牌当前条款；开发 `dev-1` 政策是占位文本，不是生产条款。客户端使用 HttpOnly、SameSite=Strict Cookie；API 同时支持 Bearer 认证。浏览器不保存访问令牌。变更请求必须带 JSON Content-Type 和 Idempotency-Key；使用 Cookie 的请求还必须带匹配 Host 的 Origin。

平台账号的服务器引导角色包含品牌创建权限，不代表允许修改用户或资金。0033只补充已绑定超级管理员的服务器引导平台角色，不自动扩展自定义角色。`POST /api/v1/admin/brands` 不发送 `X-Brand-ID`；创建回执是初始快照，后续状态须独立读取。创建后由服务器拥有者显式 `create-admin --username new_operator --brand 新品牌代码`，或按已有账号/角色管理流程授权；不得生成默认密码或自动给创建者品牌写权限。

未种入品牌的空库也可管理：服务器拥有者须先在 `platform_domains` 配置实际管理入口主机名并启用，再显式创建平台账号。正常迁移不自动信任任意域名，开发种子仅初始化localhost入口。此时平台管理登录、会话恢复、品牌列表及创建可用；裸 `/api/v1/context` 仍404，不能自动挑选新品牌替代用户入口。真实DNS、TLS与反向代理由部署方配置，服务始终依据实际Host而非转发Host。

共享密码重置会影响全部品牌。按已确认规则，操作者须对该用户每个已加入品牌都有重置权限；仅有一个品牌权限时不得修改多品牌用户的全局密码。超级管理员始终不能修改用户。

管理端账号/角色、运营新增成员、认证配置、积分账本和人工充值/冻结/调整已接入真实 API。角色按品牌隔离，授予权限不得超出操作者的目标品牌权限；账号角色/状态/密码变更撤销目标会话。运营新增成员必须由本人首次登录确认条款。用户钱包与流水展示真实余额；后台也已接入独立玩法模拟 API：`POST /api/v1/admin/rule-simulations`，需显式品牌上下文和 `rule.simulate.brand` 或 `rule.simulate.platform` 权限。它只计算并审计，不投注、不改积分、不创建订单，也不发布或审批玩法。

S4-b 已接入彩种/玩法创建、规则草稿、持久化用例验证、送审、独立审核及旧定义克隆。目录读取需 `game.view.brand` 或 `game.view.platform`、创建需 `game.write.brand`；规则读取需 `rule.view.brand` 或 `rule.view.platform`，写入、验证、送审、审核分别需 `rule.write.brand`、`rule.validate.brand`、`rule.submit.brand`、`rule.review.brand`。这些工作流的全部超级管理员写入均拒绝；创建者及所有草稿编辑者不能审核。验证报告保留完整输入、输出、定义 hash 和警告，批准须确认警告；`version` 乐观锁与历史 `version_no` 不同。立即模式审核通过即生效，下期模式每玩法仅一个待生效版本，由内部实际开期事务激活。S4-c1 已接入不可变日历修订、期次生成与可选调度 worker；旧定义克隆仅支持 active/expired/rolled_back，新草稿定义不可改写且须重新验证审核。后台六个模板不是完整 DSL 编辑器；版本工作流不动积分。用户投注/提现与业务订单仍待后续接入；详细边界见 [玩法规则引擎](docs/03-rule-engine.md)、[API 契约](docs/04-api-contract.md)，阶段验收见 [实施记录](docs/implementation-progress.md)。

后台新建规则草稿默认选择立即生效（`immediate`）；创建、更新及克隆 API 仍必须显式发送 `effect_mode`，后端没有默认值。

Telegram 使用当前 OIDC 登录与一次性 nonce；品牌 `auth_config` 中设置 `telegram_enabled` 和 `telegram_client_id` 后，还需在 Telegram 配置允许的域名。未提供真实应用配置时默认关闭，不能用任意 Telegram 用户名冒充授权。验证码通过 `auth_config.captcha_enabled` 开启，是基本图形挑战，并不替代 WAF 和反自动化服务。

迁移执行器在事务中获取 advisory lock，并保存 SQL checksum；已应用迁移不可修改。重复执行 migrate/seed 是幂等的。服务启动不自动迁移，部署时应先执行迁移命令。

## 测试

S4-c2 已接入来源配置的不可变修订、采集尝试历史、人工补录以及结算前人工覆盖外部结果。后台“期次和开奖”页面使用真实管理 API，人工结果优先且不会被旧在途采集覆盖。API/DOM 适配器仍为无网络伪实现；worker 记录 no_data，不抓取真实第三方数据。该阶段不执行注单结算、派奖或支付。

S4-d 已提供通用运营编辑器，与六个快捷模板并存：自定义彩种模型、全部 schema-v1 条件、多个奖级、属性/特征、限额和舍入，以及最多 32 组独立验证用例。非模板定义不再只读，普通草稿可编辑，回滚来源草稿仍锁定。验证和品牌独立审核仍走原真实 API，不绕过审批，也不自动生成商业赔率安全结论。

S5-a1 新增真实投注/取消核心与 API：预览规范选号和展开复式，确认按充值→中奖→赠送扣分，订单/账本/审计/outbox 同事务，取消全额原路退款。每单保存当时规则/策略，立即批准的新规则仅影响之后的新订单。品牌与彩种限额/取消开关可通过管理 API 配置，超级管理员只读。S5-a2 已接入用户目录、真实期次/规则、三种号码模型及四种选号模式、服务器预览和单独确认、注单及取消退款页面。确认携带服务器签发的账户上下文，切换会员不能复用旧确认；网络结果不确定时保留原请求键。

S5-a3 接入后台品牌/彩种策略、注单分页与快照查询，以及人工异常标记。异常证据不可改写；标记不退款，运营可另外取消并按原来源退款，超级管理员仍只读。S5-a4 接入整期投注取消/判定取消及可恢复的逐笔原来源退款，失败任务需要运营明确重试，全部目标完成才显示退款完成。S5-a5 接入单注判定取消与不可变判定证据，只影响该单且原路退款。S5-b 接入真实公开开奖结果、筛选/详情和已开始历史期次，取消期次明确标注结果不作有效判定。结算、派奖、提现和佣金仍待后续。这些子阶段不表示平台已可运营，500 次/秒仍未验收。

S6-a 新增后台提现规则及不可变历史，品牌范围/来源/审核模式/默认 N 与彩种 N 覆盖分开管理。配置不会创建申请或冻结积分。基数为占用前全部可用充值＋赠送余额；所有来源的有效投注除以投注时N快照后精确累加，N必须大于0，修改只影响新投注。0043保存私有不可变N快照，0044修复恢复查找路径。平台已配置真实TurnoverChecker，新增当前会员只读资格预览并在申请时重新检查；品牌初始提现政策仍关闭，需管理员明确启用。双端已验证实际投注/派奖后申请、原请求重放、混合来源取消退款和成功周期截止，第一期仅处理内部积分，不接银行或虚拟币出款；生产容量及客户人工审核另行验收。

S5-c1 接入真实核算预览，S5-c2 接入整期结算、异常分类与中奖积分账本；品牌初始派奖模式 null，须显式选择 automatic/manual 并启动，manual 需运营批准。S5-c3 接入开奖更正、全额反向旧 prize、结算代次与重新结算；原规则/扣款、结果/核算/账本保留，先全部冲正再发布新结果，异常/已取消不复活。已使用/冻结的中奖 available 不足时安全停止，仅人工恢复，不扣其他来源或自动产生欠款。无真实支付；生产默认模式、佣金/奖励/报表回溯及 500 次/s 验收仍待后续，不代表可生产上线。

S6-c 扩展站内通知为实际正额派奖和奖金全额冲正的历史事实，延迟消费按不可变账本/目标验证，不以当前注单状态覆盖旧消息；后台通知铃铛提供真实品牌投递查询及带原因、幂等和审计的失败重试。无外部发送；提现流水口径仍需确认。

S7-l 运营可编辑八类站内通知的中英文纯文本模板，独立版本、权限、原因、确认、审计和历史；消息落库时复制当前内容，既有消息不被后续改版覆盖。旧v1仍用原文案，派奖/冲正保留不可编辑历史事实说明；发布配置不生成新事件或修改积分。后台未知结果可离开/返回按原正文与键重试。升级先migrate至0037、重启API/worker，数据库须为UTF8编码；不清空旧通知、任务或资金。详见 [通知模板合同](docs/12-notification-templates.md)。

S6-d 管理报表接入原注单时间区间的最终当前代次统计、实际账本入账/冲正与当前桶汇总，替换假趋势和假对账成功。聚合积分为精确字符串，可超过单账户int64；品牌日时区、独立快照、筛选、权限和审计明确。此处不计算提现资格/佣金，不生成不可变日月结或全链对账结论。

S7-k 对上述两种报表提供完整筛选范围CSV，最多10,000分组/4 MiB，超限拒绝而非截断；读取和导出分别授权，审计提交后才输出文件。管理端按已提交条件下载并验证摘要、范围和总计，积分列需按文本导入表格软件以保留精度。升级先migrate至0036再重启API/worker；仅服务器引导角色自动补权限。格式与接入见 [CSV导出合同](docs/11-report-csv-exports.md)，不表示日月结、提现/佣金或生产交付已完成。

S6-e 代理配置接入真实树、政策/节点历史、父子比例与深度约束、后台操作和用户直属下级设置。默认代理管理disabled/比值上限0；启用配置不启用佣金计算或支付。加入代码/晋升/旧注单归属、实际佣金分配和周月结仍待后续，配置读写不改变钱包。

S6-f 加入码接入后台创建/停用/有效期和审计、代理码/推荐码首次加入、用户 `/invites` 自身编码与原加入来源。会员归属和新注单提交时代理配置由数据库保存不可改写快照，历史缺失仅标legacy；编码停用不重分配已有会员，不自动晋升代理或发奖励。真实佣金/奖励政策、分配、周期与派发仍待后续；保密材料不进入仓库。

用户 `/notifications` 为按品牌会员隔离的真实站内收件箱，业务outbox由worker去重落库，单条/本页已读可原请求重试；中英快照不包含后台人员/理由或证明。管理端可查投递、带审计重试失败项及维护模板，超管只读。真实派奖/冲正及六种提现历史状态已接入；开奖受众及外部渠道仍未接入，不发送邮件/短信/Telegram。请保持API与worker指向同一主库。

升级至 S6-b 前先执行 migrate（0018–0020），再重启 API/worker。旧的服务器引导品牌角色补充通知查询/重试权限，平台超级管理员引导角色仅补查询；普通自定义角色不自动扩权。首次加入只从新业务事务生成欢迎事件，不为历史会员伪造注册；已有未消费的真实注单 outbox 会纳入站内队列。

单元和前端检查：

~~~sh
cd backend
go vet ./...
go test ./...
cd ..
pnpm typecheck
pnpm test
pnpm build
~~~

集成测试必须提供独立 PostgreSQL：

~~~sh
TEST_DATABASE_URL=postgres://lottery:lottery_local@localhost:5432/lottery_test?sslmode=disable go test -count=1 ./...
~~~

从 backend 目录运行上述命令。测试在目标数据库中创建独立随机 schema，结束后删除该 schema；禁止使用生产数据库。未配置 TEST_DATABASE_URL 时集成测试会显示 SKIP，不能据此宣称数据库验收通过。具备 C 编译工具链的环境还应运行 go test -race ./...；CI 使用 Linux 完成该检查。

浏览器回归：每个Playwright视口及分片必须使用独立的临时测试PostgreSQL与API/worker，分别迁移、seed并创建测试管理员；不要指向开发共用库或生产环境。先安装浏览器：

~~~sh
pnpm exec playwright install chromium
~~~

GitHub Actions 的五组浏览器任务统一使用 `node scripts/prepare-ci-browser.mjs`：先下载锁定版本的 Chromium，并实际启动、读取测试页面；只有明确缺少系统库时才补装依赖。下载最多180秒，补装由 root timeout 限制180秒并在10秒后强制终止，整个步骤最多7分钟。下载失败、其他启动错误、补装失败或再次探测失败均使任务失败，不跳过场景、不提高业务用例重试次数。脚本仅允许在 GitHub Actions Linux 运行；开发机沿用上面的安装方式，不自动修改系统软件包。

佣金专项夹具在初始化和后续命令前只读检查程序内置的完整迁移集合及每份校验和，不依赖写死的最新文件名。旧库、缺失历史、未知迁移或校验和损坏都会拒绝；校验不自动迁移、不修补元数据，也不放宽独立库白名单或非空身份保护。

启动 desktop 专属测试 API 后运行：

~~~sh
pnpm test:e2e --project=desktop --shard=1/2 --workers=1 --retries=0
~~~

浏览器回归还需启动 `go run ./cmd/platform worker`，并确保测试 API 与 worker 指向该 project 的同一个临时测试库。

连续重跑也应使用新的临时测试库，或等待真实限流窗口结束；不要关闭限流、清理限流记录或复用客户/生产数据来让测试通过。

再切换到 mobile 专属的另一套临时数据库/API 后运行（本地顺序运行，避免前端/API 端口冲突）：

~~~sh
pnpm test:e2e --project=mobile --shard=1/2 --workers=1 --retries=0
~~~

上述命令分别运行各视口的第1分片；还须运行 `--shard=2/2` 才覆盖全部普通浏览器场景。每个视口、每个分片都需要独立的临时测试数据库与API/worker，不能只换project或shard参数后复用同一库。真实认证限流为每IP 30次、每账号10次/5分钟；同库合跑和失败后反复登录可能触发429。不得关闭认证限流、清除限流记录或伪造forwarded IP绕过。CI使用 `project: [desktop, mobile]` 和 `shard: [1, 2]` 矩阵，每个job自带独立PostgreSQL/API，执行 `pnpm test:e2e --project=${{ matrix.project }} --shard=${{ matrix.shard }}/2 --workers=1 --retries=0`；同一品牌配置写入场景在每个分片内串行执行。首轮失败必须修复并使用新测试库复验，不能据自动重试后的绿色结果宣称首轮稳定。

测试会自动启动两个前端，验证 PC/移动视口、真实注册/登录/会话恢复、品牌隔离、规则创建→验证→送审→独立审核→立即生效及原型选号/取消流程。每套临时测试库都需配置以下凭证：

- `TEST_ADMIN_USERNAME` / `TEST_ADMIN_PASSWORD`：事先由 `create-admin --brand aurora` 创建的隔离测试管理员，供后台成员、角色/账号和财务回归使用。
- `TEST_HARBOR_ADMIN_USERNAME` / `TEST_HARBOR_ADMIN_PASSWORD`：由 `create-admin --brand harbor` 创建，供 Harbor 配置与规则创建等回归使用。
- `TEST_RULE_REVIEWER_USERNAME` / `TEST_RULE_REVIEWER_PASSWORD`：另用 `create-admin --brand harbor` 创建的独立审核账号，必须不同于 Harbor 规则创建者，不能用同账号完成创建和审核。
- `TEST_PLATFORM_ADMIN_USERNAME` / `TEST_PLATFORM_ADMIN_PASSWORD`：由 `create-admin --super` 创建的独立平台测试账号，供品牌创建回归使用。
- `TEST_COMPLIANCE_ADMIN_USERNAME` / `TEST_COMPLIANCE_ADMIN_PASSWORD`：由 `create-admin --brand harbor` 创建的独立管理员，供合规配置回归使用。
- `TEST_EXPORT_ADMIN_USERNAME` / `TEST_EXPORT_ADMIN_PASSWORD` 和 `TEST_EXPORT_PLATFORM_USERNAME` / `TEST_EXPORT_PLATFORM_PASSWORD`：分别独立创建Harbor与平台管理员，供导出权限和隔离回归使用。
- `TEST_TEMPLATE_ADMIN_USERNAME` / `TEST_TEMPLATE_ADMIN_PASSWORD`：由 `create-admin --brand harbor` 创建的独立管理员，供模板发布及历史消息回归使用。

例如在 backend 目录、连接当前 project 的临时测试库并安全注入 `BOOTSTRAP_ADMIN_PASSWORD` 后创建审核账号（密码不写入命令参数）：

~~~sh
go run ./cmd/platform create-admin --username "$TEST_RULE_REVIEWER_USERNAME" --brand harbor
~~~

将该账号密码配置为 `TEST_RULE_REVIEWER_PASSWORD`，完成引导后清除引导密码环境变量。Harbor 桌面认证配置用例开启验证码后恢复原值，移动用例检查真实编辑器读取与布局；项目隔离仍不可省略。未提供凭证时对应组明确 SKIP，不能宣称全部验收通过。不要使用客户或生产凭证。已有 Chrome 可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指定可执行文件。浏览器会经真实公开 API 写入合成测试成员、角色/账号、配置、规则版本和审核记录，部分财务用例也会写真实测试账本；所有写入仅允许落到当前 project 的临时测试库，不是生产业务。原型演示订单本身不操作账本。

## 开发与测试方式

采用混合流程：核心业务 TDD（先写失败的边界与不变量测试，再实现）；UI 按页面原型迭代，补充状态/契约与浏览器回归。新页面先跑独立短链路冒烟，通过后集中阶段全量；复用合法测试会话，不关闭真实限次/权限校验。每阶段执行类型检查、单元、真实数据库、构建及相关端到端验证。积分并发、幂等、原路退款、规则审核、结算与纠正必须有自动化证据，不能仅凭页面提示验收。

开发数据库、测试数据库和生产数据库分离。开发种子只提供虚构品牌，不创建默认管理员密码；不开启真实支付。性能目标另做真实业务压力测试，页面原型和健康接口的性能不能代替投注吞吐验收。

## 工程结构

~~~text
backend/cmd/platform         服务、迁移、种子入口
backend/internal            领域和基础设施模块
backend/migrations          不可变迁移 SQL
user-web                    用户端 Vue PWA
admin-web                   管理端 Vue PWA
shared                      API、积分类型、品牌主题
docs                        实现规格和验收记录
.github/workflows           自动化检查
~~~

主库处理关键读写，从库通过 DATABASE_READ_URLS 显式配置。配置的从库无法提供合格历史读取时明确报错，不回退主库；未配置从库时使用主库。域名品牌解析使用主库，避免暂停和配置更新因复制延迟失效。未实现模块须以阶段记录为准。
