package controller

import (
	"bytes"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionBillingGroupAPI(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "plans.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}))
	oldDB := model.DB
	model.DB = db
	oldPayment := *operation_setting.GetPaymentSetting()
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	oldGroups := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"GPT-3":1,"auto":1}`))
	t.Cleanup(func() {
		model.DB = oldDB
		*operation_setting.GetPaymentSetting() = oldPayment
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroups))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func callSubscriptionBillingGroupAPI(t *testing.T, id int, fields map[string]any) bool {
	t.Helper()
	plan := map[string]any{"title": "group-billing-plan", "price_amount": 0, "duration_unit": "month", "duration_value": 1, "enabled": true, "total_amount": 1000}
	maps.Copy(plan, fields)
	body, err := common.Marshal(map[string]any{"plan": plan})
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if id == 0 {
		AdminCreateSubscriptionPlan(ctx)
	} else {
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
		AdminUpdateSubscriptionPlan(ctx)
	}
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	return response.Success
}

func TestSubscriptionBillingGroupAPICreatesAndValidatesConcreteGroups(t *testing.T) {
	for _, tc := range []struct {
		group string
		valid bool
	}{{"GPT-3", true}, {"", true}, {" default ", true}, {"missing", false}, {"auto", false}} {
		t.Run(tc.group, func(t *testing.T) {
			setupSubscriptionBillingGroupAPI(t)
			assert.Equal(t, tc.valid, callSubscriptionBillingGroupAPI(t, 0, map[string]any{"billing_group": tc.group}))
			var count int64
			require.NoError(t, model.DB.Model(&model.SubscriptionPlan{}).Count(&count).Error)
			if tc.valid {
				assert.Equal(t, int64(1), count)
			} else {
				assert.Zero(t, count)
			}
		})
	}
}

func TestSubscriptionBillingGroupAPIUpdatesExistingPlanWithoutClearingOmittedField(t *testing.T) {
	for _, tc := range []struct {
		name    string
		include bool
		value   string
		want    string
		valid   bool
	}{
		{"old-client-omits", false, "", "GPT-3", true},
		{"set-target", true, "default", "default", true},
		{"clear-limit", true, "", "", true},
		{"invalid-does-not-save", true, "missing", "GPT-3", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSubscriptionBillingGroupAPI(t)
			plan := model.SubscriptionPlan{Id: 32001, Title: "existing", BillingGroup: common.GetPointer("GPT-3"), UpgradeGroup: "default"}
			require.NoError(t, model.DB.Create(&plan).Error)
			fields := map[string]any{}
			if tc.include {
				fields["billing_group"] = tc.value
			}
			assert.Equal(t, tc.valid, callSubscriptionBillingGroupAPI(t, plan.Id, fields))
			var saved model.SubscriptionPlan
			require.NoError(t, model.DB.First(&saved, plan.Id).Error)
			assert.Equal(t, tc.want, strings.Join(saved.GetBillingGroups(), ","))
		})
	}
}

// 覆盖新接口数组、旧接口字段和省略字段的兼容边界，失败时不能改写原范围。
func TestSubscriptionBillingGroupAPIUpdatesMultipleGroups(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields map[string]any
		valid  bool
		want   []string
	}{
		{"set-and-deduplicate", map[string]any{"billing_groups": []string{" GPT-3 ", "default", "GPT-3"}}, true, []string{"GPT-3", "default"}},
		{"omit-preserves", nil, true, []string{"GPT-3", "default"}},
		{"null-preserves", map[string]any{"billing_groups": nil}, true, []string{"GPT-3", "default"}},
		{"clear", map[string]any{"billing_groups": []string{}}, true, []string{}},
		{"array-takes-precedence", map[string]any{"billing_groups": []string{}, "billing_group": "GPT-3"}, true, []string{}},
		{"legacy-set", map[string]any{"billing_group": "default"}, true, []string{"default"}},
		{"legacy-clear", map[string]any{"billing_group": ""}, true, []string{}},
		{"unknown-group", map[string]any{"billing_groups": []string{"GPT-3", "missing"}}, false, []string{"GPT-3", "default"}},
		{"auto", map[string]any{"billing_groups": []string{"auto"}}, false, []string{"GPT-3", "default"}},
		{"empty-entry", map[string]any{"billing_groups": []string{" "}}, false, []string{"GPT-3", "default"}},
		{"wrong-type", map[string]any{"billing_groups": "GPT-3"}, false, []string{"GPT-3", "default"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSubscriptionBillingGroupAPI(t)
			// 创建和更新都走实际接口，创建返回的数据也须能表示多分组。
			require.True(t, callSubscriptionBillingGroupAPI(t, 0, map[string]any{"billing_groups": []string{"GPT-3", "default"}}))
			var plan model.SubscriptionPlan
			require.NoError(t, model.DB.First(&plan).Error)
			assert.Equal(t, tc.valid, callSubscriptionBillingGroupAPI(t, plan.Id, tc.fields))
			var saved model.SubscriptionPlan
			require.NoError(t, model.DB.First(&saved, plan.Id).Error)
			assert.Equal(t, tc.want, saved.GetBillingGroups())
			data, err := common.Marshal(saved)
			require.NoError(t, err)
			var response struct {
				Groups []string `json:"billing_groups"`
			}
			require.NoError(t, common.Unmarshal(data, &response))
			assert.Equal(t, tc.want, response.Groups)
		})
	}
}
