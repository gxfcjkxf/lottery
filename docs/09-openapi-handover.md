# 已实现接口与契约检查

第三方开发人员或 AI 接入现有服务时，使用 [openapi.json](openapi.json) 获取实际注册的接口、请求参数、响应模型和认证要求。[04-api-contract.md](04-api-contract.md) 保留完整业务设计，包含尚未实现的功能；不能据设计稿假定接口已经可调用。

## 当前范围

已实现接口包含健康检查、品牌上下文、用户认证、管理账号与权限、积分账本和人工充值、玩法与期次、投注、开奖、结算与更正、站内通知、报表、代理配置和加入码。用户标准路径与平台 `/b/{brandCode}` 路径分别列出，域名解析和品牌隔离仍由服务端执行。

佣金政策、周期发现/核算、单人审核与实际佣金来源入账、独立人工差额修正及实际入账报表已接入。派发使用独立`/commission-payment-policy`与`/commission-payments`接口及金融写权限，核算ready不表示已派发。默认关闭的运行开关须品牌管理员显式启用，开启会处理已有历史就绪周期；人工/自动方式仍来自原投注快照，混合周期整期人工批准。已入账结果更正补偿仍未实现，不由独立人工修正解锁。当前契约覆盖260条实际操作和335个组件，不能据此宣称整个平台已完成。

人工奖励使用六条`/api/v1/admin/reward-orders`接口及独立奖励权限，三个写操作必须携带确认时的X-Reward-Actor-ID；平台账号只读。真实发放与全额原赠送来源撤销已接入，余额不足返回持久化待处理200，需运营处理后手动继续。双语后台已验证原键恢复与当前查询分离；没有自动奖励worker、奖励通知或独立奖励报表。字段、错误及原回执语义见[奖励合同](20-manual-reward-orders.md)。

平台品牌创建使用POST `/api/v1/admin/brands`，需 `brand.create.platform`，不发送 `X-Brand-ID`。请求恰好为code/name/default_locale/timezone/reason；201返回固定暂停/v1的不可变初始快照及audit_log_id。未知结果以原正文/键重试，不能把旧创建回执当成当前状态；无自动管理员、会员、域名、彩种或积分。可信平台Host在无公开品牌时仍能管理登录/恢复会话及创建，用户裸品牌上下文仍保持404。具体模型和错误见 [API合同](04-api-contract.md)。

品牌运行状态及展示配置也已接入真实接口。展示配置使用 `/api/v1/admin/brand-presentation` 的 GET/PUT，历史使用同路径 `/history` 的 GET；只以 `X-Brand-ID` 选择品牌，不接受品牌路径别名。读写分别要求明确的 `brand_presentation.view`/`write` 品牌或平台权限，超级管理员身份本身不绕过授权。历史按新到旧分页，版本可能因其他品牌配置变更而跳号。

PUT 必须提供当前共享版本、全部 16 个配置键和操作原因。可空覆盖项必须显式传 `null`；`content` 非空时必须包含 `en`、`zh-CN` 及各自的 `tagline`、`announcement`。写回执包含原版本加一、提交配置和审计 ID；缓存重放仍重新验证当前权限和停用状态。版本及停用冲突分别返回 `BRAND_PRESENTATION_VERSION_CONFLICT`、`BRAND_PRESENTATION_STATE_CONFLICT`，不能据缓存回执覆盖当前读取。预设、素材和语言范围见 [05-ui-spec.md](05-ui-spec.md)。

提现政策、正式流水资格查询、用户申请/查询和运营审核/内部积分处理接口已注册，具体字段、确认上下文与幂等语义见[API合同](04-api-contract.md)。正式资格使用实际投注时N快照与有效投注证据；旧无快照记录不能补造资格。mark-paid只记录内部积分成功，不执行法币或虚拟币转账。真实支付、真实外部开奖API/DOM适配器不可调用。500次投注/秒只有受控本地基线证据，不代表完整生产容量验收。

域名管理使用 `/api/v1/admin/brand-domains` 的GET/POST、`/{domainID}`的PATCH及`/history`的GET。创建要求完整 `version,domain,enabled,is_primary,reason`，更新只接受 `version,enabled,is_primary,reason`；主机名不可修改。写回执返回完整绑定列表、共享版本加一及审计ID；读取中的审计ID仅在当前共享版本来自域名变更时存在。读写要求显式的 `brand_domains.view/write.brand/platform` 权限，超级管理员身份不绕过授权。

错误包括 `BRAND_DOMAIN_INPUT_INVALID`、`BRAND_DOMAIN_NOT_FOUND`、`BRAND_DOMAIN_VERSION_CONFLICT`、`BRAND_DOMAIN_STATE_CONFLICT`及`BRAND_DOMAIN_CONFLICT`；禁用绑定继续占用全局主机名。请求的Host必须先属于可用管理入口，当前入口被关闭时可能先返回404，不据此认为旧成功回执可重放。域名绑定不验证所有权、不操作DNS/TLS/重定向；客户部署方需另行验证，详见 [05-ui-spec.md](05-ui-spec.md)。

## 接入约束

提现可查询独立授权的当前状态报表并完整导出CSV，工作台提供真实审核中/提现中计数；六种提现状态事件通过既有站内收件箱读取。报表按申请created_at筛选，不是实际出款时间或不可变结账；通知记录不可变历史状态，不代表当前余额或外部支付。字段、范围、文件校验与权限见[API合同](04-api-contract.md)，模板和历史说明见[通知合同](12-notification-templates.md)。查询报表或通知不授权提交提现，申请仍须通过当前配置、资格、余额及并发检查。

钱包批量对账使用GET/POST `/api/v1/admin/reconciliations`、GET `/{id}`和`/{id}/targets`、POST `/{id}/retry`。查看接受明确品牌或平台wallet.view权限，创建/重试还需当前品牌wallet.reconcile授权且超管只读；所有查询使用主库并提交审计后返回。结果只诊断，观察快照不授权自动修复；原创建/重试回执与当前状态分开，详细生命周期、字段和资源上限见[对账交接](13-wallet-reconciliation.md)。

六个不可变历史GET可采用物理从库，授权和查询审计仍以主库为准。成功响应中的`X-Read-Source`、`X-Read-Reason`及条件性的`X-Read-Replica`仅用于诊断，不是客户端选择器；原JSON结构、权限、品牌范围和分页不变。完整名单、WAL屏障、连接方式及回退说明见[复制与读路由手册](10-replication-and-recovery.md)。资金、投注、会话、当前配置及其他查询不进入该白名单。

站内模板配置使用GET `/api/v1/admin/notification-templates`、GET `/{key}/history`和PUT `/{key}`，需要独立模板权限；版本是每品牌每事件的安全整数，不是品牌配置版本。用户消息content为生成时的双语源文案，只有旧v1允许null；模板更新不改旧通知，也不改变业务积分。服务端及用户端拒绝未知占位符和私密变量，详见 [模板合同](12-notification-templates.md)。

合规配置GET/PUT `/api/v1/admin/compliance-policy`、历史GET `/compliance-policy/history`、显式检查POST/GET `/compliance-checks`及真实业务拒绝GET `/compliance-gates` 已注册。政策五键全量替换，版本独立；任一检查开启但未接真实适配器时，新注册/首次入品牌/运营新增及投注预览/提交被409 COMPLIANCE_REVIEW_REQUIRED拒绝，不建身份/扣分/建单。拒绝证据与加密负回执同事务，原键重放不重复证据。提现状态机亦拒绝启用而未接入的合规检查。显式管理模拟不是实际业务检查；默认关闭仅跳过，不证明验证完成。真实验证、复核与合规资金冻结未实现，边界见 [安全说明](07-security-risk-compliance.md)。

用户与管理员令牌不能互换。非浏览器客户端可以使用 Bearer；管理员浏览器 Cookie 名称为 `lottery_admin`，用户 Cookie 名称随品牌 UUID 变化，见安全方案的说明。生成客户端通常使用 Bearer；网页客户端继续使用既有 HttpOnly Cookie 流程，不把令牌存入本地存储。

管理接口按实际要求携带 `X-Brand-ID`；权限列表是说明，不替代服务端授权。`x-permissions` 中多个值是否为替代权限、以及超级管理员的特殊限制，以操作描述和业务权限规则为准。可信反向代理必须保留原始 Host，品牌路径不能覆盖独立品牌域名。

变更操作按照 `x-idempotent-operation` 和请求头定义发送幂等键。结果未知时，使用原键和完全相同的请求正文重试，不创建新意图。Cookie 请求还必须携带匹配 Host 的 Origin。成功回执代表原操作结果，不一定代表当前资源状态，应独立查询确认。

积分和财务合计使用十进制字符串。钱包与单笔积分由服务端限制在 int64 范围，积分变动允许负数，正常余额不允许负数；报表合计可能超过 int64。不得转换为 JavaScript Number。Schema 的字符串模式检查规范格式，数值上限、UTF-8 字节长度、重复 JSON 字段、规则语义和业务状态仍由 Go 校验。

两个报表的GET `/api/v1/admin/reports/{betting|ledger}/export`成功返回CSV原始字节，不套JSON信封；错误仍为JSON。view和export分别接受当前品牌或显式平台授权，必须同时满足，不发送分页参数。客户端应校验完整字节摘要和筛选回显，再按文本导入积分列；10,000分组/4 MiB上限、固定列和审计语义见 [导出合同](11-report-csv-exports.md)。不把导出当作不可变财务结账。

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
