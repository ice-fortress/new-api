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
import { beforeEach, describe, expect, test } from 'vitest'

import type { AnnouncementItem } from '../types'
import {
  rememberAnnouncementPopup,
  selectPopupAnnouncements,
} from './announcement-popup'

const now = Date.parse('2026-11-17T10:00:00.000Z')

function item(
  overrides: Partial<AnnouncementItem> & Pick<AnnouncementItem, 'content'>
): AnnouncementItem {
  return {
    id: 1,
    publishDate: '2026-11-17T09:00:00.000Z',
    popup: true,
    ...overrides,
  }
}

describe('selectPopupAnnouncements', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  test('keeps published popup announcements newest first', () => {
    const older = item({ id: 1, content: 'older' })
    const newer = item({
      id: 2,
      content: 'newer',
      publishDate: '2026-11-17T09:30:00.000Z',
    })
    const selected = selectPopupAnnouncements(
      [
        older,
        item({ id: 3, content: 'off', popup: false }),
        item({
          id: 4,
          content: 'later',
          publishDate: '2026-11-18T00:00:00.000Z',
        }),
        newer,
      ],
      7,
      now
    )

    expect(selected.map((entry) => entry.id)).toEqual([2, 1])
  })

  test('remembers one announcement without hiding the rest', () => {
    const first = item({ id: 1, content: 'first' })
    const second = item({ id: 2, content: 'second' })
    rememberAnnouncementPopup(7, first)

    expect(selectPopupAnnouncements([first, second], 7, now)).toEqual([second])
    expect(selectPopupAnnouncements([first, second], 8, now)).toEqual([
      first,
      second,
    ])
  })

  test('shows an announcement again after its content changes', () => {
    const original = item({ id: 1, content: 'original' })
    rememberAnnouncementPopup(7, original)

    expect(
      selectPopupAnnouncements([item({ id: 1, content: 'updated' })], 7, now)
    ).toHaveLength(1)
  })
})
