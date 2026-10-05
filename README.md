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

测试会自动启动两个前端，验证 PC/移动视口、真实品牌上下文、品牌隔离与原型选号/取消流程。已有 Chrome 可通过 `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` 指定可执行文件。CI 使用独立数据库运行这些测试；浏览器测试中的演示订单不会操作真实账本。

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
