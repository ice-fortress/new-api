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
import type { AnnouncementItem } from '../types'

const STORAGE_KEY = 'new-api.announcement-popups'
const MAX_PER_USER = 200

type PopupStore = Record<string, string[]>

export function announcementPopupKey(
  item: Pick<AnnouncementItem, 'id' | 'content' | 'publishDate'>
) {
  return `${item.id ?? ''}\n${item.publishDate ?? ''}\n${item.content}`
}

function readStore(): PopupStore {
  if (typeof localStorage === 'undefined') return {}
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {}
    }
    const store: PopupStore = {}
    for (const [userId, value] of Object.entries(parsed)) {
      if (!Array.isArray(value)) continue
      store[userId] = value.filter(
        (entry): entry is string => typeof entry === 'string'
      )
    }
    return store
  } catch {
    return {}
  }
}

export function isAnnouncementPopupRead(
  userId: number,
  item: AnnouncementItem
) {
  const keys = readStore()[String(userId)] ?? []
  return keys.includes(announcementPopupKey(item))
}

export function rememberAnnouncementPopup(
  userId: number,
  item: AnnouncementItem
) {
  const store = readStore()
  const userKey = String(userId)
  const fingerprint = announcementPopupKey(item)
  const current = store[userKey] ?? []
  if (current.includes(fingerprint)) return
  store[userKey] = [...current, fingerprint].slice(-MAX_PER_USER)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(store))
}

function publishTime(item: AnnouncementItem) {
  if (!item.publishDate) return Number.NaN
  return Date.parse(item.publishDate)
}

export function selectPopupAnnouncements(
  items: AnnouncementItem[],
  userId: number,
  now = Date.now()
) {
  return items
    .filter((item) => {
      if (item.popup !== true) return false
      const publishedAt = publishTime(item)
      if (Number.isNaN(publishedAt) || publishedAt > now) return false
      return !isAnnouncementPopupRead(userId, item)
    })
    .sort((left, right) => publishTime(right) - publishTime(left))
}
