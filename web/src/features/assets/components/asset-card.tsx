import { FileAudio, FileImage, FileVideo, Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

import type { Asset } from '../types'

export function AssetCard(props: { asset: Asset; onDelete: () => void }) {
  const { t } = useTranslation()
  const asset = props.asset
  const url = typeof asset.assetUrl === 'string' ? asset.assetUrl : ''
  const type = asset.assetType?.toLowerCase()
  let preview: ReactNode = (
    <FileVideo className='text-muted-foreground size-10' aria-hidden='true' />
  )
  if (type === 'image' && url) {
    preview = (
      <img
        src={url}
        alt={asset.assetName || t('Asset preview')}
        loading='lazy'
        className='h-full w-full object-cover'
      />
    )
  }
  if (type === 'video' && url) {
    preview = (
      <video
        src={url}
        controls
        preload='metadata'
        className='h-full w-full object-cover'
        aria-label={asset.assetName || t('Video preview')}
      />
    )
  }
  if (type === 'audio' && url) {
    preview = (
      <audio
        src={url}
        controls
        preload='metadata'
        className='w-full px-2'
        aria-label={asset.assetName || t('Audio preview')}
      />
    )
  }
  if (type === 'audio' && !url) {
    preview = (
      <FileAudio className='text-muted-foreground size-10' aria-hidden='true' />
    )
  }
  if (type === 'image' && !url) {
    preview = (
      <FileImage className='text-muted-foreground size-10' aria-hidden='true' />
    )
  }
  return (
    <Card size='sm' className='min-w-0'>
      <div className='bg-muted/40 flex aspect-video items-center justify-center overflow-hidden'>
        {preview}
      </div>
      <CardContent className='flex items-start gap-2'>
        <div className='min-w-0 flex-1'>
          <p className='truncate font-medium'>
            {asset.assetName || t('Unnamed asset')}
          </p>
          <p className='text-muted-foreground truncate text-xs'>
            {asset.status || asset.assetType || t('Unknown')}
          </p>
        </div>
        {asset.assetId ? (
          <Button
            variant='ghost'
            size='icon-sm'
            aria-label={t('Delete asset')}
            onClick={props.onDelete}
          >
            <Trash2 className='text-destructive' />
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function formatBytes(value: number) {
  if (value < 1024 * 1024) return `${Math.ceil(value / 1024)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}
