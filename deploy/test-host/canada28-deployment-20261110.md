# 加拿大28测试机部署

2026-11-11 休市处理修复：上游 `rest_status=resting` 且 `estimated_timestamp=null` 时保留最新开奖结果，下一期期号及时间均返回 JSON null。映射器接受显式 null 对，暂停下一期排期；恢复后使用上游时间。缺字段、单个 null、无效号码或非休市缺时间仍拒绝。已覆盖 Python 校验、Go 映射解析及独立测试库的休市/恢复排期测试；本地 DEV 已通过标准入口重编译启动。适配服务脚本备份在 `/var/backups/lottery/jnd-rest-fix-20261111/`，证据位于 `.local/jnd-rest-*`。

2026-11-10（Asia/Singapore），将本地 `.local/review.env` 对应数据库中的平台彩种“加拿大28”部署到 `ssh lottery`，使用测试机现有应用版本 `20261109T124708Z-98baf623`。

## 数据范围

- 保留彩种 ID `01a14225-623e-7286-b797-8a58f4b3a89c`、编码 `canada_28_trial`、数字模型与彩种时区。
- 保留 1 个当前已发布配置化规则版本、原快照及校验值，以及 7 个历史加拿大赔率版本和当前指针。
- 按用户指定接入 `https://jnd-28.net/api/latest`，使用 `/api/countdown` 提供下一期期号和预计开奖时间。自动化版本从测试机版本 1 开始。
- 不导入本地合成来源、9,238 个合成期次和采集尝试；不混用合成开奖结果与真实开奖。
- 不迁入本地品牌、销售、会员、账号、注单、余额或资金流水。创建人和审核人外键映射到测试机现有平台管理员，追加迁移审计；没有登录共享管理员账号。

两端基线均为 `2eecf70b0e6d24778fa5f48122cd51984c61c4b00849ea6d6438225cc39a2a18`，未修改迁移记录。导入前后逐项比较既有品牌、彩种关联、销售、订单、会员、账号、积分账户、积分桶、资金流水及迁移记录的行数和全行摘要，全部一致。品牌的彩种接入与销售启用仍由其原有流程配置。

## 来源服务

现有映射器读取数字数组，上游返回三个独立数字字段，且下一期信息来自另一个接口。因此部署专用 Python 标准库适配服务，只读取上述两个公开接口，不执行预测或号码生成。

| 文件或入口 | 用途 |
| --- | --- |
| `/opt/lottery/feeds/canada28-feed.py` | 对应仓库 `deploy/test-host/canada28-feed.py` |
| `lottery-canada28-feed.service` | 非 root 服务账号运行，开机启动 |
| `127.0.0.1:8097/draw` | 本机 HTTP 适配入口 |
| `/etc/nginx/conf.d/lottery-canada28-feed.conf` | 对应仓库 `canada28-feed.nginx.conf`，仅监听 `127.0.0.1:8443` |
| `https://canada28-feed.cp1608.xyz:8443/draw` | worker 使用的 HTTPS 来源 |
| `/etc/hosts` | 将上述来源主机名解析到 `127.0.0.1`，无需公网 DNS |

HTTPS 使用现有通配符证书并验证证书，不改变生产环境的 HTTPS 来源限制，不对公网开放 8097 或 8443。来源已加入 `/etc/lottery/platform.env` 与 `/etc/lottery/deploy.json` 的白名单。Nginx 配置独立于日常更新生成的站点配置，`03-update.sh` 不覆盖适配服务文件。

适配服务核对数字范围、和值、下一期期号连续性、时间时区和先后顺序；网络或数据错误返回 502，不回退到合成来源或旧数据。worker 沿用现有 5 秒采集间隔，两接口各约 12 次/分钟。上游时间标记为 estimated，建期沿用其预计开奖时间，提前 210 秒开放、提前 15 秒封盘；不使用旧合成源的 120 秒周期。

本次仅接入最新开奖，不提供中断期间漏期的历史补采。来源更新停止时不会生成伪造结果。

## 验证与备份

先备份测试机并恢复到独立数据库 `lottery_canada28_live_test_20261110`，创建专属临时品牌后导入。现有应用的 schema 和全部来源读取检查通过。测试 worker 仅允许本机网络访问，避免副本中的其他业务向外部服务发送请求。

独立库实测自动建立 `3504706` 期，11:28:30 开奖后自动发布 `4、3、5`，并建立下一期 `3504707`。结束时停止测试 worker，清理独立库及临时品牌。

正式导入前短暂停止 API/worker、再次备份，在单事务中导入并核对全部导入值；既有业务数据摘要一致后更新来源白名单、完成来源探测并恢复服务。API readiness 正常。

测试机部署后再次实测：11:32:00 的 `3504707` 期于 11:32:06 自动发布 `5、1、4`，随后自动建立 11:35:30 的 `3504708` 期。来源状态为 `ready`，API、worker、适配服务与 Nginx 全部 active，总后台 HTTPS 健康检查通过。当前已发布规则快照 SHA-256 与本地一致：`79d7edc4e3fd8c8fa9935c277c39f0e419e498b7d96443aec8caf6245d78ee27`。

备份、原始只读快照、调整后的部署数据、导入 SQL、业务摘要及验证证据位于测试机 root 专属目录：

```text
/var/backups/lottery/canada28-20261110/
```

原始快照 SHA-256：`a3e67cbbee7e712455efd06f206579b4eedb42aa6ba80aae917d82c7f60c7e5d`。`before-apply.dump` 为正式导入前备份。导入拒绝重复彩种 ID 或编码，不覆盖已有配置。

运维检查：

```sh
ssh lottery 'systemctl is-active lottery-api lottery-worker lottery-canada28-feed nginx'
ssh lottery 'curl -fsS https://canada28-feed.cp1608.xyz:8443/draw'
ssh lottery 'journalctl -u lottery-canada28-feed -u lottery-worker -n 50 --no-pager'
```

更新适配服务时，将仓库脚本安装到 `/opt/lottery/feeds/canada28-feed.py`，执行 `systemctl restart lottery-canada28-feed`，核验 HTTPS 来源读取。修改 Nginx 配置后先 `nginx -t` 再 reload。日常应用更新继续使用既有 `03-update.sh lottery`。
