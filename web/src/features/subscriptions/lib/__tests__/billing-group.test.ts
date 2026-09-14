import i18next from 'i18next'
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, it } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { subscriptionPlanSchema } from '../../types'
import {
  formValuesToPlanPayload,
  planToFormValues,
  PLAN_FORM_DEFAULTS,
} from '../plan-form'

describe('subscription billing groups', () => {
  it.each([
    { billing_groups: ['GPT-3', 'GPT-4'], want: ['GPT-3', 'GPT-4'] },
    { billing_groups: null, billing_group: 'GPT-3', want: ['GPT-3'] },
    { billing_groups: undefined, billing_group: 'GPT-3', want: ['GPT-3'] },
    { billing_groups: [], billing_group: 'GPT-3', want: [] },
  ])('preserves the effective groups when editing a plan: %j', (testCase) => {
    const plan = subscriptionPlanSchema.parse({
      ...PLAN_FORM_DEFAULTS,
      id: 3,
      ...testCase,
    })
    const form = planToFormValues(plan)
    const payload = formValuesToPlanPayload(form)
    expect(payload.plan.billing_groups).toEqual(testCase.want)
    expect(payload.plan).not.toHaveProperty('billing_group')
  })
  it('sends an explicit empty group list for an unrestricted plan', () => {
    expect(
      formValuesToPlanPayload(PLAN_FORM_DEFAULTS).plan.billing_groups
    ).toEqual([])
  })
})

it('displays the billing group label and unrestricted option in Chinese', async () => {
  const instance = i18next.createInstance()
  await instance.init({ lng: 'zh', resources: { zh } })
  expect(instance.t('Quota billing groups')).toBe('额度适用分组')
  expect(instance.t('Unrestricted')).toBe('不限制')
})
