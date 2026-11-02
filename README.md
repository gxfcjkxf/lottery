# Lottery 平台

> 当前投注与结算统一走[配置化链路](docs/65-unified-configured-betting.md)。

多品牌彩票运营平台，尚未发布。用户端、代理端、品牌后台与总后台是四个独立 Vue 项目；Go 后端的 API 和 worker 分别运行。第一期仅处理内部整数积分，不接银行、虚拟资产钱包或链上转账，不上架应用市场。

开发遵循 [AGENTS.md](AGENTS.md)：不写旧版本兼容或 fallback，保持 KISS。保留业务账本、审计、资金幂等和品牌隔离。投注与结算的当前实现以[65号交接](docs/65-unified-configured-betting.md)为准，早期旧引擎记录仅用于追溯。

## 当前业务与交接

当前包含全局身份及品牌会员、角色权限、四来源账本、人工充值、配置化投注与结算、共享期次及自动开奖、内部提现、代理佣金、人工奖励、站内通知和报表。共享彩种支持可配置 JSON、HTML 和执行 JavaScript 的 DOM 开奖来源。

所有模型统一由 `ConfiguredEngine` 报价、下单与结算，只写入 `configured_orders`；用户使用 `/configured/:gameId`，后台统一查询配置化订单。旧品牌自建投注、独立六合彩／加拿大投注及结算任务执行链路已移除，不转换旧试用注单。共享开奖结果发布后不可改写。配置化订单冻结购买时的规则、价格、销售版本及资金分配；更新规则不影响已购买订单。品牌在“彩种管理 → 销售配置”明确保存价格、投注开关及限额后才能销售；首次保存使用版本0，不自动启用。规则发布后才能开期；自动与手动期次共用窗口，品牌不另设自动开奖开关。自动开奖仅执行当前映射来源。现有开发库来源已明确重新配置，实际来源范围及闭环验证见[开发库来源记录](docs/65-unified-configured-betting.md#开发库自动来源映射配置与闭环验证2026-10-28)，其中加拿大来源仍为本地模拟数据。

借记投注按充值→中奖→佣金→赠送扣除；风险预留投注冻结最大亏损，结算只消费实际亏损，退款或释放按原来源归还。账户锁、期次限额锁和固定操作键保护并发扣款与重复派彩。提现按占用前可用充值＋赠送余额与投注时流水倍数快照判断，成功后切换流水周期。佣金、汇总及报表均读取配置化订单的真实财务事实。

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

### 早期界面精简记录（以64／65号当前交接为准）

以下为阶段记录；其中维护工具、旧注单查询及旧结算流程的后续移除见[64号](docs/64-betting-points-simplification.md)和[65号](docs/65-unified-configured-betting.md)。

#### 一期界面精简（2026-10-27）

业务报表仅保留投注、账本、提现、奖励及佣金入账五个页签，代理与加入来源筛选并入投注报表。周期核对不再提供独立页面。总后台余额修复、批量对账移到有权限账号的“维护工具”；品牌端已无入口的维护、归档、投递记录页面及其专用客户端移除，服务端接口、账本、审计和归档数据保留。独立资金/运营政策只读页合并到原有设置入口，通知模板仅编辑当前内容，不自动读取修订历史。实时 CSV 由服务端负责构造和业务完整性校验，前端只检查下载响应、类型、品牌范围和大小；不可变归档下载验证不变。测试按改动风险选择，资金幂等、余额与品牌隔离的必要回归仍保留。

#### 第二批界面精简（2026-10-27）

品牌展示和平台通知模板直接保存，失败保留草稿；结果未确认或版本冲突时先读取最新配置，再由用户保存，不保存跨页面待恢复意图。资金、认证、域名及安全政策的保护不随之删除。实时 JSON 报表由服务端计算业务汇总，浏览器保留格式、精度、品牌、查询范围与分页校验，不重复验算金额和分组等式；归档文件校验不变。用户常用投注积分改为手动保存。品牌安全政策保留开关与业务拒绝行为，移除无真实服务的手动伪检查工具。规则管理仅保留现有配置工作台，移除独立模板模拟器；运营工作台佣金区只展示待审批、暂停和失败事项。平台归档配置与通知投递记录从账户菜单的维护工具进入；归档查询与通知模板仍保留日常入口。代理端从会员进入注单，移除重复菜单。会员推荐及自定义玩法业务范围未调整。

#### 第三批界面精简（2026-10-28）

用户钱包及账本移除独立五秒轮询，钱包通过已验证的同会员钱包快照更新，账本仅在进入、翻页和主动刷新时读取；顶部积分保留可见页面的低频更新，概览统计复用钱包更新事件。代理佣金翻页只读取记录，进入或主动刷新时分别读取余额和记录，其中一项失败不清除另一项。总后台提现、奖励详情先读取订单，操作记录按需读取并独立重试，切换品牌、关闭详情或卸载后忽略旧请求。品牌后台代理、域名及合规页面直接存储 i18n 消息，不再通过原文反查翻译键。用户流水详情保留业务原因、变化积分、历史操作后可用积分及已知关联注单，完整四来源/四状态快照仍由接口及后台保留。批量删除会员、推荐体系和自定义玩法不在本批调整范围。

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
cd ..
```

seed提供Aurora/Harbor虚构品牌及localhost/harbor.localhost域名，在production拒绝运行。`APP_ENV=development`时还创建总管理员`admin / admin123`；test不创建默认管理员。重复seed不重置已有同名账号的密码、状态或权限。AUTH_KEY_FILE相对backend工作目录解析；密钥0600且不入Git，多实例共享，不能丢失或直接替换。

日常本地开发统一从仓库根目录使用以下入口管理 API 和 worker，禁止重启时手工摘取部分环境变量。配置文件路径必须显式指定；已有开发环境使用其原配置文件（当前本机为 `.local/review.env`），保留数据库、端口与认证密钥。

```sh
pnpm dev:backend start --env-file .env
pnpm dev:backend restart --env-file .env
pnpm dev:backend status --env-file .env
pnpm dev:backend check --env-file .env
pnpm dev:backend stop --env-file .env
```

入口完整加载同一配置文件，先编译新程序、检查迁移和实际读取来源，通过后才停止旧服务；启动后检查 API 就绪并再次实际读取来源，失败明确退出并停止本次新服务，不报告启动成功。`check` 只编译并读取核验，不发布开奖结果或执行结算。来源读取成功仅代表地址、映射和响应可用，不能代替缺期补采。按期查询的源须已有到开奖时间的期次才能核验。

首次将既有本地进程交给统一入口时，使用 `restart --env-file <原配置文件> --adopt-pids <原PID文件>`；入口核对进程身份及数据库、认证密钥、端口、环境一致后接管。之后 PID 和日志统一位于 `.local/dev-backend/`，不再使用各任务目录的启动脚本。

直接执行 `platform worker` 同样在启动前校验已启用来源及仍承担未开奖期次的映射来源版本。缺失白名单、源域名未授权、映射无效或密钥未配置均拒绝启动；没有采集来源的空安装允许白名单为空。部署继续通过 systemd 的同一个 `EnvironmentFile` 加载配置。`platform check-draw-sources` 提供独立的只读来源核验命令。

采集失败保留现有状态分类：请求或配置失败为 `source_error`，密钥缺失为 `credential_missing`，响应或期号/时间/号码异常为 `invalid_result`。结构化日志额外记录 `source_denied`、`network_error`、`http_status_<状态码>`、`response_invalid` 等具体原因；不记录密钥或响应正文。

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

提现初始关闭，须明确设置；佣金派发/更正开关默认关闭。保存政策不等于开启派发，页面提示不等于资金已入账。

## 检查与测试

采用核心业务TDD、UI迭代与契约/浏览器回归。先配置明确命名的独立测试 PostgreSQL 的 TEST_DATABASE_URL，不得指向共享开发库或生产；测试建立并清理自有随机schema，不清空整个数据库。缺失配置时数据库测试会SKIP，不能宣称验收通过。

```sh
go -C backend vet ./...
bash scripts/test-go-regression.sh
pnpm typecheck
pnpm test
pnpm build
pnpm api:check
pnpm test:contracts
```

全量 Go 回归的独立 PostgreSQL 测试实例须与 CI 一致配置 `max_locks_per_transaction=256`，为并行创建和清理完整基线 schema 提供锁容量。该实例仅用于测试，不复用正在联调的共享开发实例。

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


设置独立WebKit可执行路径和受保护测试凭据后，`pnpm test:webkit`运行真实账号、后台权限及风控双视口功能专项；服务工作线程明确禁用，不将其当作Safari/iPhone真机或PWA验证。安装、环境及范围见[WebKit交接](docs/46-webkit-functional-verification.md)。

`pnpm test:responsive`在768px/1024px复用真实注册与完整投注浏览器流程，要求独立合成库的正式API/worker、不同的Harbor创建/审核账号，以及明确的Chromium路径；环境变量同投注测试（TEST_HARBOR_ADMIN_USERNAME/PASSWORD、TEST_RULE_REVIEWER_USERNAME/PASSWORD、PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH）。串行、零重试，不复用已占用的5173/5174/5175前端服务。它不覆盖这两种尺寸的完整提现/佣金或真机安装。



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


2026-10-27 投注与积分一期精简：结算预览改即时只读试算；移除批量对账／在线修复和奖励待追缴，保留资金幂等、来源分配、品牌隔离及审计。见[当前范围](docs/64-betting-points-simplification.md)。

旧通用／专用投注的容量与提现／佣金浏览器夹具已随旧执行器退役。当前真实资金回归使用独立测试数据库运行 `go test -p 1 -parallel 4 ./internal/betting`，覆盖统一配置链路；不把旧容量数字用于当前版本。
