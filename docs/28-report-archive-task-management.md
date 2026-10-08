# 自动归档任务管理交接

自动归档的业务口径沿用[27号核心合同](27-automatic-report-archive-core.md)。本页合同覆盖当前配置只读、任务查询和失败人工重试；配置写入另见[29号启用合同](29-report-archive-activation.md)，不提供公开历史回补。首次启用已确认从品牌时区当天/当月开始，周期结束后首版；日/月初始关闭，不因部署代码自动开启。

## 正式接口

全部位于 `/api/v1/admin`，必须携带 `X-Brand-ID`，使用主库及当前管理会话，不接受品牌路径别名。

| 方法与路径 | 用途 | 成功数据 |
| --- | --- | --- |
| GET `/report-archive-policy` | 当前配置，只读 | AutomaticPolicy |
| GET `/report-archive-tasks` | 本品牌分页任务 | AutomaticTaskPage |
| GET `/report-archive-tasks/{id}` | 本品牌指定任务 | AutomaticTask |
| POST `/report-archive-tasks/{id}/retry` | 显式重试失败任务 | 原操作的 pending 任务回执，HTTP 200 |

列表仅接受 `limit`、`offset`，默认20/0，范围1–100和0–1,000,000；按创建时间、ID降序。总数是精确十进制字符串，空页不是“所有任务均完成”的证明。其他读取和重试不接受查询参数；GET不接受正文。路径ID须规范UUID。

配置恰好包含 `brand_id,version,daily_enabled,monthly_enabled,daily_start_period,monthly_start_period,timezone,audit_log_id,updated_at`。初始版本1、两个开关false、两个起点及审计null；后续保存须有真实审计。当前配置不是旧任务的原配置。

任务恰好包含 `id,brand_id,policy_version,window,state,version,attempt_count,archive_id,last_error_code,creation_audit_log_id,last_audit_log_id,created_at,updated_at`。window恰好为 `kind,period_key,timezone,from,to`，类型daily/monthly。原配置版本、时区和绝对区间冻结，前端不能按当前品牌时区或浏览器时区数据库重新推算其范围。

pending初始版本1/尝试0；每次执行终结时版本及尝试各加1，failed显式重试只增加版本、不增加尝试。故pending版本为2×尝试+1，终态版本为2×尝试。completed和skipped均有真实archive_id且错误null；skipped表示复用已有归档、不自动追加。failed归档null，错误仅 `ARCHIVE_FAILED`。响应不包含SQL、配置游标或钱包私有证据。

## 授权、审计及原回执

查看需要明确的 `report_archive.view.brand` 或 `report_archive.view.platform`。重试还必须是该品牌成员，同时具备品牌查看和 `report_archive_task.retry.brand`，超级管理员不能重试；平台查看不能替代品牌查看授权。停用品牌拒绝重试，暂停品牌仍允许处理既有工作。

读取完成并提交查询审计后才响应，动作分别为 `report_archive.policy.read`、`report_archive.task.list`、`report_archive.task.read`。锁等待及审计等待后复核会话；过期会话或撤权不能获得数据或旧成功回执。重试审计记录原/后任务、实际管理员、IP、请求编号及原因，与状态和幂等回执同事务提交。

重试正文恰好为 `{version,reason}`，版本为1至MAX_SAFE_INTEGER−1，原因已去首尾空白、非空、最多500 UTF-8字节、无控制字符。不接受未知/重复/缺失键或null。头还须包含 `Idempotency-Key` 和确认时的 `X-Report-Archive-Actor-ID`，后者须匹配当前管理员。幂等命名空间为 `admin.report_archive.task.retry`，正文指纹包含品牌和任务ID。

新重试只能针对匹配版本的failed，且当前对应开关允许。成功200只确认原任务已恢复pending，不表示worker已完成归档。之后即使任务已经completed，原键重放仍返回原pending回执；当前状态须独立GET。关闭开关后不允许新重试，但有效会话和授权仍可确认此前已提交的原回执。

非法正文或分页沿用 `REQUEST_INVALID`；确认账号不匹配401、权限不足403、品牌/本品牌目标不存在404、旧版本或状态冲突409、忙锁/技术/审计错误503。忙锁和审计失败不缓存成功回执，不留半份状态。具体错误码以生成的[OpenAPI](openapi.json)及真实处理器为准。

## PC与移动后台

“自动归档任务”提供桌面侧栏及移动更多入口，按需加载，中英文切换。完整显示配置、分页任务和详情，版本、尝试数及原范围分开展示；只有授权品牌账号且详情failed时才出现重试准备操作。

先填写原因，核对任务、原版本、原配置/范围、账号、品牌和键，再勾选确认提交。每个账号/品牌只保留一个未解决意图，冻结普通JSON正文，不随当前任务状态改变目标或版本。意图仅在当前应用会话内存中保存；离页返回、只读刷新及切换语言不自动发送，不使用localStorage/sessionStorage，不承诺硬刷新或关闭后的恢复。

响应丢失或畸形写入结果显示“结果未知”；须核对冻结请求并再次勾选后才能用原键显式重放。列表或详情已经completed也不能擅自确认原请求。实际原ACK独立展示，可与最新completed详情同时显示pending版本，不能拿最新GET冒充回执。

409保留冲突意图，成功重读原任务及列表后，人工勾选确认丢弃，才可准备新操作；丢弃仅清本地输入，不删除服务端任务。当前401/注销/换账号清理会话意图；旧会话迟到401、成功或409不能清理或改绑新请求。品牌、权限、目标与请求代次隔离迟到读取。

移动任务按钮及长UUID允许断行，分页总数不转换为Number，不以横向溢出扩大的innerWidth作为验收宽度。页面无启用、回补、资金调整、删除或自动重试控件。

## 验证及环境边界

真实PostgreSQL HTTP测试覆盖初始关闭、空页、严格输入、品牌/平台授权、跨品牌隔离、版本冲突、原ACK重放、撤权、审计等待过期、忙锁/审计回滚和资金不变。SDK验证闭合字段、精确分页数量、全页ID唯一、任务状态/版本关系及未知写入；组件验证原请求与最新查询独立、409核对、会话隔离及语言切换。

独立desktop/mobile浏览器库通过正常迁移、seed、管理员及会员创建、真实人工充值37。仅带 `browserfixture` 标签的隔离工具通过核心配置/发现和一次捕获审计故障产生真实failed任务，随后正式HTTP重试提交后丢失响应；显式worker执行完成，再从页面原键取得原pending ACK。归档只有一版，余额37/账本一笔，全流程经济数据不变。fixture不进入生产构建，不关闭来源守卫，不接受原开发库、远端库、未知用户、读库覆盖或非空初始化。

新增CI双视口独立数据库任务，每端必须恰好一项成功、零重试/跳过/flaky。通用浏览器仅排除已有独立覆盖的人工及自动归档两个spec，其他既有分片保留。视口验收不代表苹果/安卓/鸿蒙真机、生产容量或客户人工代码审核完成。

本阶段无新迁移，部署仍须先正常升级至0067，再协调API、worker及管理端。原开发public保持0041且未切换服务。首次启用起点已确认、配置PUT已接入，配置编辑页面、OPEN-106外部文件保留与财务模板、OPEN-117特殊组合及其他生产验收继续保留；默认不自动删除。
