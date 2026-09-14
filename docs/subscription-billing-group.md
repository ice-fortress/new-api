# 订阅套餐的额度适用分组

本分支从上游 `main` 的 `bdef117505247769268b209665fb3ad7554c3da7` 建立，新增套餐级额度范围，解决请求使用 GPT-3 分组却扣到另一分组套餐的问题。现支持一个套餐选择多个适用分组，所选分组共用同一份订阅额度。

## 配置与行为

在「订阅管理 → 新建/编辑套餐」设置「额度适用分组」：

| 套餐 | 示例配置 | 可抵扣的请求 |
| --- | --- | --- |
| GPT 系列 | GPT-3、GPT-4 | 实际路由分组为 GPT-3 或 GPT-4 的请求，共用套餐额度 |
| 通用套餐 | default | 实际路由分组为 default 的请求 |
| 需要全组共享额度的套餐 | 不限制 | 任意分组的请求 |

该设置独立于「升级分组」「降级分组」，不会变更用户账号所属分组，也不会改写旧订阅的 `upgrade_group`。用户仍使用各分组对应的 API key；套餐范围不授予用户或令牌额外的分组访问权限。

- 规则以套餐当前配置为准，对该套餐全部订阅的后续请求生效；已经开始的请求按预扣时选中的订阅和分组继续结算或退款。
- 在所有符合分组条件的有效订阅中，继续按到期时间、订阅 ID 排序，选择能覆盖预扣额度的一份。同组多个套餐及“不限制”套餐仍遵循这个顺序，并非具体分组套餐优先。
- 要让 GPT-3 与通用额度完全隔离，需要分别将两个套餐设置成 GPT-3 和 default。如果通用套餐仍为“不限制”，它仍可抵扣 GPT-3 请求。
- 匹配套餐额度不足时，不使用不匹配分组的套餐。只有适用订阅都允许钱包回退时，`subscription_first` 才按原机制回退；有订阅但没有适用订阅时明确失败。用户显式选择 `wallet_only` / `wallet_first` 时保留钱包偏好，完全没有有效订阅时也保留原钱包逻辑。
- 受限套餐预扣成功后，固定本次请求的实际分组，允许组内渠道重试；即使另一分组也在套餐所选范围内，尝试换组仍会在请求上游前被拒绝，预扣费退回原订阅。这里不提供跨组切换套餐的自动结算功能。
- 此设置限制的是套餐额度用途；分组的渠道/模型权限仍由原有访问控制管理。

## 数据库和接口兼容

`subscription_plans` 保留旧版 nullable `billing_group` 字段，并新增 TEXT 类型的 `billing_groups` JSON 数组。新列表非 NULL 时优先使用，`[]` 表示“不限制”；列表为 NULL 时继续读取原单分组配置。升级无需改写旧套餐、订阅额度或账目。

`subscription_pre_consume_records` 新增 nullable `billing_group`，记录预扣时固定的实际分组，避免同一请求 ID 换组复用预扣。空字符串保留不受限套餐的原有行为。旧记录没有快照，若对应套餐已经改成多分组，不能可靠确定原分组时拒绝复用，但仍可正常退款。

- SQLite 的专用建表/补列流程和 MySQL/PostgreSQL 的 AutoMigrate 都包含此字段；迁移不清空用户、订阅、已用额度或旧分组快照。
- 套餐创建/更新接口使用 `billing_groups: ["GPT-3", "GPT-4"]`，逐项校验分组存在、去除首尾空格并去重，拒绝空白项和 `auto` 这种未解析分组。
- 更新套餐时省略分组字段或传 `billing_groups: null`，保留原限制；显式传 `billing_groups: []` 解除限制。仍接受旧客户端的 `billing_group` 字符串输入并转换为列表，旧字段空字符串表示解除限制；同时提供两种字段时以非 NULL 列表为准。使用现有完整套餐更新接口，勿把它当作部分 PATCH 接口调用。
- 每次预扣从数据库读取该用户有效订阅对应的套餐范围，在 Go 中精确匹配分组，不使用数据库专用 JSON 查询，也不因套餐详情缓存滞后继续跨组扣费。
- 退款的额度调整与幂等记录在同一个事务提交，修正旧嵌套事务在 SQLite 下的锁冲突，并防止已退额度与退款记录状态分离。
- 历史扣款不自动重分配；生产账目纠正需另行核对。

## 本地验证（2026-09-10）

环境：Go 1.26.2 / Bun 1.4.0；所有测试使用本机临时数据，没有连接 Fedora 数据库。

| 检查 | 结果 |
| --- | --- |
| SQLite 3.50.4：选择、配置变更、额度不足、周期重置、重复请求、并发与退款 | 通过 |
| MySQL 8.4.11：相同定向测试，使用生产 MySQL 方言适配器 | 通过 |
| PostgreSQL 15.18：相同定向测试，使用生产 PostgreSQL 方言适配器 | 通过 |
| 从 rc.36 套餐结构迁移；三库重复迁移两次、第二次无结构变更，旧套餐/订阅/已用额度及主键保留 | 通过 |
| 套餐 API：创建、目标组校验、旧客户端省略字段、解除限制 | 通过 |
| 资金来源：钱包回退、跨组拒绝、配置变更后的原订阅结算和退款 | 通过 |
| Go race：新增计费与资金来源回归 | 通过 |

自动化检查命令：

```sh
# 默认使用独立 SQLite 文件；不会使用应用的生产数据库配置。
GOWORK=off go test ./model ./service ./controller -run 'TestSubscription(BillingGroup|GroupBilling)' -count=1
GOWORK=off go test -race ./model ./service -run 'TestSubscription(BillingGroup|GroupBilling)' -count=1
make test
GOWORK=off go vet ./...

# 为一次性测试实例配置 DSN；数据库名称必须包含 newapi_group_billing_test。
NEWAPI_BILLING_TEST_DB=mysql NEWAPI_BILLING_TEST_DSN="$MYSQL_TEST_DSN" GOWORK=off go test ./model -run TestSubscriptionBillingGroup -count=1 -v
NEWAPI_BILLING_TEST_DB=postgres NEWAPI_BILLING_TEST_DSN="$POSTGRES_TEST_DSN" GOWORK=off go test ./model -run TestSubscriptionBillingGroup -count=1 -v

# 前端（在 web 目录运行）
bun run test src/features/subscriptions/lib/__tests__/billing-group.test.ts src/features/subscriptions/components/__tests__/billing-group.test.tsx
bun run typecheck
bun run build
```

前端复用现有套餐抽屉和 Combobox，增加中/英等现有七种语言文案；定向测试覆盖表单载入、选择、保存、解除限制的数据载荷和中文翻译。修改的前端文件另经 oxlint、oxfmt 检查。页面在本机隔离实例完成实际新建、保存、重启后重新编辑及 390px 手机宽度的中文显示验收，数据库与生产无关。


## 多分组扩展验证（2026-09-14）

复用 `MultiSelect` 多选组件；七种语言文案已同步。此次实际检查结果：

| 检查 | 结果 |
| --- | --- |
| SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 15.18 | 多组共享额度、精确匹配、并发、幂等、退款均通过 |
| rc.37 原始结构及本分支旧单分组结构升级 | 三库均通过；连续迁移两次，第二次无结构变更，历史额度、配置、主键和请求 ID 唯一约束保留 |
| 套餐 API | 多选保存、去重、旧字段兼容、清空和无效分组拒绝通过 |
| 请求资金来源 | 已选分组之间的跨组重试拒绝、原订阅结算退款、钱包回退通过 |
| `GOWORK=off go test ./model ./service ./controller` | 完整受影响包测试通过 |
| Go race 定向测试（命令见上文） | 通过 |
| 前端定向测试、typecheck、涉及文件 oxlint/oxfmt、build | 通过；前端定向测试 8 个用例 |

数据库定向验证沿用上文命令，MySQL 与 PostgreSQL 分别使用临时容器 `mysql:8.4`、`postgres:15`，端口为 `127.0.0.1:13316` 和 `127.0.0.1:15436`，数据库名均为 `newapi_group_billing_test`；测试后清理容器。本次未改变独立日志数据库路径。

新前后端需一同升级。旧单分组版本不读取 `billing_groups`，回退前须将多分组配置转换为旧版本能够表达的单分组限制，不能直接依赖旧版本解释新配置。

## 同步上游 main（2026-09-14）

功能提交 `5cf832a3a` 后，合并上游 `7fd063819`，共纳入 23 个提交。唯一内容冲突位于 `BillingSession.Reserve`：保留订阅固定分组校验，并接入上游图片请求追加预扣逻辑。跨组拒绝和退款测试同时覆盖普通请求与图片请求。

合并后的验证记录：

- `GOFLAGS=-p=2 make test`：根 Go 模块及 relaykit 全量测试通过。
- `cd relaykit && GOWORK=off go build ./...`、根目录 `GOWORK=off go build -o /tmp/newapi-main-sync-check .`：独立模块及主程序构建通过。
- `bun run typecheck`、`bun run build`：通过；`bun install --frozen-lockfile` 确认上游依赖已安装，`bun run i18n:sync` 后无额外差异。
- `bun run test` 覆盖 134 个文件、1487 个用例。首次高并发运行出现超时及可见性断言失败；失败文件以 `--maxWorkers=2` 复测，剩余设置引导用例以 `--maxWorkers=1` 单独运行后通过。纯上游对照用例也通过，没有为此修改上游 UI 或测试断言。
- SQLite 3.50.4、MySQL 8.4.11、PostgreSQL 15.18：订阅多分组及升级迁移定向测试通过，命令沿用上文 `NEWAPI_BILLING_TEST_DB` / `NEWAPI_BILLING_TEST_DSN` 的两种外部数据库配置。
- 设置临时数据库的 `TEST_MYSQL_DSN` / `TEST_POSTGRES_DSN` 后，`GOWORK=off go test ./model -run 'TestMigrationSchemaStability|TestMigratePrefillGroupUniqueness' -count=1 -v` 通过，覆盖上游迁移适配器、旧唯一约束和预填分组索引迁移。
- 对独立日志库 `newapi_group_billing_log_test`，临时 Go overlay 仅将迁移套件的 `chooseDB(..., false)` 改为 `chooseDB(..., true)`；`GOWORK=off go test -overlay=/tmp/newapi-main-sync-log-overlay.json ./model -run TestMigrationSchemaStability -count=1 -v` 在三种数据库上通过。overlay 未写入仓库。

验证仅使用本地临时数据库；未推送分支或执行生产部署。

## 后续部署与升级

本轮仅完成本地修复，不执行 Fedora 切换。部署前需要备份并验证现有数据库与配置；构建包含本分支提交的自定义镜像，沿用 Fedora 的 `.env`、Compose 项目、PostgreSQL 卷及应用数据挂载，再分别配置各套餐的目标分组。

这次基线按用户要求使用最新 main，包含 rc.36 之后的上游更改。迁移验证覆盖本补丁涉及的套餐表；正式升级前还应审查对应上游升级内容和生产备份恢复方案。不要将本地测试解释为已完成生产升级验收。

后续升级将本补丁应用到新的上游版本，重新跑上述计费和迁移测试。使用官方镜像不会包含本补丁；保留自定义镜像版本和对应源码提交。回退时同时考虑上游自身的数据库迁移兼容性。
