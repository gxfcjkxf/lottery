# 单机测试环境部署

## 机器检查（2026-11-01）

以下为首次通过 `ssh lottery` 只读检查时的状态。基础设施现已安装，实际验收结果见文末。

| 项目 | 当前状态 / 处理方案 |
| --- | --- |
| 系统 | Ubuntu 26.04.1 LTS，x86_64，1 vCPU |
| 内存 | 956 MiB RAM，496 MiB swap；增加 1.5 GiB swap，swappiness 调整为 10 |
| 磁盘 | 25 GB 根盘，可用约 21 GB；限制 journal 为 100 MB、保留 7 天 |
| 服务 | 仅 SSH、DNS、chrony 等基础服务；无失败的 systemd 单元 |
| 必需设施 | 安装 Ubuntu 源的 PostgreSQL 18、Nginx、Certbot、客户端与运维工具 |
| PostgreSQL | 仅监听 127.0.0.1；40 个连接、128 MB shared_buffers、2 MB work_mem；API/worker 各 6 个连接 |
| 安全 | UFW 未启用；脚本只开放实际 SSH 端口、80、443；关闭 SSH 密码登录，保留 root 密钥登录 |
| 时间 | UTC，chrony 时间同步正常；保留当前系统时区，业务时区由项目设置控制 |
| 构建 | 在本地构建 Linux amd64 二进制及四个前端，服务器不安装 Go、pnpm 或运行 Vite |
| 暂不安装 | 当前业务后端未接入 Redis、NATS、MinIO，不运行 compose 中的这些预留服务 |

当前规格适合少量功能试用，不代表并发容量已验证。浏览器渲染开奖源额外需要 Node、Playwright 与 Chromium；本套脚本不安装这部分，也不设置 `PLATFORM_DRAW_BROWSER_*`。启用浏览器源前须单独部署并验证该运行时，不能当作普通 HTTP 源使用。当前小内存机器上，浏览器进程还会与 API、worker 和数据库竞争内存。

PostgreSQL 18 是当前 Ubuntu 26.04 的系统包，原开发 compose 使用 PostgreSQL 17。首次部署已确认同一基线能在 18.6 上初始化并通过应用迁移校验。参数依据这台共用 1 GB 内存的主机设置，未关闭 fsync、同步提交、完整页写入或 autovacuum。参考：[Ubuntu 包](https://packages.ubuntu.com/resolute/postgresql-18)、[PostgreSQL 内存参数](https://www.postgresql.org/docs/18/runtime-config-resource.html)、[Nginx TLS](https://nginx.org/en/docs/http/configuring_https_servers.html)、[Certbot 续期钩子](https://eff-certbot.readthedocs.io/en/stable/using.html#renewing-certificates)。

## 入口与域名

| 入口 | 域名 |
| --- | --- |
| 用户端 | `cp1608.xyz`、`www.cp1608.xyz` |
| 总后台 | `manager.cp1608.xyz` |
| 代理端 | `agent.cp1608.xyz` |
| 品牌后台 | `admin.cp1608.xyz` |

首次检查时 Cloudflare `1.1.1.1` 和 Google `8.8.8.8` 返回 `NXDOMAIN`；本次安装后复查，两者和测试机解析器已全部正确解析到 `172.104.41.93`。`*` 不覆盖根域名 `@`。全部入口的 A 记录应指向测试机 `172.104.41.93`；如设置 AAAA，也必须指向这台机器的可用 IPv6 地址。首次脚本会验证所有实际域名的解析，不会在失败时降级到 HTTP。

总后台、品牌后台和代理端为固定管理入口，三个域名均登记到 `platform_domains`，不绑定某个品牌。只有用户端域名写入 `brand_domains`。不运行 `seed`，不创建演示品牌。品牌后台登录后按账号授权选择品牌；代理用户名全平台唯一，登录后由会话确定品牌。固定入口登录前使用平台验证码配置。代理注册链接读取账号所属品牌启用的用户域名（按域名字典序选取一个），不再使用构建时固定域名。

品牌域名在总后台管理。当前同一根域名下的一级用户端子域名使用通配符 DNS、证书和 Nginx 路由，新增时绑定并启用即可。未绑定的主机由业务接口明确返回品牌不存在，不指定默认品牌。根域名和一级子域名必须已解析到测试机；外部根域名、二级嵌套子域名及后台站点拓扑不在此方案支持范围，须单独配置。`manager`、`admin`、`agent` 保留明确的固定站点路由，优先于用户端子域名规则。

## 三个脚本

在仓库根目录执行，构建机需要 Bash、Node >=24、pnpm、Go >=1.26、Python 3、OpenSSL、tar、SSH/SCP。脚本通过 `pnpm dlx` 使用 `package.json` 指定的精确 pnpm 版本，安装依赖时使用 frozen lockfile。编译使用当前工作区快照，包含未提交的源码；隔离临时构建目录，不触碰本地 DEV 服务和前端 dist。

1. 基础设施初始化：

   ```sh
   ssh -o BatchMode=yes lottery 'bash -s' < deploy/test-host/01-infrastructure.sh
   ```

   初始化还会创建 `/etc/letsencrypt/namecom.json` 空模板，root 0600。首次部署前填入 Name.com 生产用户名和 Production token（配置方法见文末），不要写入仓库或聊天。

   仅支持这次检查的 Ubuntu 26.04 x86_64 新机器；安装后会启用防火墙并禁用 SSH 密码登录。当前 SSH 会话不关闭，完成后用另一个终端验证 `ssh -o BatchMode=yes lottery true`。不对已有项目执行此脚本。

2. 全新项目部署：

   ```sh
   cp deploy/test-host/config.example.json /tmp/lottery-test-config.json
   # 编辑 certificate_email，核对域名、admin_username 及 draw_allowed_origins。
   deploy/test-host/02-deploy.sh lottery /tmp/lottery-test-config.json
   ```

   配置为严格 JSON。`draw_allowed_origins` 初始空数组表示尚未批准外部开奖源；配置真实来源后应同步更新远端受保护的 `platform.env`，不能跳过来源检查。首次创建专用数据库/角色、持久认证密钥、管理域名和平台管理员，通过 Name.com DNS-01 签发根域名与通配符 HTTPS 证书，安装并启动 systemd 服务。管理员密码随机生成，保存在远端 root 专属 `/etc/lottery/bootstrap.env`，脚本不打印密码；在私密终端读取，首次登录修改密码后删除该文件。默认用户名 `admin` 保留项目已有的初始平台账户能力，可在配置里明确更改。

   初始部署不是自动重试流程。失败后保留原始错误及已创建资源，检查状态后继续处理，不删除数据库或重造认证密钥。基础设施已安装但未创建项目时，可以纠正错误后再次执行基础设施脚本；若已经开始项目初始化，不要靠重跑初始化脚本恢复。

3. 日常更新：

   ```sh
   deploy/test-host/03-update.sh lottery
   ```

   读取远端已保存的域名配置，在本地构建和上传带校验和的新版本。远端部署加锁，切换前校验数据库迁移与实际开奖源可读性、准备证书并校验 Nginx；停止 API/worker 后备份数据库，再切换 `/opt/lottery/current` 并重启服务。校验 API readiness、四端 HTTPS 页面和服务状态。更新期间有短暂维护窗口。切换后仅提供当前版本的静态资源；已打开的旧页面需要刷新后继续使用。

   当前是发布前单基线阶段，日常更新**只接受相同基线**。基线变化会在停止服务前报错；须按仓库迁移约束先备份、在副本验证结构和业务数据，再显式同步迁移记录，不自动修改 checksum。正式发布后若引入增量迁移，需要另行审阅并调整发布步骤，当前脚本不擅自迁移现有库。

## 文件和检查

- `/opt/lottery/releases/<版本>`：root 所有的当前应用二进制、静态文件和发布配置；新版本健康检查通过后删除旧发布目录。
- `/etc/lottery/platform.env`：root 0600 的 systemd EnvironmentFile，不能当 shell 脚本 source。
- `/etc/lottery/auth.key`：lottery 0600，更新时永久保留；丢失会影响幂等记录解密。
- `/etc/lottery/deploy.json`：域名及构建配置，不含密码。
- `/var/backups/lottery/<版本>.dump`：更新前数据库逻辑备份及 SHA-256；root 专属。脚本检查归档目录可读性，未声称做过恢复验证。它不包含认证密钥，也不提供异地备份。

```sh
ssh lottery 'systemctl status lottery-api lottery-worker nginx postgresql@18-main --no-pager'
ssh lottery 'journalctl -u lottery-api -u lottery-worker -n 80 --no-pager'
ssh lottery 'curl -fsS http://127.0.0.1:8080/health/ready'
```

失败会明确停止并保留候选版本；若已进入停服阶段，API/worker 保持停止，不自动回滚应用或数据库。排查后人工修复，再重新更新。成功更新后删除旧发布目录及旧静态资源副本，数据库备份继续保留。失败时保留当前运行版本和候选目录，供排查。API readiness 核验数据库和迁移；worker 本次只检查进程状态及部署前来源读取，未启用 metrics，不代表业务任务或资金流程已验收。

脚本验证不登录共享开发账号、不向正常开发库写测试数据。实际测试机的首次业务试用、PostgreSQL 18 迁移验证和 HTTPS 验收需要执行部署后完成。

## 本次本地验证

三个入口和 SSH 下发的脚本均通过 Bash 语法与 ShellCheck。通过拦截 SSH/SCP 的演练实际完成隔离快照构建、四端类型检查与生产构建、Go Linux amd64 编译、上传包与校验和生成；检查包内包含四端页面、正确代理注册链接，不含本地 env、认证密钥或 node_modules。非法域名配置会在连接 SSH 前拒绝。

构建通过，但现有共享 CSS 的 `.reward-list-actions)` 选择器产生语法警告，部分 JS 包也超过 Vite 的 500 kB 提示值；本次不修改业务页面。远端基础设施及首次部署已完成，实际验收结果见下文。


## 基础设施安装验收（2026-11-01，Asia/Singapore）

已执行 `01-infrastructure.sh`。实际 Ubuntu Nginx 默认配置已声明 `server_tokens`，脚本已修正为修改原声明，并复用其 gzip 开关，避免重复定义；修正后的脚本执行成功。

- PostgreSQL **18.6**、Nginx **1.28.3**、Certbot **4.0.0** 已安装。数据库、Nginx、证书续期定时器及自动安全更新服务均为 active，已配置开机启动。PostgreSQL 集群由 Ubuntu 的 systemd generator 管理，集群 `auto` 启动对应 `enabled-runtime`。
- PostgreSQL 仅监听 `127.0.0.1:5432`；实际查询确认 40 个连接、128 MiB shared_buffers、2 MiB work_mem，fsync、完整页写入、同步提交及 autovacuum 均保持开启。
- swap 总量约 **2 GiB**，`vm.swappiness=10`；journal 限额配置已落盘。UFW 已启用，入站仅允许 **22/80/443**（IPv4/IPv6）。
- 新建独立 SSH 连接验证密钥登录成功；密码和交互式密码认证已关闭，root 保留密钥登录，X11 转发已关闭。
- Nginx 配置检查通过，systemd 无失败服务，系统没有待重启标记。此阶段尚未创建项目数据库和应用服务；这些步骤已在下述首次部署中完成。
- 公网 `1.1.1.1`、`8.8.8.8` 及测试机解析器均确认 `cp1608.xyz`、`www.cp1608.xyz`、`manager.cp1608.xyz`、`agent.cp1608.xyz`、`admin.cp1608.xyz` 的 A 记录为 **172.104.41.93**；没有 AAAA 记录。两家公网解析器查询根域名均无 CAA 限制。

五个域名的 HTTP ACME 验证路径均通过公网访问和测试机按域名访问测试，临时验证文件已清理。构建机本地系统解析器本轮仍返回找不到主机，公网连通性检查显式使用公网 DNS 已核实的 IP（curl `--resolve`）；测试机自身的正常域名解析和访问均已通过，不影响下一步 Certbot 在测试机申请证书。

基础设施及 DNS 验收完成后，已使用用户指定邮箱继续执行首次部署。


## 首次部署验收（2026-11-01，Asia/Singapore）

已部署版本 `20261101T103134Z-7e8324a0`，当前链接为 `/opt/lottery/current`。实际邮箱已保存在本地 `.local/test-host-deploy.json` 和远端受保护部署配置中，不写入示例配置。

- 在专用空库 `lottery` 初始化 `0001_baseline.up.sql`，应用校验通过；仅创建平台管理员 `admin`，品牌数为 0，没有导入演示数据。
- Let's Encrypt 证书包含五个入口域名，到期日 **2027-01-30**；Certbot 续期定时器正常。公网校验证书链、五个入口的 HTTPS readiness、公共显示设置接口及 HTTP→HTTPS 308 跳转均通过。
- API、worker、Nginx 和数据库均 active；API/worker 自动重启计数均为 0，系统无失败服务。worker 已完成官方日历同步；没有批准或配置外部开奖源，不能据此宣称开奖业务流程已经验收。
- 浏览器访问五个入口，页面均返回 200、无未捕获 JavaScript 错误。总后台登录表单正常；总后台和品牌后台在 320 CSS px 下无横向溢出。未登录初始管理员，未创建测试账号或业务对象。
- 用户端和代理端当前展示品牌不可用状态，品牌后台也需要先绑定具体品牌才能加载登录配置。下一步由用户登录总后台创建品牌，绑定 `cp1608.xyz`、`www.cp1608.xyz`、`agent.cp1608.xyz`、`admin.cp1608.xyz`，并配置品牌后台账号及业务设置。
- 初始密码位于测试机 root 专属 `/etc/lottery/bootstrap.env`；认证密钥和环境文件权限已验证为 0600。部署日志不包含密码。

实际运行修正已写回脚本：地址检查跳过 `iproute2` 返回的空地址记录；本地打包禁用 macOS 扩展属性，避免 Linux tar 的无关警告；首次部署仅登记总后台域名，品牌后台域名留给真实品牌绑定。最初地址检查失败的候选发布包和目录保留在测试机，不是当前运行版本。

## 彩种数据迁移

随后已按用户确认范围迁入香港、新澳门和老澳门三个平台彩种及其配置与开奖数据。未迁入原品牌及销售配置、会员、注单或资金数据；详见 [迁移记录](marksix-migration-20261101.md)。来源白名单已同步到远端 `platform.env`、`deploy.json` 和本地实际部署配置，日常更新可继续执行既有来源校验。

## 域名管理归属（2026-11-01 更新）

域名管理已统一放在总后台“品牌列表 → 对应品牌操作菜单 → 域名”。品牌后台不再显示域名页签，品牌域名 API 已下线；品牌角色目录及可分配权限不再包含旧的域名权限，旧授权不能继续操作。平台管理员仍须具备精确的 `brand_domains.view.platform` / `brand_domains.write.platform` 权限，不以管理员身份代替授权。现有绑定保持不变。

已在独立测试库验证平台写入、原键重试、权限撤销、品牌隔离及旧品牌接口关闭，并清理测试库。中英文界面覆盖安卓/iOS 横竖屏、桌面及 320 CSS px；浏览器使用拦截接口的独立模拟会话，未登录使用中的管理员或写入真实品牌。最终部署版本为 `20261101T112631Z-a5475790`。日常更新配置检查同步修正为比较 JSON 内容，避免因末尾换行误判为配置变化。


## 通配符 HTTPS（2026-11-01 更新）

已启用 `lottery-wildcard` 证书，覆盖 **cp1608.xyz** 和 **\*.cp1608.xyz**，到期日 **2027-01-30**。`https://yx.cp1608.xyz` 已验证解析到「游侠」品牌。通配符覆盖一级子域名，不覆盖 `admin.yx.cp1608.xyz` 这类嵌套子域名；根域名本身作为独立 SAN 一并申请。

部署脚本以 `user_domains` 第一项作为 DNS 根域，要求所有固定入口属于此根域或其一级子域名。Name.com 的生产 DNS API 权限会在构建前检查，缺失、DEV 凭据或授权失败均明确停止，不切换到其他验证方式。三个既有脚本已接入 DNS-01 签发及通配符路由；辅助文件 `namecom-dns.py`、`render-nginx.py` 随发布包复制，DNS hook 稳定安装在 `/usr/local/lib/lottery/`，不会因发布目录切换而失效。

生产凭据仅保存在测试机 **root:root 0600** 的 `/etc/letsencrypt/namecom.json`：

```json
{"username":"生产用户名","token":"Production token"}
```

通过私密终端编辑，不在 shell 参数、源码或日志中放 token。DEV 用户名（通常带 `-test`）和 DEV token 不能修改真实域名 DNS，不能用于真实通配符证书。当前 hook 只调用 Name.com **Core API**，只新增 `_acme-challenge` TXT；清理前校验记录类型、主机和本次验证值，只删除本次创建的记录 ID，不修改现有 A、AAAA、MX、NS 或其他 TXT。

Certbot 保存了 DNS auth/cleanup hook 和 root 域名，沿用 `certbot.timer` 自动续期；续期成功后执行已有 Nginx 检查与重载 hook。已完成测试签发、正式签发，以及读取持久化配置的 `certbot renew --cert-name lottery-wildcard --dry-run --run-deploy-hooks`，全部成功。旧单域名证书暂时保留，当前站点均使用 `lottery-wildcard`；新证书签发失败会报错，不使用旧验证方式冒充成功。

验收：根域、www、yx、manager、admin、agent 的 HTTPS 和静态站点内容逐一核对通过；`yx` 的 context 指向 `youxia`，未绑定子域名返回 404。游侠用户端在桌面、Pixel 7 和 iPhone 13 下无脚本或资源加载错误，无横向溢出。Nginx 配置切换前备份位于测试机 `/var/backups/lottery/tls-wildcard-20261101_122217/`。本次仅重载 Nginx，没有修改品牌数据或重启 API/worker。

参考：[Let's Encrypt DNS-01](https://letsencrypt.org/docs/challenge-types/#dns-01-challenge)、[Name.com 生产/测试环境](https://docs.name.com/guides/authentication)、[Core API 创建 DNS 记录](https://docs.name.com/api/v1/reference/dns/create-record)、[删除 DNS 记录](https://docs.name.com/api/v1/reference/dns/delete-record)。


## 未绑定用户端域名（2026-11-01 更新）

版本 `20261101T123834Z-f3a10be7` 在展示品牌界面前完成域名识别。未绑定域名（例如 `y6x.cp1608.xyz`）显示无品牌的“站点不可用”页面，清除默认品牌名称、图标和安装入口，不显示登录表单或普通业务错误提示；品牌识别接口继续返回 `404 BRAND_NOT_FOUND`，不将请求落到任何默认品牌。真实读取故障提供重试，不把网络故障当作已绑定成功。

已验证登录页和受保护路径直接访问、中英文、安卓/iOS 横竖屏及 320 CSS px；正常的 `yx.cp1608.xyz` 继续展示游侠登录页。本次只修改用户端展示，未修改品牌绑定、账户或资金数据。通配符证书及自动续期配置随本次日常更新保留并生效。


### 2026-11-01 固定管理入口修复

旧基线 `7b002f5d…` 的一次性升级使用 `03-update.sh lottery --upgrade-fixed-entries`，后续仍使用普通更新。升级会停止写入、备份、恢复独立副本，核对所有业务表的行哈希，并将升级副本与新基线空库结构逐项比较；通过后执行相同 DDL 并显式同步迁移记录。不清库、不修改业务账号名；发现重名代理账号直接失败。变更仅为代理用户名全平台唯一和允许验证码不绑定品牌。


本次最终发布 `20261101T130755Z-9ef59f07`。三个管理入口已登记，只有 `yx.cp1608.xyz` 保留品牌域名绑定。远端升级证据位于 `/var/backups/lottery/fixed-entries/20261101T130110Z-6efbfe05/`，本地证据在 `.local/fixed-entry/local-applied/`；两端新基线 checksum 为 `d63a4186289ba6ffe370b69c01e7acd3026204d499587ea7fe6efa2351f3a6be`，业务表逐行哈希不变，结构与新建基线一致。

验证：后端身份与代理测试、固定入口 Cookie 登录/修改密码/撤销会话、品牌隔离和全局账号唯一约束通过。独立测试数据库已清理。代理固定入口 12 项、品牌后台单/多品牌选择 24 项、三端验证码交互 16 项浏览器用例通过；覆盖中英文、安卓/iOS、横竖屏、320px 和桌面，实际渲染无范围内可见问题，沿用共享控件及设计令牌；无新增装饰动效。线上三个登录页的 18 个设备视图已检查，无误请求用户 context、无页面脚本异常、无横向溢出。me 在未登录时的 401 用于正常恢复会话，不是登录失败。

未计为通过：完整契约检查 121 项中 116 项通过、5 项失败，涉及平台域名路由目录、两项筛选目录、历史读取头与提现 OrderPage DTO 的既有不一致；完整旧代理业务浏览器用例还引用已不存在的 `.facts` 汇总节点。本次未扩展修改这些断言，专项结果与完整检查结果分开保留于 `.local/fixed-entry/`。


### 2026-11-01 总后台旧彩种接口清理

发布 `20261101T133257Z-f49f366e`。移除总后台旧品牌自建彩种列表、期次、开奖结果、玩法及规则版本客户端和失效详情面板；彩种与开奖页只使用现行 `/platform-games` 链路，不再依赖品牌选择。注单彩种筛选改为 `filter-options?resource=bet&kind=game`，按注单权限读取该品牌实际配置化订单引用的彩种。

152项总后台单元测试、3项后端真实HTTP集成测试和12项中英文浏览器用例通过，覆盖安卓／iOS横竖屏、320px及桌面，逐项检查所有彩种详情与注单筛选，无旧接口调用或横向溢出。类型检查、构建、OpenAPI检查通过。本地DEV已用 `.local/review.env` 重编译重启并完成来源和就绪检查；独立测试数据库已清理。本次不改数据库结构。

测试机API和worker运行正常，健康检查通过。现行彩种及新增筛选组合在匿名请求下返回401，旧 `/platform/games` 按下线约定仍返回404；部署后检查实际JavaScript中已不存在旧彩种、玩法和规则版本客户端。未登录共享管理员账号，也未向正常使用品牌写入测试对象。证据保存于 `.local/platform-games-fix/`。


### 2026-11-01 普通品牌账号授权加载修复

已发布 `20261101T140440Z-f9e9d7c5`。品牌基础目录按账号已分配的 `admin_brand_scopes` 返回，不额外要求 `brand.view.brand`；通用管理入口的首页允许账号已授权品牌，继续要求 `overview.view.brand`。专属品牌域名限制、跨品牌拒绝、业务权限和总后台目录授权保持有效。本次不修改数据库结构，也不修改 `yx001` 或其角色权限。

独立测试库真实HTTP回归通过；普通账号仅有首页查看权限的12项真实浏览器用例通过，覆盖中英文、安卓/iOS横竖屏、320px及桌面，从登录到品牌发现、首页、刷新恢复、403权限检查和退出均使用真实接口。类型及前端构建随部署通过，OpenAPI校验通过。本地DEV已用原配置编译重启并完成实际来源读取和就绪检查；测试数据库及临时schema已清理。测试机API/worker运行正常，readiness通过，只读确认 `yx001` 为普通账号、1个品牌范围且有首页权限，未登录该使用中账号。证据位于 `.local/brand-scope-fix/`。


### 2026-11-02 手动历史结果基线升级与本地版本部署

本地工作区快照（含未提交源码）发布为 `20261102T044346Z-df77a06e`。新增一次性入口 `03-update.sh lottery --upgrade-manual-results`，只接受旧基线 `d63a4186…` 和目标基线 `a65ec6f3…`；本次升级后继续使用普通 `03-update.sh lottery`，不可重跑一次性升级。

变更仅放宽 `platform_draw_history.source` 约束以接受 `manual`。停写后先备份、恢复独立副本，验证所有业务表逐行哈希不变，并与新基线空库结构比较，通过后对原库应用相同 DDL 并显式同步迁移记录。远端证据沿用现有升级备份目录：`/var/backups/lottery/fixed-entries/20261102T044346Z-df77a06e/`，其中 `verification.json` 确认数据未变且结构一致；临时验证数据库已清理。常规更新前数据库备份为 `/var/backups/lottery/20261102T044346Z-df77a06e.dump`。

共享组件 224 项单元测试、独立测试库的 3 项彩种 HTTP 权限/重试/隔离测试、OpenAPI 校验以及四端类型检查和生产构建通过。本地 DEV 按 `.local/review.env` 重编译重启，来源读取及 readiness 通过。测试机 API/worker active，重启计数为 0，readiness 和三个实际开奖源读取通过。未登录共享账号或向正常品牌写入测试对象。构建仍有部分 JavaScript 包超过 500 kB 的提示。

线上浏览器 26 个 HTTPS 视图检查通过，覆盖六个域名桌面，以及用户端和三个管理入口的 Android/iOS 横竖屏与 320 CSS px；无页面脚本异常、静态资源失败或横向溢出。检查未登录账号。桌面和 320px 截图及验证日志保存在本地 `.local/deploy-20261102/`。
