# Lottery 平台

多品牌彩票运营平台。实现依据位于 [docs/README.md](docs/README.md)，阶段验收与当前覆盖范围位于 [docs/implementation-progress.md](docs/implementation-progress.md)。

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

共享密码重置会影响全部品牌。按已确认规则，操作者须对该用户每个已加入品牌都有重置权限；仅有一个品牌权限时不得修改多品牌用户的全局密码。超级管理员始终不能修改用户。

管理端账号/角色、运营新增成员、认证配置、积分账本和人工充值/冻结/调整已接入真实 API。角色按品牌隔离，授予权限不得超出操作者的目标品牌权限；账号角色/状态/密码变更撤销目标会话。运营新增成员必须由本人首次登录确认条款。用户钱包与流水展示真实余额；后台也已接入独立玩法模拟 API：`POST /api/v1/admin/rule-simulations`，需显式品牌上下文和 `rule.simulate.brand` 或 `rule.simulate.platform` 权限。它只计算并审计，不投注、不改积分、不创建订单，也不发布或审批玩法。

S4-b 已接入彩种/玩法创建、规则草稿、持久化用例验证、送审、独立审核及旧定义克隆。目录读取需 `game.view.brand` 或 `game.view.platform`、创建需 `game.write.brand`；规则读取需 `rule.view.brand` 或 `rule.view.platform`，写入、验证、送审、审核分别需 `rule.write.brand`、`rule.validate.brand`、`rule.submit.brand`、`rule.review.brand`。这些工作流的全部超级管理员写入均拒绝；创建者及所有草稿编辑者不能审核。验证报告保留完整输入、输出、定义 hash 和警告，批准须确认警告；`version` 乐观锁与历史 `version_no` 不同。立即模式审核通过即生效，下期模式每玩法仅一个待生效版本，由内部实际开期事务激活。S4-c1 已接入不可变日历修订、期次生成与可选调度 worker；旧定义克隆仅支持 active/expired/rolled_back，新草稿定义不可改写且须重新验证审核。后台六个模板不是完整 DSL 编辑器；版本工作流不动积分。用户投注/提现与业务订单仍待后续接入；详细边界见 [玩法规则引擎](docs/03-rule-engine.md)、[API 契约](docs/04-api-contract.md)，阶段验收见 [实施记录](docs/implementation-progress.md)。

后台新建规则草稿默认选择立即生效（`immediate`）；创建、更新及克隆 API 仍必须显式发送 `effect_mode`，后端没有默认值。

Telegram 使用当前 OIDC 登录与一次性 nonce；品牌 `auth_config` 中设置 `telegram_enabled` 和 `telegram_client_id` 后，还需在 Telegram 配置允许的域名。未提供真实应用配置时默认关闭，不能用任意 Telegram 用户名冒充授权。验证码通过 `auth_config.captcha_enabled` 开启，是基本图形挑战，并不替代 WAF 和反自动化服务。

迁移执行器在事务中获取 advisory lock，并保存 SQL checksum；已应用迁移不可修改。重复执行 migrate/seed 是幂等的。服务启动不自动迁移，部署时应先执行迁移命令。

## 测试

S4-c2 已接入来源配置的不可变修订、采集尝试历史、人工补录以及结算前人工覆盖外部结果。后台“期次和开奖”页面使用真实管理 API，人工结果优先且不会被旧在途采集覆盖。API/DOM 适配器仍为无网络伪实现；worker 记录 no_data，不抓取真实第三方数据。该阶段不执行注单结算、派奖或支付。

S4-d 已提供通用运营编辑器，与六个快捷模板并存：自定义彩种模型、全部 schema-v1 条件、多个奖级、属性/特征、限额和舍入，以及最多 32 组独立验证用例。非模板定义不再只读，普通草稿可编辑，回滚来源草稿仍锁定。验证和品牌独立审核仍走原真实 API，不绕过审批，也不自动生成商业赔率安全结论。

S5-a1 新增真实投注/取消核心与 API：预览规范选号和展开复式，确认按充值→中奖→赠送扣分，订单/账本/审计/outbox 同事务，取消全额原路退款。每单保存当时规则/策略，立即批准的新规则仅影响之后的新订单。品牌与彩种限额/取消开关可通过管理 API 配置，超级管理员只读。当前用户端投注仍为明确标记的原型，尚未接入这些 API；用户目录、结算、派奖、提现、佣金和整期退款仍待后续。这一子阶段不表示平台已可运营，500 次/秒仍未验收。

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

浏览器回归：每个 Playwright project 必须使用各自独立的临时测试 PostgreSQL 与 API，分别迁移、seed 并创建测试管理员；不要指向开发共用库或生产环境。先安装浏览器：

~~~sh
pnpm exec playwright install chromium
~~~

启动 desktop 专属测试 API 后运行：

~~~sh
pnpm test:e2e --project=desktop --workers=2
~~~

浏览器回归还需启动 `go run ./cmd/platform worker`，并确保测试 API 与 worker 指向该 project 的同一个临时测试库。

连续重跑也应使用新的临时测试库，或等待真实限流窗口结束；不要关闭限流、清理限流记录或复用客户/生产数据来让测试通过。

再切换到 mobile 专属的另一套临时数据库/API 后运行（本地顺序运行，避免前端/API 端口冲突）：

~~~sh
pnpm test:e2e --project=mobile --workers=2
~~~

例如本地可分别使用 PostgreSQL 55440（desktop）与 55441（mobile），各自的 API 必须实际连接对应测试库，不能只换浏览器 project。不要在 5 分钟内让两个 project 共用同一数据库/API：真实认证限流为每 IP 30 次/5 分钟，合跑会触发 429。不得通过关闭认证限流、清除限流记录或伪造 forwarded IP 绕过；CI 使用 `project: [desktop, mobile]` 矩阵，每个 job 自带独立 PostgreSQL/API，执行 `pnpm test:e2e --project=${{ matrix.project }} --workers=2`。

测试会自动启动两个前端，验证 PC/移动视口、真实注册/登录/会话恢复、品牌隔离、规则创建→验证→送审→独立审核→立即生效及原型选号/取消流程。每套临时测试库都需配置以下凭证：

- `TEST_ADMIN_USERNAME` / `TEST_ADMIN_PASSWORD`：事先由 `create-admin --brand aurora` 创建的隔离测试管理员，供后台成员、角色/账号和财务回归使用。
- `TEST_HARBOR_ADMIN_USERNAME` / `TEST_HARBOR_ADMIN_PASSWORD`：由 `create-admin --brand harbor` 创建，供 Harbor 配置与规则创建等回归使用。
- `TEST_RULE_REVIEWER_USERNAME` / `TEST_RULE_REVIEWER_PASSWORD`：另用 `create-admin --brand harbor` 创建的独立审核账号，必须不同于 Harbor 规则创建者，不能用同账号完成创建和审核。

例如在 backend 目录、连接当前 project 的临时测试库并安全注入 `BOOTSTRAP_ADMIN_PASSWORD` 后创建审核账号（密码不写入命令参数）：

~~~sh
go run ./cmd/platform create-admin --username "$TEST_RULE_REVIEWER_USERNAME" --brand harbor
~~~

将该账号密码配置为 `TEST_RULE_REVIEWER_PASSWORD`，完成引导后清除引导密码环境变量。Harbor 桌面认证配置用例开启验证码后恢复原值，移动用例检查真实编辑器读取与布局；项目隔离仍不可省略。未提供凭证时对应组明确 SKIP，不能宣称全部验收通过。不要使用客户或生产凭证。已有 Chrome 可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指定可执行文件。浏览器会经真实公开 API 写入合成测试成员、角色/账号、配置、规则版本和审核记录，部分财务用例也会写真实测试账本；所有写入仅允许落到当前 project 的临时测试库，不是生产业务。原型演示订单本身不操作账本。

## 开发与测试方式

采用混合流程：核心业务 TDD（先写失败的边界与不变量测试，再实现）；UI 按页面原型迭代，补充组件/浏览器回归。每个阶段执行类型检查、单元测试、真实数据库集成测试和构建，涉及业务链路时增加端到端测试。积分并发、幂等、原路退款、规则审核、结算与纠正必须有自动化测试，不能仅凭页面提示验收。

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

主库处理关键读写，从库连接通过 DATABASE_READ_URL 单独配置。域名品牌解析使用主库，避免暂停/配置更新因复制延迟失效。未实现模块须以阶段记录为准，原型演示不代表业务已持久化。
