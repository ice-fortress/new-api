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
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { statusQueryOptions } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

import { useAnnouncements } from '../../hooks/use-status-data'
import {
  rememberAnnouncementPopup,
  selectPopupAnnouncements,
} from '../../lib/announcement-popup'
import type { AnnouncementItem } from '../../types'

export function AnnouncementPopup() {
  const { t } = useTranslation()
  const userId = useAuthStore((state) => state.auth.user?.id)
  const statusQuery = useQuery(statusQueryOptions)
  const { items } = useAnnouncements()
  const [queue, setQueue] = useState<AnnouncementItem[] | null>(null)
  const [index, setIndex] = useState(0)

  useEffect(() => {
    if (!statusQuery.isSuccess || !userId || queue) return
    setQueue(selectPopupAnnouncements(items, userId))
  }, [items, queue, statusQuery.isSuccess, userId])

  const current = queue?.[index]
  if (!userId || !current || !queue) return null

  const total = queue.length
  const isLast = index === total - 1

  const advance = () => {
    rememberAnnouncementPopup(userId, current)
    setIndex((value) => value + 1)
  }

  return (
    <Dialog
      open
      disablePointerDismissal
      showCloseButton={false}
      onOpenChange={(next, details) => {
        if (!next) details.cancel()
      }}
      title={t('Announcements')}
      description={t('Announcement {{current}} of {{total}}', {
        current: index + 1,
        total,
      })}
      contentClassName='sm:max-w-lg'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <div className='flex w-full items-center justify-between gap-3'>
          <span className='text-muted-foreground text-sm tabular-nums'>
            {index + 1}/{total}
          </span>
          <Button type='button' onClick={advance}>
            {isLast ? t('Done') : t('View next announcement')}
          </Button>
        </div>
      }
    >
      <ScrollArea className='max-h-[min(58vh,520px)] pr-4'>
        <div className='space-y-4'>
          {current.content && <RichContent breaks content={current.content} />}
          {current.extra && (
            <RichContent
              breaks
              content={current.extra}
              className='text-muted-foreground'
            />
          )}
        </div>
      </ScrollArea>
    </Dialog>
  )
}
