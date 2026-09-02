import { api, type ApiRequestConfig } from '@/lib/api'

import type { Asset, AssetGroup, StorageInfo, UploadResult } from './types'
import { normalizeAssets, normalizeGroups } from './lib/normalize'

type ApiResponse<T> = {
  success: boolean
  message?: string
  data: T
}

const mutationConfig: ApiRequestConfig = {
  skipBusinessError: true,
  skipErrorHandler: true,
}

function requireSuccess<T>(response: ApiResponse<T>): T {
  if (!response.success) throw new Error(response.message || 'Request failed')
  return response.data
}

export async function getStorageInfo() {
  const response = await api.get<ApiResponse<StorageInfo>>(
    '/api/mobilecloud/storage'
  )
  return requireSuccess(response.data)
}

export async function listAssetGroups() {
  const response = await api.get<ApiResponse<unknown>>(
    '/api/mobilecloud/asset-groups',
    { params: { page: 1, page_size: 100 } }
  )
  return normalizeGroups(requireSuccess(response.data))
}

export async function createAssetGroup(input: {
  groupName: string
  description?: string
}) {
  const response = await api.post<ApiResponse<unknown>>(
    '/api/mobilecloud/asset-groups',
    { groupType: 'AIGC', ...input },
    mutationConfig
  )
  return requireSuccess(response.data)
}

export async function deleteAssetGroup(groupId: string) {
  const response = await api.delete<ApiResponse<unknown>>(
    `/api/mobilecloud/asset-groups/${encodeURIComponent(groupId)}`,
    mutationConfig
  )
  return requireSuccess(response.data)
}

export async function listAssets(groupId?: string) {
  const response = await api.get<ApiResponse<unknown>>('/api/mobilecloud/assets', {
    params: { page: 1, page_size: 100, ...(groupId ? { group_id: groupId } : {}) },
  })
  return normalizeAssets(requireSuccess(response.data))
}

export async function deleteAsset(assetId: string) {
  const response = await api.delete<ApiResponse<unknown>>(
    `/api/mobilecloud/assets/${encodeURIComponent(assetId)}`,
    mutationConfig
  )
  return requireSuccess(response.data)
}

export async function uploadAsset(input: {
  file: File
  groupId: string
  assetName: string
  assetType: string
}) {
  const form = new FormData()
  form.append('file', input.file)
  form.append('group_id', input.groupId)
  form.append('asset_name', input.assetName)
  form.append('asset_type', input.assetType)
  const response = await api.post<ApiResponse<UploadResult>>(
    '/api/mobilecloud/uploads',
    form,
    mutationConfig
  )
  return requireSuccess(response.data)
}
