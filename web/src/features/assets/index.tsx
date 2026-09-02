import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FolderPlus, RefreshCw, Trash2, Upload } from 'lucide-react'
import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import {
  createAssetGroup,
  deleteAsset,
  deleteAssetGroup,
  getStorageInfo,
  listAssetGroups,
  listAssets,
  uploadAsset,
} from './api'
import { AssetCard, formatBytes } from './components/asset-card'
import type { Asset, AssetGroup, AssetType } from './types'

const assetTypes: AssetType[] = ['Image', 'Video', 'Audio']

export function Assets() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedGroup, setSelectedGroup] = useState('')
  const [groupName, setGroupName] = useState('')
  const [groupDescription, setGroupDescription] = useState('')
  const [assetType, setAssetType] = useState<AssetType>('Video')
  const [assetName, setAssetName] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const storageQuery = useQuery({ queryKey: ['asset-storage'], queryFn: getStorageInfo })
  const groupsQuery = useQuery({ queryKey: ['asset-groups'], queryFn: listAssetGroups })
  const assetsQuery = useQuery({
    queryKey: ['assets', selectedGroup],
    queryFn: () => listAssets(selectedGroup || undefined),
    enabled: groupsQuery.isSuccess,
  })
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ['asset-groups'] })
    void queryClient.invalidateQueries({ queryKey: ['assets'] })
  }
  const createGroupMutation = useMutation({
    mutationFn: () => createAssetGroup({ groupName: groupName.trim(), description: groupDescription.trim() }),
    onSuccess: () => {
      toast.success(t('Asset group created'))
      setGroupName('')
      setGroupDescription('')
      invalidate()
    },
    onError: (error) => toast.error(error.message),
  })
  const uploadMutation = useMutation({
    mutationFn: () => {
      if (!file) throw new Error(t('Choose a file first'))
      return uploadAsset({ file, groupId: selectedGroup, assetName: assetName.trim() || file.name, assetType })
    },
    onSuccess: () => {
      toast.success(t('Asset uploaded'))
      setFile(null)
      setAssetName('')
      if (fileInput.current) fileInput.current.value = ''
      invalidate()
    },
    onError: (error) => toast.error(error.message),
  })
  const deleteAssetMutation = useMutation({
    mutationFn: deleteAsset,
    onSuccess: () => { toast.success(t('Asset deleted')); invalidate() },
    onError: (error) => toast.error(error.message),
  })
  const deleteGroupMutation = useMutation({
    mutationFn: deleteAssetGroup,
    onSuccess: () => { toast.success(t('Asset group deleted')); setSelectedGroup(''); invalidate() },
    onError: (error) => toast.error(error.message),
  })
  const groups = groupsQuery.data ?? []
  const assets = assetsQuery.data ?? []
  const selected = groups.find((group) => group.groupId === selectedGroup)
  const selectableGroups = groups.filter(
    (group): group is AssetGroup & { groupId: string } => Boolean(group.groupId)
  )
  const maxMB = Math.round((storageQuery.data?.max_bytes ?? 0) / 1024 / 1024)
  const storageError = storageQuery.error instanceof Error ? storageQuery.error.message : ''
  const uploadDisabled = !selectedGroup || !file || uploadMutation.isPending
  const canCreateGroup = groupName.trim().length > 0 && !createGroupMutation.isPending
  let uploadHint = t('Storage limit is configured on the server')
  if (maxMB) uploadHint = t('Maximum {{size}} MB', { size: maxMB })
  if (file) uploadHint = `${file.name} · ${formatBytes(file.size)}`

  const providerSummary = useMemo(() => {
    if (groupsQuery.isError) return t('Mobile Cloud is not configured')
    if (groupsQuery.isLoading) return t('Loading provider asset groups')
    return t('{{count}} asset groups available', { count: groups.length })
  }, [groups.length, groupsQuery.isError, groupsQuery.isLoading, t])

  return (
    <SectionPageLayout fixedContent>
      <SectionPageLayout.Title>{t('Asset Library')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          onClick={() => invalidate()}
          disabled={groupsQuery.isFetching}
        >
          <RefreshCw className={groupsQuery.isFetching ? 'animate-spin' : ''} />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center gap-2 rounded-xl border p-3'>
            <span className='bg-primary/10 text-primary rounded-md px-2 py-1 text-sm font-medium'>Mobile Cloud</span>
            <span className='text-muted-foreground text-sm'>{providerSummary}</span>
            <span className='text-muted-foreground ml-auto text-xs'>
              {t('Runyuan integration pending upstream API')}
            </span>
          </div>

          {storageError ? (
            <div className='text-destructive rounded-xl border border-dashed p-4 text-sm'>
              {storageError}
            </div>
          ) : null}
          <div className='grid gap-4 lg:grid-cols-[280px_1fr]'>
            <Card size='sm'>
              <CardHeader>
                <CardTitle>{t('Asset groups')}</CardTitle>
              </CardHeader>
              <CardContent className='space-y-3'>
                <div className='space-y-1'>
                  <Label htmlFor='asset-group-select'>{t('Current group')}</Label>
                  <select
                    id='asset-group-select'
                    value={selectedGroup}
                    onChange={(event) => setSelectedGroup(event.target.value)}
                    className='border-input bg-background h-8 w-full rounded-lg border px-2 text-sm'
                  >
                    <option value=''>{t('All groups')}</option>
                    {selectableGroups.map((group) => (
                      <option key={group.groupId} value={group.groupId}>
                        {group.groupName || group.groupId}
                      </option>
                    ))}
                  </select>
                </div>
                <Input
                  value={groupName}
                  onChange={(event) => setGroupName(event.target.value)}
                  placeholder={t('New group name')}
                  aria-label={t('New group name')}
                />
                <Input
                  value={groupDescription}
                  onChange={(event) => setGroupDescription(event.target.value)}
                  placeholder={t('Description (optional)')}
                  aria-label={t('Description (optional)')}
                />
                <Button
                  className='w-full'
                  onClick={() => createGroupMutation.mutate()}
                  disabled={!canCreateGroup}
                >
                  <FolderPlus />
                  {t('Create group')}
                </Button>
                {selected ? (
                  <Button
                    variant='destructive'
                    className='w-full'
                    onClick={() => {
                      if (window.confirm(t('Delete this asset group?')))
                        deleteGroupMutation.mutate(selectedGroup)
                    }}
                    disabled={deleteGroupMutation.isPending}
                  >
                    <Trash2 />
                    {t('Delete group')}
                  </Button>
                ) : null}
              </CardContent>
            </Card>

            <div className='space-y-4'>
              <Card size='sm'>
                <CardHeader>
                  <CardTitle>{t('Upload local asset')}</CardTitle>
                </CardHeader>
                <CardContent className='grid gap-3 md:grid-cols-[1fr_180px_auto] md:items-end'>
                  <div className='space-y-1'>
                    <Label htmlFor='asset-file'>{t('File (image, video, or audio)')}</Label>
                    <Input
                      ref={fileInput}
                      id='asset-file'
                      type='file'
                      accept='image/*,video/*,audio/*'
                      onChange={(event) =>
                        setFile(event.target.files?.[0] ?? null)
                      }
                    />
                    <p className='text-muted-foreground text-xs'>
                      {uploadHint}
                    </p>
                  </div>
                  <div className='space-y-1'>
                    <Label htmlFor='asset-type'>{t('Asset type')}</Label>
                    <select
                      id='asset-type'
                      value={assetType}
                      onChange={(event) =>
                        setAssetType(event.target.value as AssetType)
                      }
                      className='border-input bg-background h-8 w-full rounded-lg border px-2 text-sm'
                    >
                      {assetTypes.map((type) => (
                        <option key={type} value={type}>
                          {type}
                        </option>
                      ))}
                    </select>
                  </div>
                  <Button
                    onClick={() => uploadMutation.mutate()}
                    disabled={uploadDisabled}
                  >
                    <Upload />
                    {uploadMutation.isPending ? t('Uploading…') : t('Upload')}
                  </Button>
                  <Input
                    className='md:col-span-2'
                    value={assetName}
                    onChange={(event) => setAssetName(event.target.value)}
                    placeholder={t('Display name (optional)')}
                    aria-label={t('Display name (optional)')}
                  />
                </CardContent>
              </Card>

              <div className='flex items-center justify-between'>
                <h2 className='text-base font-semibold'>
                  {selected ? selected.groupName : t('All assets')}
                </h2>
                <span className='text-muted-foreground text-sm'>{assets.length}</span>
              </div>
              {assetsQuery.isLoading ? (
                <div className='text-muted-foreground rounded-xl border border-dashed p-10 text-center'>
                  {t('Loading assets…')}
                </div>
              ) : null}
              {assetsQuery.isError ? (
                <div className='text-destructive rounded-xl border border-dashed p-10 text-center'>
                  {t('Unable to load assets')}
                </div>
              ) : null}
              {!assetsQuery.isLoading &&
              !assetsQuery.isError &&
              assets.length === 0 ? (
                <div className='text-muted-foreground rounded-xl border border-dashed p-10 text-center'>
                  {t('No assets yet. Upload the first one above.')}
                </div>
              ) : null}
              <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-3'>
                {assets.map((asset) => (
                  <AssetCard
                    key={asset.assetId ?? asset.assetUrl}
                    asset={asset}
                    onDelete={() => {
                      if (
                        asset.assetId &&
                        window.confirm(t('Delete this asset?'))
                      ) {
                        deleteAssetMutation.mutate(asset.assetId)
                      }
                    }}
                  />
                ))}
              </div>
            </div>
          </div>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
