# Lottery 平台

多品牌彩票运营平台，尚未发布。用户端、品牌后台与总后台是三个独立 Vue 项目；Go 后端的 API 和 worker 分别运行。第一期仅处理内部整数积分，不接银行、虚拟资产钱包或链上转账，不上架应用市场。

开发遵循 [AGENTS.md](AGENTS.md)：不写旧版本兼容或 fallback，保持 KISS。业务账本、结果更正、审计和不可变归档版本必须保留，它们不是版本兼容。全仓删除项与保留依赖已记录在[清理收尾清单](docs/59-configurable-mark-six-rule-design.md#1516-全仓清理收尾与明确保留项2026-10-20)。

## 当前业务与交接

当前包含全局身份及品牌会员、角色权限、四来源账本、人工充值、投注/取消、玩法审核、期次、人工开奖、结算/更正、内部提现、代理佣金、人工奖励、站内通知、报表及对账。共享彩种已支持可配置 JSON、HTML 和执行 JavaScript 的 DOM 开奖来源；普通品牌彩种 API/DOM 来源及身份检查仍有一期伪适配器，不能视为真实验证。

积分按充值→中奖→佣金→赠送扣除，取消原路退款。同品牌同彩种上期已结算或取消全部退款后才能开放下期，错过窗口直接跳过。提现按占用前全部可用充值＋赠送余额与投注时 N 快照判断，成功后按申请提交时间切换流水周期。首页、钱包和订单使用真实接口，没有本地假订单或示例余额。

配置式规则经编辑、验证、审核和发布后，由品牌明确保存销售价格、开关及限额；不再提供旧规则/赔率转换导入或首次发布时自动搬运旧销售配置。六合彩和加拿大彩的新投注统一使用 `/configured/:gameId` 与配置式 API；专用目录、报价、下单及旧规则/销售写入已移除。旧注单查询、原快照结算和资金记录继续保留。已有彩种需明确发布配置式规则并由品牌保存销售配置，不自动迁移旧销售。平台自动开奖通过“保存命名来源修订 → 明确启用”发布，旧平台 POST automation 直接发布入口已删除。新来源仅接受 `mapped` 字段映射；旧 `interval_json`、`hk_marksix_6hch` 不能保存或重新启用。已存旧版本继续执行原日程、采集及未结单收尾，不转换历史结果与资金，详见[旧开奖清理边界](docs/59-configurable-mark-six-rule-design.md#1515-旧开奖入口收敛与统一发布2026-10-20)。

以 [交接索引](docs/README.md)、[业务规格](docs/01-product-spec.md) 和 [验收及未决事项](docs/08-acceptance-and-open-items.md) 为依据。[实施记录](docs/implementation-progress.md)保存阶段证据，早期增量升级或未接入说明不能作为当前安装步骤。

| 范围 | 当前交接 |
| --- | --- |
| 三端入口、角色和登录边界 | [管理入口](docs/42-administration-entries.md) |
| 玩法和第三方接口 | [规则引擎](docs/03-rule-engine.md)、[已实现OpenAPI](docs/openapi.json)、[API交接](docs/09-openapi-handover.md) |
| 账本、退款与诊断 | [四来源账本](docs/15-four-source-ledger.md)、[对账](docs/13-wallet-reconciliation.md)、[品牌引用](docs/37-brand-business-inventory.md) |
| 佣金、奖励和更正 | [周期及派发](docs/17-commission-cycles.md)、[更正管理](docs/24-commission-correction-management.md)、[重新核算](docs/39-commission-manual-recalculation-policy.md)、[奖励](docs/20-manual-reward-orders.md) |
| 报表、通知、审计和PWA | [周期分析](docs/38-commission-cycle-analysis.md)、[归档](docs/26-report-archive-core.md)、[自动归档](docs/29-report-archive-activation.md)、[开奖通知](docs/31-draw-result-notifications.md)、[审计](docs/32-audit-query-and-export.md)、[PWA](docs/44-pwa-production-cache.md) |
| 并发和运维 | [容量基线](docs/43-current-capacity-baselines.md)、[复制恢复](docs/10-replication-and-recovery.md)、[可观测性](docs/33-operational-observability.md)、[发布手册](docs/34-release-and-operations.md) |

开发证据不等于整套最新CI、生产容量、真机或客户人工审核通过。合规市场、商业号码/赔率、生产时区/模式、Telegram真实应用、域名/TLS与灾备目标另行确认，不索取或提交客户保密牌照材料。

## 空库安装

需要 Go 1.26+、Node 24+、pnpm 11.19.0、PostgreSQL 17。当前业务实际用 PostgreSQL 保存会话、限流、幂等和持久任务；Redis、NATS与对象存储尚未接入，不是API/worker启动依赖。

数据库必须为 UTF8。正式版本发布前，`backend/migrations/` 统一维护一份 `0001_baseline.up.sql`，新空库执行 `migrate`。发布前的结构变更可合并到该基线；已有开发库须先备份，在副本验证结构与目标基线一致、业务数据完整后，显式同步迁移记录，不能直接重放建表 SQL，也不能仅改 checksum 绕过校验。正式版本发布后冻结已发布迁移及其 checksum，后续结构变更使用增量迁移。API 和 worker 启动始终核验完整迁移集合，不自动迁移或修复记录，不清空现有数据库。

写入型浏览器回归不得使用共享开发库。`playwright.review.config.ts` 要求 `APP_ENV=test`，数据库必须为 `lottery_admin_entries_desktop1440`、`lottery_admin_entries_mobilepixel7` 或明确新建的 `lottery_admin_entries_local_*`；在该库初始化匹配版本服务和独立测试账号后运行。规则自检另外创建专属临时品牌，结束或失败时通过正常 API 清理；不得将测试彩种写入日常使用的 Aurora 等品牌。

以下用于首次安装；已有.env或认证密钥时保留它们，不重复生成：

```sh
cp .env.example .env
docker compose up -d postgres
set -a
source .env
set +a
pnpm install --frozen-lockfile
cd backend
go run ./cmd/platform generate-auth-key .local/auth.key
go run ./cmd/platform migrate
go run ./cmd/platform seed
go run ./cmd/platform serve
```

seed提供Aurora/Harbor虚构品牌及localhost/harbor.localhost域名，在production拒绝运行。`APP_ENV=development`时还创建总管理员`admin / admin123`；test不创建默认管理员。重复seed不重置已有同名账号的密码、状态或权限。AUTH_KEY_FILE相对backend工作目录解析；密钥0600且不入Git，多实例共享，不能丢失或直接替换。

另一个终端从仓库根目录加载相同环境，启动独立worker：

```sh
set -a
source .env
set +a
cd backend
go run ./cmd/platform worker
```

分别在三个终端从仓库根目录运行 `pnpm dev:user`、`pnpm dev:admin`、`pnpm dev:platform`。用户端localhost:5173、品牌后台localhost:5174、总后台localhost:5175；API localhost:8080，存活/就绪分别为 /health/live、/health/ready。

使用现有 8085 后端时，总后台运行 `pnpm --filter @lottery/platform-web dev:review`，访问 `http://127.0.0.1:5185`。这是 Vite 开发模式，保存 Vue、TypeScript、CSS 后页面自动更新，不需要执行 build。`vite preview` 只提供上一次构建的 dist，不监听源码；不要用它进行日常页面开发。同一端口只能启动一个服务，切换模式前先停止旧进程。Go 后端改动仍需重新编译、重启，并通过数据库迁移一致性检查。

## 账号与安全

开发环境执行seed后，可用`admin / admin123`登录独立总后台；这不是品牌管理员，不能进入品牌后台。已存在admin时不会覆盖，请使用其已有密码。固定短密码仅在development总后台登录时接受，不放宽普通用户、账号创建或密码重置规则；不可用于公网或正式部署。

test/production不自动创建管理员。服务器拥有者安全注入BOOTSTRAP_ADMIN_PASSWORD（6–30字节），从backend显式创建，随后清除环境变量。实际部署密码不放入命令参数、日志或仓库：

```sh
go run ./cmd/platform create-admin --username operator --brand aurora
go run ./cmd/platform create-admin --username platform_reader --super
```

品牌后台使用 /api/v1/admin，总后台使用 /api/v1/platform，账号类型与Cookie隔离，未登录只显示登录入口。平台账号只查看品牌会员/业务，不修改用户、审核品牌玩法或执行品牌资金操作。

平台创建品牌是 `POST /api/v1/platform/brands`，需要brand.create.platform，不发送X-Brand-ID。新品牌固定暂停/v1，不生成会员、域名、游戏、资金或默认账号，不自动授予品牌写权限。配置真实平台入口后可引导空库平台账号，见管理入口交接。

用户用户名/手机号/密码全局共用，会员资料、资金与会话按品牌隔离。首次加入须接受该品牌条款，dev-1仅为开发占位文本。共享密码重置要求操作者对该用户全部已加入品牌有权限，平台账号不能重置用户密码。

浏览器使用HttpOnly、SameSite=Strict Cookie，不存访问令牌。生产必须HTTPS与Secure Cookie；用户品牌来自实际Host或 /api/v1/b/{brandCode}，不信任伪造品牌头或转发Host。写请求须JSON、Idempotency-Key及匹配Host的Origin。认证连接池至少4连接，限流每IP30次、每账号10次/5分钟，不是投注吞吐限制；不关闭限流或伪造IP让测试通过。

提现初始关闭、派奖模式未配置，须明确设置；佣金派发/更正开关默认关闭。保存政策不等于开启派发，页面提示不等于资金已入账。

## 检查与测试

采用核心业务TDD、UI迭代与契约/浏览器回归。先配置独立测试PostgreSQL的TEST_DATABASE_URL，不能指向生产；测试建立并清理自有随机schema，不清空整个数据库。缺失配置时数据库测试会SKIP，不能宣称验收通过。

```sh
go -C backend vet ./...
bash scripts/test-go-regression.sh
pnpm typecheck
pnpm test
pnpm build
pnpm api:check
pnpm test:contracts
```

本地全量Go脚本要求已设置 `TEST_DATABASE_URL`，完整运行 `./...`，开启race且禁用测试结果缓存；包间串行、包内最多4项并行，避免迁移锁与真实Argon2计算争用。每包20分钟与现有数据库CI一致，包含整包累计的迁移、准备及真实期次等待；不修改业务请求时限，也不筛掉慢测试。回归须对应固定源码；同一工作区有并行编辑时，从包含未提交改动的独立快照运行相同命令，并保存源码校验值。浏览器开奖、报表容量边界等显式构建标签仍按对应专项命令验收。

Go与Node须同时在PATH；API检查核对实际Go路由。OpenAPI仅列当前可调用接口，未来设计不代表已经可用。

### CI运行方式

日常push和pull_request运行四组检查：前端类型检查/单测/静态检查/构建、接口契约、后端vet/单测/构建，以及 Chromium/WebKit 多设备布局检查。后端快检不连接PostgreSQL、不启用race，数据库集成测试会跳过；快检绿色不代表资金、权限或浏览器全量验收通过。

阶段完成或准备交付时，在[Actions的check工作流](https://github.com/gxfcjkxf/lottery/actions/workflows/ci.yaml)点击Run workflow，选择待验收分支并运行。手动运行启用完整40项任务，保留真实数据库、race、资金流程、各尺寸浏览器、PWA/WebKit和报表边界检查。确认通过的提交SHA后再交付，不把旧提交的全量结果套给新提交。

同一ref的新push/PR会取消旧快检。手动全量使用独立并发组，不会被新快检取消。涉及积分、投注、提现、佣金或权限的改动，开发时仍运行相关真实数据库及并发测试；阶段交付前执行手动全量回归。没有定时全量任务，也不使用复杂的文件依赖分类。

普通浏览器回归需要独立临时库、API/worker和明确测试凭据。当前CI文件是完整矩阵与专项夹具的运行依据，两个视口的两个分片分别使用独立环境：

```sh
pnpm test:e2e --project=desktop --shard=1/2 --workers=1 --retries=0
pnpm test:e2e --project=desktop --shard=2/2 --workers=1 --retries=0
pnpm test:e2e --project=mobile --shard=1/2 --workers=1 --retries=0
pnpm test:e2e --project=mobile --shard=2/2 --workers=1 --retries=0
```

显式设置PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH；没有浏览器时先 `pnpm exec playwright install chromium`。测试自动启动三套前端，并真实写入合成身份、配置、订单及账本。提前引导如下独立账号，分别提供对应PASSWORD环境变量：

- TEST_ADMIN_USERNAME：Aurora；TEST_HARBOR_ADMIN_USERNAME：Harbor。
- TEST_RULE_REVIEWER_USERNAME：不同于Harbor创建者的审核账号。
- TEST_PLATFORM_ADMIN_USERNAME：平台账号。
- TEST_COMPLIANCE_ADMIN_USERNAME、TEST_EXPORT_ADMIN_USERNAME、TEST_TEMPLATE_ADMIN_USERNAME：各自独立Harbor账号；TEST_EXPORT_PLATFORM_USERNAME：独立平台导出账号。

提现、佣金、奖励、归档等专项使用各自受保护的合成库和夹具，按CI对应job配置，不用通用seed冒充专项资金。缺凭据导致SKIP或显式失败均不算验收。保留失败追踪，修复后使用新合成数据或等待限流窗口，不清除业务或限流记录。

`pnpm test:pwa`验证生产构建元数据、图标和公开哈希缓存，不连接API/数据库，不缓存HTML、私有API或写请求，不排队重放投注/提现，不等于真机安装或完整离线验收。

三模型复式/倍投受控压测的原始报告及复跑命令见[容量交接](docs/43-current-capacity-baselines.md)；`pnpm test:capacity-mixed-models REPORT.json`独立核对15000笔请求、各模型实际组合/倍数/金额及账本不变量，不将本机结果视为生产或主从验收。

设置独立WebKit可执行路径和受保护测试凭据后，`pnpm test:webkit`运行真实账号、后台权限及风控双视口功能专项；服务工作线程明确禁用，不将其当作Safari/iPhone真机或PWA验证。安装、环境及范围见[WebKit交接](docs/46-webkit-functional-verification.md)。

`pnpm test:responsive`在768px/1024px复用真实注册与完整投注浏览器流程，要求独立合成库的正式API/worker、不同的Harbor创建/审核账号，以及明确的Chromium路径；环境变量同投注测试（TEST_HARBOR_ADMIN_USERNAME/PASSWORD、TEST_RULE_REVIEWER_USERNAME/PASSWORD、PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH）。串行、零重试，不复用已占用的5173/5174/5175前端服务。它不覆盖这两种尺寸的完整提现/佣金或真机安装。

四尺寸提现专项使用`pnpm exec playwright test --config playwright.qualification.config.ts`。先在新的UTF8合成库按基线migrate/seed，并运行带browserfixture标签的qualification-fixture：它只允许APP_ENV=test、明确确认、含密码的本机lottery_test连接及lottery_withdrawal_qualification_s9数据库，要求身份和钱包为空。夹具创建四个独立用户，通过真实充值、投注和派奖准备资格。启动正式API/worker，设置TEST_QUAL_ADMIN_PASSWORD、TEST_QUAL_USER_PASSWORD及PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH后串行验证360/768/1024/1440px。不能在已使用的夹具库重复运行或清空原数据来重试；此流程不接入外部支付。

佣金中间尺寸使用`pnpm test:commission-responsive tablet768`或`laptop1024`。各自先初始化新的UTF8合成库lottery_commission_ui_tablet768_s16或lottery_commission_ui_laptop1024_s16，设置APP_ENV=test、含密码的本机lottery_test DATABASE_URL、COMMISSION_FIXTURE_CONFIRM=owned_synthetic_database、COMMISSION_FIXTURE_ADMIN_PASSWORD及COMMISSION_FIXTURE_USER_PASSWORD；带browserfixture标签构建的commission-fixture执行init已包含当前基线与seed。启动正式API，不启动常驻worker：测试显式推进真实任务。设置COMMISSION_FIXTURE_BIN为夹具绝对路径、TEST_COMMISSION_ADMIN_PASSWORD及PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH；顺序入口依次执行周期、派发、修正、报表、分析，任一步失败就停止。不要直接运行整份配置的字母排序测试或重用已完成的有状态夹具。

## 工程结构

```text
backend/cmd/platform     API、worker、迁移、seed及账号引导
backend/internal         领域服务和基础设施
backend/migrations       正式发布前的单一基线；发布后追加增量迁移
user-web                 用户端独立Vue PWA
admin-web                品牌后台独立Vue PWA
platform-web             总后台独立Vue PWA
shared                   共享类型、积分与品牌工具
tests                    契约、静态及浏览器回归
docs                     实现规格、交接与验收
.github/workflows        自动化检查
```

关键业务与授权走主库。从库通过DATABASE_READ_URLS显式配置，仅支持已实现的不可变历史白名单；节点不合格报错，不回退主库，未配置从库时正常使用主库。本地复制/恢复不等于自动故障切换、生产PITR或客户灾备验收。


### 移动端布局验证

移动端不固定为 `360×800`。`pnpm test:layout` 使用 Pixel / iPhone 设备预设视口、横竖屏和不同宽高，在 Chromium 与 WebKit 验证三端登录入口、用户端开奖/投注和弹窗，并覆盖中英文。首次运行先执行 `pnpm exec playwright install chromium webkit`。完整范围、业务回归与真机试用的区别见 [UI 规范第 6 节](docs/48-ui-architecture-and-design-system.md#6-移动端适配与布局验收)。

### 代理管理

运行 `pnpm dev:agent`，本地 DEV 入口为 `http://127.0.0.1:5186/?brand=aurora`；使用品牌后台直接创建的代理账号登录。所有层级共用此入口，会员账号与后台账号均不能用于代理端登录。默认连接本地后端 8085；独立测试可通过 `AGENT_API_URL` 显式指定测试 API。

代理管理的个人信息提供用户端注册链接、复制链接和二维码 PNG 保存，二维码也支持手机长按保存。它与用户端推荐链接共用展示组件和地址生成方法，代理使用 `agent_code`，推荐使用 `referral_code`。代理管理的 `VITE_USER_APP_ORIGIN` 必须指向所属品牌的用户端站点；开发配置 `agent-web/.env.development` 明确使用 `http://127.0.0.1:5183`。构建部署时显式设置该变量（参见 `agent-web/.env.example`），不根据代理端域名或端口猜测用户端地址。地址未配置或无效时显示不可生成提示，不提供错误链接。

代理端真实浏览器回归使用 `TEST_DATABASE_URL=postgres://.../lottery_agent_portal_test_local pnpm test:agent`，数据库名称必须含 `_test_` 且由本次测试独占。入口依次为每个设备创建独立 schema/API，运行中英文创建会员、下级代理、范围查询及修改密码后重新登录，结束或失败时由 Go 测试清理 schema。每个设备保留真实认证限流，不共用累计认证额度、不伪造 IP；测试账号均在自有临时品牌中。
