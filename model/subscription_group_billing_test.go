package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// billingGroupDatabase 仅连接一次性测试库；默认是真实 SQLite 文件。
func billingGroupDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	engine := os.Getenv("NEWAPI_BILLING_TEST_DB")
	dsn := os.Getenv("NEWAPI_BILLING_TEST_DSN")
	var dialector gorm.Dialector
	var dbType common.DatabaseType
	switch engine {
	case "mysql", "postgres":
		require.Contains(t, dsn, "newapi_group_billing_test", "只允许隔离测试数据库")
		if engine == "mysql" {
			dialector = mysqlMigrationDialector{Dialector: *mysql.Open(dsn).(*mysql.Dialector)}
			dbType = common.DatabaseTypeMySQL
		} else {
			dialector = postgresMigrationDialector{Dialector: *postgres.Open(dsn).(*postgres.Dialector)}
			dbType = common.DatabaseTypePostgreSQL
		}
	default:
		require.Empty(t, engine)
		dialector = sqlite.Open(filepath.Join(t.TempDir(), "billing.db") + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate")
		dbType = common.DatabaseTypeSQLite
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	oldDB, oldLog := DB, LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(dbType, dbType)
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLog
		common.SetDatabaseTypes(oldMainType, oldLogType)
		initCol()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.Migrator().DropTable(&SubscriptionPreConsumeRecord{}, &UserSubscription{}, &SubscriptionPlan{}))
	// 使用启动时的真实套餐迁移分支，不能用 AutoMigrate 代替 SQLite 专用迁移。
	if dbType == common.DatabaseTypeSQLite {
		require.NoError(t, ensureSubscriptionPlanTableSQLite())
	} else {
		require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
	}
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &SubscriptionPreConsumeRecord{}))
	var version string
	if engine == "" {
		require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
	} else {
		require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
	}
	t.Logf("database %s: %s", dbType, version)
	for _, id := range []int{1, 2, 3} {
		InvalidateSubscriptionPlanCache(id)
	}
	return db
}

func seedBillingGroupSubscriptions(t *testing.T) {
	t.Helper()
	now := GetDBTimestamp()
	for id, group := range map[int]string{1: "default", 2: "GPT-3"} {
		plan := SubscriptionPlan{Id: id, Title: group, TotalAmount: 1000, QuotaResetPeriod: SubscriptionResetNever, BillingGroup: common.GetPointer(group)}
		require.NoError(t, DB.Create(&plan).Error)
		sub := UserSubscription{Id: 100 + id, UserId: 71, PlanId: id, AmountTotal: 1000, StartTime: now - 100, EndTime: now + int64(id)*3600, Status: "active", AllowWalletOverflow: false}
		require.NoError(t, DB.Create(&sub).Error)
	}
}

func billingGroupUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub UserSubscription
	require.NoError(t, DB.First(&sub, id).Error)
	return sub.AmountUsed
}

func TestSubscriptionBillingGroupSelectsOnlyMatchingPlan(t *testing.T) {
	billingGroupDatabase(t)
	seedBillingGroupSubscriptions(t)
	got, err := PreConsumeUserSubscription("gpt", 71, "model", 0, 100, "GPT-3")
	require.NoError(t, err)
	assert.Equal(t, 102, got.UserSubscriptionId)
	assert.Equal(t, "GPT-3", got.BillingGroup)
	assert.Equal(t, int64(0), billingGroupUsed(t, 101))
	assert.Equal(t, int64(100), billingGroupUsed(t, 102))
	got, err = PreConsumeUserSubscription("base", 71, "model", 0, 50, "default")
	require.NoError(t, err)
	assert.Equal(t, 101, got.UserSubscriptionId)
	var subs []UserSubscription
	require.NoError(t, DB.Find(&subs).Error)
	for _, sub := range subs {
		assert.Empty(t, sub.UpgradeGroup, "不改写旧的账号升降组快照")
	}
}

func TestSubscriptionBillingGroupUsesUpdatedPlanForExistingSubscribers(t *testing.T) {
	billingGroupDatabase(t)
	seedBillingGroupSubscriptions(t)
	_, err := GetSubscriptionPlanById(1)
	require.NoError(t, err) // 缓存中仍是旧分组。
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 1).Update("billing_group", "GPT-3").Error)
	got, err := PreConsumeUserSubscription("updated", 71, "model", 0, 40, "GPT-3")
	require.NoError(t, err)
	assert.Equal(t, 101, got.UserSubscriptionId)
	_, err = PreConsumeUserSubscription("old-group", 71, "model", 0, 40, "default")
	require.ErrorContains(t, err, "no active subscription")
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 1).Update("billing_group", nil).Error)
	got, err = PreConsumeUserSubscription("unrestricted", 71, "model", 0, 20, "another-group")
	require.NoError(t, err)
	assert.Equal(t, 101, got.UserSubscriptionId)
	assert.Empty(t, got.BillingGroup)
}

func TestSubscriptionBillingGroupInsufficientQuotaDoesNotUseOtherGroup(t *testing.T) {
	billingGroupDatabase(t)
	seedBillingGroupSubscriptions(t)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", 102).Update("amount_used", 990).Error)
	_, err := PreConsumeUserSubscription("insufficient", 71, "model", 0, 20, "GPT-3")
	require.ErrorContains(t, err, "subscription quota insufficient")
	assert.Zero(t, billingGroupUsed(t, 101))
	assert.Equal(t, int64(990), billingGroupUsed(t, 102))
	var count int64
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Count(&count).Error)
	assert.Zero(t, count)
	group := "absent"
	found, err := HasActiveUserSubscription(71, &group)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSubscriptionBillingGroupResetsOnlyEligibleQuota(t *testing.T) {
	billingGroupDatabase(t)
	seedBillingGroupSubscriptions(t)
	now := GetDBTimestamp()
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id IN ?", []int{1, 2}).Update("quota_reset_period", SubscriptionResetDaily).Error)
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", 71).Updates(map[string]any{"amount_used": 999, "last_reset_time": now - 86400, "next_reset_time": now - 1}).Error)
	_, err := PreConsumeUserSubscription("reset", 71, "model", 0, 20, "GPT-3")
	require.NoError(t, err)
	assert.Equal(t, int64(999), billingGroupUsed(t, 101))
	assert.Equal(t, int64(20), billingGroupUsed(t, 102))
}

func TestSubscriptionBillingGroupIdempotencyAndRefundStayOnOriginalSubscription(t *testing.T) {
	billingGroupDatabase(t)
	seedBillingGroupSubscriptions(t)
	got, err := PreConsumeUserSubscription("repeat", 71, "model", 0, 60, "GPT-3")
	require.NoError(t, err)
	again, err := PreConsumeUserSubscription("repeat", 71, "model", 0, 60, "GPT-3")
	require.NoError(t, err)
	assert.Equal(t, got.UserSubscriptionId, again.UserSubscriptionId)
	_, err = PreConsumeUserSubscription("repeat", 71, "model", 0, 60, "default")
	require.ErrorIs(t, err, ErrSubscriptionScopeMismatch)
	_, err = PreConsumeUserSubscription("repeat", 72, "model", 0, 60, "GPT-3")
	require.ErrorIs(t, err, ErrSubscriptionScopeMismatch)
	assert.Equal(t, int64(60), billingGroupUsed(t, 102))
	// 套餐设置发生变化，退款仍按预扣记录找到原订阅。
	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", 2).Update("billing_group", "default").Error)
	require.NoError(t, RefundSubscriptionPreConsume("repeat"))
	require.NoError(t, RefundSubscriptionPreConsume("repeat"))
	assert.Zero(t, billingGroupUsed(t, 102))
	assert.Zero(t, billingGroupUsed(t, 101))
	_, err = PreConsumeUserSubscription("repeat", 71, "model", 0, 60, "default")
	require.ErrorContains(t, err, "already refunded")
}

func TestSubscriptionBillingGroupConcurrentReservations(t *testing.T) {
	for _, sameRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_request_%t", sameRequest), func(t *testing.T) {
			billingGroupDatabase(t)
			seedBillingGroupSubscriptions(t)
			require.NoError(t, DB.Model(&UserSubscription{}).Where("id = ?", 102).Update("amount_total", 100).Error)
			start := make(chan struct{})
			results := make(chan error, 2)
			var wg sync.WaitGroup
			for i := range 2 {
				wg.Go(func() {
					<-start
					id := fmt.Sprintf("concurrent-%d", i)
					if sameRequest {
						id = "same-request"
					}
					_, err := PreConsumeUserSubscription(id, 71, "model", 0, 60, "GPT-3")
					results <- err
				})
			}
			close(start)
			wg.Wait()
			close(results)
			succeeded := 0
			for err := range results {
				if err == nil {
					succeeded++
				} else {
					assert.Contains(t, err.Error(), "subscription quota insufficient")
				}
			}
			if sameRequest {
				assert.Equal(t, 2, succeeded)
			} else {
				assert.Equal(t, 1, succeeded)
			}
			assert.Equal(t, int64(60), billingGroupUsed(t, 102))
			assert.Zero(t, billingGroupUsed(t, 101))
		})
	}
}

// releasedSubscriptionPlan 是生产 rc.36 / ea7cb0b 的套餐结构，用于真实增量迁移验证。
type releasedSubscriptionPlan struct {
	Id int `json:"id"`

	Title    string `json:"title" gorm:"type:varchar(128);not null"`
	Subtitle string `json:"subtitle" gorm:"type:varchar(255);default:''"`

	// Display money amount (follow existing code style: float64 for money)
	PriceAmount float64 `json:"price_amount" gorm:"type:decimal(10,6);not null;default:0"`
	Currency    string  `json:"currency" gorm:"type:varchar(8);not null;default:'USD'"`

	DurationUnit  string `json:"duration_unit" gorm:"type:varchar(16);not null;default:'month'"`
	DurationValue int    `json:"duration_value" gorm:"type:int;not null;default:1"`
	CustomSeconds int64  `json:"custom_seconds" gorm:"type:bigint;not null;default:0"`

	Enabled   bool `json:"enabled" gorm:"default:true"`
	SortOrder int  `json:"sort_order" gorm:"type:int;default:0"`

	AllowBalancePay *bool `json:"allow_balance_pay"`

	// Allow falling back to wallet balance after subscription quota is exhausted (empty = true)
	AllowWalletOverflow *bool `json:"allow_wallet_overflow"`

	StripePriceId         string `json:"stripe_price_id" gorm:"type:varchar(128);default:''"`
	CreemProductId        string `json:"creem_product_id" gorm:"type:varchar(128);default:''"`
	WaffoPancakeProductId string `json:"waffo_pancake_product_id" gorm:"type:varchar(128);default:''"`

	// Max purchases per user (0 = unlimited)
	MaxPurchasePerUser int `json:"max_purchase_per_user" gorm:"type:int;default:0"`

	// Upgrade user group after purchase (empty = no change)
	UpgradeGroup string `json:"upgrade_group" gorm:"type:varchar(64);default:''"`

	// Downgrade user group on expiry (empty = revert to the group held before purchase)
	DowngradeGroup string `json:"downgrade_group" gorm:"type:varchar(64);default:''"`

	// Total quota (amount in quota units, 0 = unlimited)
	TotalAmount int64 `json:"total_amount" gorm:"type:bigint;not null;default:0"`

	// Quota reset period for plan
	QuotaResetPeriod        string `json:"quota_reset_period" gorm:"type:varchar(16);default:'never'"`
	QuotaResetCustomSeconds int64  `json:"quota_reset_custom_seconds" gorm:"type:bigint;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint"`
}

func (releasedSubscriptionPlan) TableName() string { return "subscription_plans" }

func TestSubscriptionBillingGroupMigrationPreservesReleasedData(t *testing.T) {
	db := billingGroupDatabase(t)
	require.NoError(t, db.Migrator().DropTable(&SubscriptionPlan{}))
	require.NoError(t, db.AutoMigrate(&releasedSubscriptionPlan{}))
	require.NoError(t, db.Create(&releasedSubscriptionPlan{Id: 3, Title: "existing-plan", TotalAmount: 150000000, UpgradeGroup: "GPT-3"}).Error)
	require.NoError(t, db.Create(&UserSubscription{Id: 14, UserId: 2, PlanId: 3, AmountTotal: 150000000, AmountUsed: 1234, Status: "active", EndTime: time.Now().Add(time.Hour).Unix()}).Error)
	assert.False(t, db.Migrator().HasColumn(&SubscriptionPlan{}, "billing_group"))
	recorder := &migrationSQLRecorder{}
	db = db.Session(&gorm.Session{Logger: recorder})
	DB = db
	for i := range 2 {
		recorder.reset()
		if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
			require.NoError(t, ensureSubscriptionPlanTableSQLite())
		} else {
			require.NoError(t, db.AutoMigrate(&SubscriptionPlan{}))
		}
		if i == 1 {
			assert.Empty(t, recorder.schemaMutations(), "重复迁移不得改写表结构")
		}
	}
	var plan SubscriptionPlan
	require.NoError(t, db.First(&plan, 3).Error)
	assert.Equal(t, "existing-plan", plan.Title)
	assert.Equal(t, "GPT-3", plan.UpgradeGroup)
	assert.Equal(t, int64(150000000), plan.TotalAmount)
	assert.Nil(t, plan.BillingGroup)
	assert.Equal(t, int64(1234), billingGroupUsed(t, 14))
	var sub UserSubscription
	require.NoError(t, db.First(&sub, 14).Error)
	assert.Empty(t, sub.UpgradeGroup)
	// 原主键唯一性和已有账户引用继续有效。
	err := db.Create(&SubscriptionPlan{Id: 3, Title: "duplicate"}).Error
	require.Error(t, err)
	var columns []gorm.ColumnType
	columns, err = db.Migrator().ColumnTypes(&SubscriptionPlan{})
	require.NoError(t, err)
	n := 0
	for _, column := range columns {
		if strings.EqualFold(column.Name(), "billing_group") {
			n++
		}
	}
	assert.Equal(t, 1, n)
}
