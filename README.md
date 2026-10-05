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

分别在另两个终端启动：

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

共享密码重置会影响全部品牌。当前安全保护要求操作者对该用户每个已加入品牌都有重置权限；仅有一个品牌权限时不得修改多品牌用户的全局密码。此规则仍待业务确认，不会提前放宽。超级管理员始终不能修改用户。

Telegram 使用当前 OIDC 登录与一次性 nonce；品牌 `auth_config` 中设置 `telegram_enabled` 和 `telegram_client_id` 后，还需在 Telegram 配置允许的域名。未提供真实应用配置时默认关闭，不能用任意 Telegram 用户名冒充授权。验证码通过 `auth_config.captcha_enabled` 开启，是基本图形挑战，并不替代 WAF 和反自动化服务。

迁移执行器在事务中获取 advisory lock，并保存 SQL checksum；已应用迁移不可修改。重复执行 migrate/seed 是幂等的。服务启动不自动迁移，部署时应先执行迁移命令。

## 测试

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

浏览器回归（先启动已迁移、已 seed 的开发 API）：

~~~sh
pnpm exec playwright install chromium
pnpm test:e2e
~~~

测试会自动启动两个前端，验证 PC/移动视口、真实注册/登录/会话恢复、品牌隔离与原型选号/取消流程。后台成员操作回归还要求 `TEST_ADMIN_USERNAME` 和 `TEST_ADMIN_PASSWORD`，指向事先由 `create-admin --brand aurora` 创建的隔离测试账号；未提供时该组明确 SKIP，不能宣称全部浏览器验收通过。不要使用客户或生产凭证。已有 Chrome 可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指定可执行文件。CI 在临时独立数据库中显式创建测试管理员；浏览器注册/状态修改会持久化到测试库，但演示订单不操作真实账本。

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
