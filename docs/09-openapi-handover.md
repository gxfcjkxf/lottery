# 已实现接口与契约检查

第三方开发人员或 AI 接入现有服务时，使用 [openapi.json](openapi.json) 获取实际注册的接口、请求参数、响应模型和认证要求。[04-api-contract.md](04-api-contract.md) 保留完整业务设计，包含尚未实现的功能；不能据设计稿假定接口已经可调用。

## 当前范围

已实现接口包含健康检查、品牌上下文、用户认证、管理账号与权限、积分账本和人工充值、玩法与期次、投注、开奖、结算与更正、站内通知、报表、代理配置和加入码。用户标准路径与平台 `/b/{brandCode}` 路径分别列出，域名解析和品牌隔离仍由服务端执行。

品牌运行状态及展示配置也已接入真实接口。展示配置使用 `/api/v1/admin/brand-presentation` 的 GET/PUT，历史使用同路径 `/history` 的 GET；只以 `X-Brand-ID` 选择品牌，不接受品牌路径别名。读写分别要求明确的 `brand_presentation.view`/`write` 品牌或平台权限，超级管理员身份本身不绕过授权。历史按新到旧分页，版本可能因其他品牌配置变更而跳号。

PUT 必须提供当前共享版本、全部 16 个配置键和操作原因。可空覆盖项必须显式传 `null`；`content` 非空时必须包含 `en`、`zh-CN` 及各自的 `tagline`、`announcement`。写回执包含原版本加一、提交配置和审计 ID；缓存重放仍重新验证当前权限和停用状态。版本及停用冲突分别返回 `BRAND_PRESENTATION_VERSION_CONFLICT`、`BRAND_PRESENTATION_STATE_CONFLICT`，不能据缓存回执覆盖当前读取。预设、素材和语言范围见 [05-ui-spec.md](05-ui-spec.md)。

提现当前只有政策配置，没有申请、资格判定或出款接口。佣金与奖励实际发放、真实支付、真实外部开奖 API/DOM 适配器也不可调用。配置存在不代表财务流程已经实现。容量目标 500 次投注/秒不属于已验证能力。

## 接入约束

用户与管理员令牌不能互换。非浏览器客户端可以使用 Bearer；管理员浏览器 Cookie 名称为 `lottery_admin`，用户 Cookie 名称随品牌 UUID 变化，见安全方案的说明。生成客户端通常使用 Bearer；网页客户端继续使用既有 HttpOnly Cookie 流程，不把令牌存入本地存储。

管理接口按实际要求携带 `X-Brand-ID`；权限列表是说明，不替代服务端授权。`x-permissions` 中多个值是否为替代权限、以及超级管理员的特殊限制，以操作描述和业务权限规则为准。可信反向代理必须保留原始 Host，品牌路径不能覆盖独立品牌域名。

变更操作按照 `x-idempotent-operation` 和请求头定义发送幂等键。结果未知时，使用原键和完全相同的请求正文重试，不创建新意图。Cookie 请求还必须携带匹配 Host 的 Origin。成功回执代表原操作结果，不一定代表当前资源状态，应独立查询确认。

积分和财务合计使用十进制字符串。钱包与单笔积分由服务端限制在 int64 范围，积分变动允许负数，正常余额不允许负数；报表合计可能超过 int64。不得转换为 JavaScript Number。Schema 的字符串模式检查规范格式，数值上限、UTF-8 字节长度、重复 JSON 字段、规则语义和业务状态仍由 Go 校验。

## 修改与验证

从仓库根目录执行，要求已安装 Go、Node 和 pnpm：

```sh
pnpm install --frozen-lockfile
pnpm api:generate
pnpm api:check
pnpm test:contracts
```

Go 不在 PATH 时，可设置 `LOTTERY_GO_BIN` 为本机 Go 可执行文件的绝对路径。契约检查不需要数据库、认证密钥或登录凭据，不调用真实支付或第三方服务。

修改 `docs/openapi/identity.mjs`、`finance.mjs`、`lottery.mjs` 中对应的接口与模型，再运行生成命令；不要手工编辑生成的 `openapi.json`。片段结构见 `docs/openapi/fragment-format.json`。新增接口必须同时修改真实路由与契约，否则生成检查会失败。

`backend/cmd/route-inventory` 通过服务启动使用的同一组注册函数导出路由。检查拒绝遗漏接口、未注册接口、重复 operationId、重复参数、未解析引用及生成文件过期。CI 的 `api-contract` 任务执行同样的检查，避免交付时接口与文档脱节。

契约测试编译全部组件和操作的 JSON Schema，并使用 `backend/cmd/contract-examples` 的真实 Go DTO 序列化结果、真实规则引擎输出核验代表性模型。示例包含负数扣分、超过 int64 的报表合计、可空字段以及特别号复式倍投计算。该检查不等于全部状态或所有返回模型已经逐一验收；业务事务仍由 PostgreSQL 集成测试覆盖，生产上线仍需客户人工审核。

契约格式采用固定版本 [OpenAPI 3.1.1](https://spec.openapis.org/oas/v3.1.1.html)，格式检查使用 [Redocly lint](https://redocly.com/docs/cli/commands/lint)，数据模型检查使用 [Ajv 2020](https://ajv.js.org/json-schema.html)。工具安装版本由 `pnpm-lock.yaml` 固定。
