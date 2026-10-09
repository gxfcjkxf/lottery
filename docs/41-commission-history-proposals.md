# 历史佣金恢复依据申请与审核

历史错误stale资金的恢复先保存完整依据，再显式核验是否仍有效。本阶段实现离线申请、单人审核与查询，复用不可变审计及加密幂等回执；`reviewed`仅表示依据已审核，`execution_available`始终false。它不修复支付状态、合并实际净额头、取消旧执行任务、解除暂停或补发/追回积分，0075升级预检继续拒绝旧错误历史。

## 入口与权限

沿用[40号离线准备](40-zero-original-commission-evidence.md)，排空并停止所有金融写入后准备到精确0074；新写入仍不允许普通API/worker在此检查点启动。新增四个CLI入口，无HTTP路径或新数据库迁移：

```sh
platform commission-recovery-source "$LOTTERY_REVIEW_BRAND_ID" "$LOTTERY_REVIEW_CYCLE_ID"
platform commission-recovery-propose "$LOTTERY_REVIEW_BRAND_ID" "$LOTTERY_REVIEW_CYCLE_ID" --confirm=record-only-review
platform commission-recovery-review "$LOTTERY_REVIEW_BRAND_ID" "$LOTTERY_REVIEW_PROPOSAL_ID" 1 --confirm=record-only-review
platform commission-recovery-get "$LOTTERY_REVIEW_BRAND_ID" "$LOTTERY_REVIEW_PROPOSAL_ID"
```

参数为规范小写UUID；审核版本为规范正整数且不超过9007199254740991。参数、确认项和必需环境值在配置/连接前校验。只使用主库，拒绝从库配置，整个命令最多30秒；流式来源读取另有15秒限制。错误不输出会话、密钥、数据库连接串或原始金融清单。

每个命令都须在安全环境提供`LOTTERY_COMMISSION_RECOVERY_BEARER`：现有服务器签发且仍有效的管理员会话令牌，不带Bearer前缀，不作为命令行参数，不保存到仓库。工具不提供密码快捷登录或绕过验证码/认证限次；应在排空前通过正常管理端取得会话。写入另需保持原`AUTH_KEY_FILE`或`AUTH_KEY`，不得生成替换密钥来恢复未知请求。

查看同时要求明确commission.view及report_commission.view，可分别为品牌或平台读授权。申请另需commission_correction.retry.brand；依据审核另需commission_correction.approve.brand，两种写入都必须是当前普通管理员、具有对应品牌范围，超级管理员不能写。钱包修复权不是替代授权。账户与角色在共享ACL门闩下重新加载，会话在事务内按墙钟复核；缓存重放前也检查当前权限，撤销或过期不会释放旧回执。一期允许同一名有权限的管理员申请并审核，不改变玩法不得自审规则。

## 来源摘要与状态

source返回完整Report和64位小写SHA256来源摘要。来源包含保存的周期、投注/结算、规则及代理修订、原/后续支付、人工修正、更正计划/执行/净额头/暂停、实际相关账本及金融审计。按固定命名空间和长度边界逐行规范化并流式哈希，不通过巨型JSON聚合或前若干行冒充完整数据；JSON数字保留精度，重复键拒绝，连接时区统一UTC。观察时间不进入摘要，申请/审核自身和无关管理员只读审计不改变金融依据；不以今天的代理比例或当前钱包余额充当实际已授予净额。

写入须同时提供环境值：`LOTTERY_COMMISSION_RECOVERY_DIGEST`为操作者已经查看的来源摘要，`LOTTERY_COMMISSION_RECOVERY_REASON`为非空且最多500字节原因，`LOTTERY_COMMISSION_RECOVERY_KEY`为8—128字符幂等键，只允许字母、数字及`_:.-`。申请锁定周期并重新读取来源，仅当摘要一致、存在stale实际资金且当前核算/有效目标完整时记录版本1 `awaiting_review`。金额及来源不由请求传入；原周期、支付、账本和余额均不修改。

审核锁定申请，要求版本1及原摘要，再重新核验全来源摘要和已保存金额投影。依据变化、版本冲突、损坏来源或权限不符直接拒绝，不覆盖原申请、不自动构造新申请。成功仅追加版本2 `reviewed`审计，保留申请人、审核人、两个时间和原因及原冻结快照，仍无金融执行权限。

记录使用审计资源`commission_history_recovery`，操作分别为`commission.history_recovery.propose`和`.review`。查询验证整条申请/审核链、品牌/申请ID、人员、版本、原摘要和前后字段，重复或畸形事件不成为可操作记录。可通过既有审计管理查询与导出。完整原始来源行只用于哈希，不输出或持久化；保存的是必要金额投影、摘要及审计记录。

## 幂等与后续恢复

申请和审核使用已有加密ExecuteChecked流程；键按品牌、当前管理员/版本及操作隔离，摘要绑定完整请求，审核还绑定申请编号。相同键/请求返回原不可变回执，不产生第二条记录；换申请、原因或摘要不能复用原键。当前查询与原回执分开，原申请回执即使后续已审核仍为版本1，不冒充最新状态。结果未知时仅显式使用原键及原请求恢复，不能自动换键重做。

本阶段没有执行入口。后续仍须对多笔原/后续实际支付、既有部分补偿及暂停建立完整追加基数，并在新事务重新验证来源、当前权限及资金门控；实际差额另需既有保存模式要求的新批准。不得将本次`reviewed`或来源摘要直接当作资金授权，不得改旧账本、造零额资金见证或跳过升级预检。生产大历史容量、实际元数据恢复及资金执行、相关管理UI与客户人工代码审核继续独立验收。
