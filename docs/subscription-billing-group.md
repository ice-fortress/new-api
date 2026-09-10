# 订阅套餐的额度适用分组

本分支从上游 `main` 的 `bdef117505247769268b209665fb3ad7554c3da7` 建立，新增套餐级额度范围，解决请求使用 GPT-3 分组却扣到另一分组套餐的问题。

## 配置与行为

在「订阅管理 → 新建/编辑套餐」设置「额度适用分组」：

| 套餐 | 示例配置 | 可抵扣的请求 |
| --- | --- | --- |
| GPT-3 | GPT-3 | 实际路由分组为 GPT-3 的请求 |
| 通用套餐 | default | 实际路由分组为 default 的请求 |
| 需要全组共享额度的套餐 | 不限制 | 任意分组的请求 |

该设置独立于「升级分组」「降级分组」，不会变更用户账号所属分组，也不会改写旧订阅的 `upgrade_group`。

- 规则以套餐当前配置为准，对该套餐全部订阅的后续请求生效；已经开始的请求按预扣时选中的订阅和分组继续结算或退款。
- 在所有符合分组条件的有效订阅中，继续按到期时间、订阅 ID 排序，选择能覆盖预扣额度的一份。同组多个套餐及“不限制”套餐仍遵循这个顺序，并非具体分组套餐优先。
- 要让 GPT-3 与通用额度完全隔离，需要分别将两个套餐设置成 GPT-3 和 default。如果通用套餐仍为“不限制”，它仍可抵扣 GPT-3 请求。
- 匹配套餐额度不足时，不使用不匹配分组的套餐。只有适用订阅都允许钱包回退时，`subscription_first` 才按原机制回退；有订阅但没有适用订阅时明确失败。用户显式选择 `wallet_only` / `wallet_first` 时保留钱包偏好，完全没有有效订阅时也保留原钱包逻辑。
- 受限套餐预扣成功后，允许组内渠道重试；尝试换到其他分组会在请求上游前被拒绝，预扣费退回原订阅。这里不提供跨组切换套餐的自动结算功能。
- 此设置限制的是套餐额度用途；分组的渠道/模型权限仍由原有访问控制管理。

## 数据库和接口兼容

只给 `subscription_plans` 增加 nullable `billing_group` 字段。NULL 与空字符串均表示“不限制”，旧数据和旧行为保留，部署不会自动依据套餐标题或 `upgrade_group` 开启限制。

- SQLite 的专用建表/补列流程和 MySQL/PostgreSQL 的 AutoMigrate 都包含此字段；迁移不清空用户、订阅、已用额度或旧分组快照。
- 套餐创建/更新接口校验非空目标组必须存在，并拒绝 `auto` 这种未解析分组。
- 旧客户端更新套餐时省略 `billing_group`，保留原限制；只有显式传空字符串才解除限制。使用现有完整套餐更新接口，勿把它当作部分 PATCH 接口调用。
- 每次预扣从数据库读取套餐当前额度范围，不因套餐详情缓存滞后继续跨组扣费。
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

## 后续部署与升级

本轮仅完成本地修复，不执行 Fedora 切换。部署前需要备份并验证现有数据库与配置；构建包含本分支提交的自定义镜像，沿用 Fedora 的 `.env`、Compose 项目、PostgreSQL 卷及应用数据挂载，再分别配置各套餐的目标分组。

这次基线按用户要求使用最新 main，包含 rc.36 之后的上游更改。迁移验证覆盖本补丁涉及的套餐表；正式升级前还应审查对应上游升级内容和生产备份恢复方案。不要将本地测试解释为已完成生产升级验收。

后续升级将本补丁应用到新的上游版本，重新跑上述计费和迁移测试。使用官方镜像不会包含本补丁；保留自定义镜像版本和对应源码提交。回退时同时考虑上游自身的数据库迁移兼容性。
