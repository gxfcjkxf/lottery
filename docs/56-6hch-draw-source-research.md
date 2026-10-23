# 6hch香港六合彩开奖来源研究

2026-10-16，研究阶段只读取公开数据。后续已按用户要求接入本地试用worker和两个后台，实施与验收记录见[57 香港真实开奖源接入](57-hong-kong-live-draw-source.md)。

页面：https://6hch.com/html/kaihistory.html

浏览器能读取历史记录。页面通过`js/config.js`定义来源域名，`js/kaihistory.js`查询年度历史，`html/public/Hkheader.html`中的脚本查询实时开奖。直接读取其公开JSON可避免解析页面布局。

## 实测接口

### 最近一期及下期信息

```sh
curl 'https://1680660.com/smallSix/findSmallSixInfo.do'
```

GET，无需登录、Cookie或密钥，本次返回HTTP200、`errorCode=0`、`result.businessCode=0`。以下为读取时的`result.data`关键字段：

```json
{
  "preDrawIssue": 2026110,
  "drawIssue": 2026111,
  "preDrawTime": "2026-10-15 21:41:21",
  "drawTime": "2026-10-17 21:30:00",
  "preDrawCode": "3,10,7,8,36,42,20",
  "chineseZodiac": [5,10,1,12,8,2,12],
  "fiveElements": [4,4,5,2,5,1,5],
  "color": [3,3,1,1,3,3,3],
  "serverTime": "2026-10-16 12:49:32",
  "type": 8,
  "lotCode": 10048
}
```

`preDrawIssue`是已开奖完整期号，`drawIssue`是下期；七个号码按顺序前六个正码、最后一个特码。当前返回对应正码03、10、07、08、36、42，特码20，与历史页第110期一致。不要把下期期号配到上期号码。

来源脚本按`type`区分等待、搅珠、节目状态及完成状态；8走完整结果展示分支。4虽显示“开奖结果”仍继续秒级查询，不能仅依据页面标题判断最终数据。本项目适配应先只接收明确完成的8及完整有效的七个号码，其他状态保持待开奖。字段不符应明确报错，不换源或填充号码。

### 年度历史

```sh
curl 'https://1680660.com/smallSix/findSmallSixHistory.do' \
  --data 'year=2026&type=1'
```

页面使用POST表单，读取`result.data.bodyList`；每行有`issue`（年内期数）、`preDrawDate`、`preDrawCode`、`color`和`czAndFe`等字段。本次首行是2026-10-15、第110期，号码与实时接口相同。它适合明确期号查证和历史导入，不应将响应首行无条件当作当前待结算期次。

开奖日历由GET `https://1680660.com/smallSix/queryLotteryDate.do?ym=YYYY-MM`提供；本次根据页面脚本定位，尚未实测其响应。年度列表接口为`findPreDrawYearAll.do`。

## 接入边界

1. 香港六合彩不能沿用本地120秒自动期次。应按该源的完整期号和下一期开奖时间预约，日期按香港时区解释；不能把本地试用期号自动映射到真实期号。
2. 适配器明确输出本项目协议的`period_no`、带时区`drawn_at`和`result.regular/special/digits`；核对期号、完成状态、1～49七个互异号码，再交给现有幂等发布与结算链路。
3. 来源配置保持服务端限制，仅未来实际接入时增加明确来源适配和允许域名。本次没有将第三方地址加入运行环境允许列表。
4. 波色、生肖、五行不是按号码永久固定。来源数组只描述当期开奖的七个号码，不能据此拼出全49号年度映射。现有年度属性版本及购买／期次快照继续保留。
5. 来源`colorEng`映射为1红、2绿、3蓝；同脚本另一个`colorCh`数组顺序不同，不应混用。生肖编码1鼠至12猪；五行为1金、2木、3水、4火、5土。

来源证据为该站[配置脚本](https://6hch.com/js/config.js?v=20256111657)、[历史脚本](https://6hch.com/js/kaihistory.js?v=20256111657)及[实时头部脚本](https://6hch.com/html/public/Hkheader.html)。本地原始响应保存在`.local/6hch-latest.json`、`6hch-history.json`，未存任何登录凭证。
