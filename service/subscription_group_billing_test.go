package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSubscriptionGroupBilling 使用包内隔离 SQLite，明确初始化套餐、用户和订阅。
func setupSubscriptionGroupBilling(t *testing.T) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	user := model.User{Id: 71001, Username: "subscription-billing-test", Quota: 10000}
	require.NoError(t, model.DB.Create(&user).Error)
	now := time.Now().Unix()
	for id, group := range map[int]string{71001: "default", 71002: "GPT-3"} {
		model.InvalidateSubscriptionPlanCache(id)
		require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: id, Title: group, BillingGroup: common.GetPointer(group), QuotaResetPeriod: model.SubscriptionResetNever}).Error)
		require.NoError(t, model.DB.Create(&model.UserSubscription{Id: id, UserId: user.Id, PlanId: id, AmountTotal: 1000, Status: "active", StartTime: now - 100, EndTime: now + int64(id-71000)*3600, AllowWalletOverflow: false}).Error)
	}
	t.Cleanup(func() {
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).Delete(&model.SubscriptionPreConsumeRecord{}).Error)
		require.NoError(t, model.DB.Where("user_id = ?", user.Id).Delete(&model.UserSubscription{}).Error)
		require.NoError(t, model.DB.Where("id IN ?", []int{71001, 71002}).Delete(&model.SubscriptionPlan{}).Error)
		require.NoError(t, model.DB.Unscoped().Delete(&user).Error)
		for _, id := range []int{71001, 71002} {
			model.InvalidateSubscriptionPlanCache(id)
		}
	})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{RequestId: t.Name(), UserId: user.Id, UserGroup: "default", UsingGroup: "GPT-3", OriginModelName: "test-model", IsPlayground: true}
	info.UserSetting.BillingPreference = "subscription_only"
	return ctx, info
}

func subscriptionGroupUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&sub, id).Error)
	return sub.AmountUsed
}

func TestSubscriptionGroupBillingSettlementKeepsSelectedSubscriptionAfterPlanEdit(t *testing.T) {
	ctx, info := setupSubscriptionGroupBilling(t)
	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	assert.Equal(t, 71002, info.SubscriptionId)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 71002).Update("billing_group", "default").Error)
	require.NoError(t, session.Reserve(150))
	require.NoError(t, session.Settle(125))
	require.NoError(t, session.Settle(125))
	assert.Equal(t, int64(125), subscriptionGroupUsed(t, 71002))
	assert.Zero(t, subscriptionGroupUsed(t, 71001))
	next := *info
	next.RequestId += "-next"
	next.Billing = nil
	_, apiErr = NewBillingSession(ctx, &next, 100)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
}

func TestSubscriptionGroupBillingCrossGroupRetryIsRejectedAndRefundsOriginal(t *testing.T) {
	ctx, info := setupSubscriptionGroupBilling(t)
	session, apiErr := NewBillingSession(ctx, info, 100)
	require.Nil(t, apiErr)
	info.Billing = session
	require.NoError(t, session.Reserve(150))
	assert.Nil(t, ValidateSubscriptionBillingGroup(info))
	info.UsingGroup = "default"
	require.NotNil(t, ValidateSubscriptionBillingGroup(info))
	require.Error(t, session.Reserve(200))
	require.Error(t, session.Settle(50))
	session.Refund(ctx)
	session.Refund(ctx)
	require.Eventually(t, func() bool {
		var sub model.UserSubscription
		return model.DB.First(&sub, 71002).Error == nil && sub.AmountUsed == 0
	}, time.Second, 5*time.Millisecond)
	assert.Zero(t, subscriptionGroupUsed(t, 71001))
}

func TestSubscriptionGroupBillingWalletFallbackUsesOnlyEligiblePlans(t *testing.T) {
	for _, allowOverflow := range []bool{false, true} {
		name := "blocked"
		if allowOverflow {
			name = "allowed"
		}
		t.Run(name, func(t *testing.T) {
			ctx, info := setupSubscriptionGroupBilling(t)
			info.UserSetting.BillingPreference = "subscription_first"
			require.NoError(t, model.DB.Model(&model.UserSubscription{}).Where("id = ?", 71002).Updates(map[string]any{"amount_used": 990, "allow_wallet_overflow": allowOverflow}).Error)
			session, apiErr := NewBillingSession(ctx, info, 20)
			if !allowOverflow {
				require.NotNil(t, apiErr)
				assert.Nil(t, session)
			} else {
				require.Nil(t, apiErr)
				assert.Equal(t, BillingSourceWallet, info.BillingSource)
				require.NoError(t, session.Settle(20))
			}
			assert.Zero(t, subscriptionGroupUsed(t, 71001))
			assert.Equal(t, int64(990), subscriptionGroupUsed(t, 71002))
		})
	}
}

func TestSubscriptionGroupBillingMissingMatchCannotConsumeAnotherPlan(t *testing.T) {
	for _, pref := range []string{"subscription_first", "subscription_only", "wallet_only", "wallet_first"} {
		t.Run(pref, func(t *testing.T) {
			ctx, info := setupSubscriptionGroupBilling(t)
			info.UsingGroup = "another-group"
			info.UserSetting.BillingPreference = pref
			session, apiErr := NewBillingSession(ctx, info, 20)
			if pref == "wallet_only" || pref == "wallet_first" {
				require.Nil(t, apiErr)
				require.NoError(t, session.Settle(20))
				assert.Equal(t, BillingSourceWallet, info.BillingSource)
			} else {
				require.NotNil(t, apiErr)
				assert.Nil(t, session)
			}
			assert.Zero(t, subscriptionGroupUsed(t, 71001))
			assert.Zero(t, subscriptionGroupUsed(t, 71002))
		})
	}
}

func TestSubscriptionGroupBillingUnrestrictedPlanPreservesCrossGroupUse(t *testing.T) {
	ctx, info := setupSubscriptionGroupBilling(t)
	require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Where("id = ?", 71001).Update("billing_group", nil).Error)
	session, apiErr := NewBillingSession(ctx, info, 20)
	require.Nil(t, apiErr)
	info.Billing = session
	assert.Equal(t, 71001, info.SubscriptionId)
	info.UsingGroup = "another-group"
	assert.Nil(t, ValidateSubscriptionBillingGroup(info))
	require.NoError(t, session.Settle(15))
	assert.Equal(t, int64(15), subscriptionGroupUsed(t, 71001))
	assert.Zero(t, subscriptionGroupUsed(t, 71002))
}
