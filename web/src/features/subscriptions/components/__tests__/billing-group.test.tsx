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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { PLAN_FORM_DEFAULTS } from '../../lib/plan-form'
import { SubscriptionsMutateDrawer } from '../subscriptions-mutate-drawer'
import { SubscriptionsProvider } from '../subscriptions-provider'

beforeEach(() => {
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: String(url).includes('/group')
      ? { success: true, data: ['default', 'GPT-3', 'auto'] }
      : { success: true, data: [] },
  }))
  vi.spyOn(api, 'put').mockResolvedValue({ data: { success: true } })
})

it('selects multiple quota groups and saves them independently of upgrade group', async () => {
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SubscriptionsProvider>
        <SubscriptionsMutateDrawer
          open
          onOpenChange={() => {}}
          currentRow={{
            plan: {
              ...PLAN_FORM_DEFAULTS,
              id: 3,
              title: 'GPT plan',
              currency: 'USD',
              upgrade_group: 'default',
            },
          }}
        />
      </SubscriptionsProvider>
    </QueryClientProvider>
  )
  const input = screen.getByLabelText('Quota billing groups')
  expect(input).toHaveValue('')
  await user.click(input)
  await user.type(input, 'GPT-3')
  await screen.findByRole('option', { name: 'GPT-3' })
  await user.keyboard('{ArrowDown}{Enter}')
  await user.click(input)
  await user.click(await screen.findByRole('option', { name: 'default' }))
  expect(screen.queryByRole('option', { name: 'auto' })).not.toBeInTheDocument()
  expect(screen.getByRole('option', { name: 'GPT-3' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  expect(screen.getByRole('option', { name: 'default' })).toHaveAttribute(
    'aria-selected',
    'true'
  )
  await user.keyboard('{Escape}')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({
        plan: expect.objectContaining({
          billing_groups: ['GPT-3', 'default'],
          upgrade_group: 'default',
        }),
      })
    )
  )
  client.clear()
})

it('loads saved groups and lets the administrator clear the restriction', async () => {
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <SubscriptionsProvider>
        <SubscriptionsMutateDrawer
          open
          onOpenChange={() => {}}
          currentRow={{
            plan: {
              ...PLAN_FORM_DEFAULTS,
              id: 3,
              title: 'Shared plan',
              currency: 'USD',
              billing_groups: ['GPT-3', 'default'],
            },
          }}
        />
      </SubscriptionsProvider>
    </QueryClientProvider>
  )
  const input = screen.getByLabelText('Quota billing groups')
  await user.click(input)
  const listbox = await screen.findByRole('listbox')
  expect(
    within(listbox).getByRole('option', { name: 'GPT-3' })
  ).toHaveAttribute('aria-selected', 'true')
  expect(
    within(listbox).getByRole('option', { name: 'default' })
  ).toHaveAttribute('aria-selected', 'true')
  // 从已选择项中逐个取消，空数组明确表示解除限制。
  await user.click(within(listbox).getByRole('option', { name: 'GPT-3' }))
  await user.click(within(listbox).getByRole('option', { name: 'default' }))
  await user.keyboard('{Escape}')
  expect(input).toHaveAttribute(
    'placeholder',
    'Select groups (empty means unrestricted)'
  )
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({
        plan: expect.objectContaining({ billing_groups: [] }),
      })
    )
  )
  client.clear()
})
