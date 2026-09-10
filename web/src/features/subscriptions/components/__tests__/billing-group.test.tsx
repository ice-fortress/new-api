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
import { render, screen, waitFor } from '@testing-library/react'
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

it('lets an administrator select a concrete quota group and saves it independently of upgrade group', async () => {
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
  const input = screen.getByRole('combobox', { name: 'Quota billing group' })
  expect(input).toHaveValue('Unrestricted')
  await user.click(input)
  await user.click(await screen.findByRole('option', { name: 'GPT-3' }))
  expect(input).toHaveValue('GPT-3')
  await user.click(screen.getByRole('button', { name: 'Save changes' }))
  await waitFor(() =>
    expect(api.put).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({
        plan: expect.objectContaining({
          billing_group: 'GPT-3',
          upgrade_group: 'default',
        }),
      })
    )
  )
  client.clear()
})
