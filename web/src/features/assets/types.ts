export type AssetType = 'Image' | 'Video' | 'Audio'

export type AssetGroup = {
  groupId?: string
  groupName?: string
  groupType?: string
  description?: string
  [key: string]: unknown
}

export type Asset = {
  assetId?: string
  assetName?: string
  assetType?: AssetType | string
  assetUrl?: string
  status?: string
  groupId?: string
  errorMessage?: string
  [key: string]: unknown
}

export type StorageInfo = {
  enabled: boolean
  upload_enabled: boolean
  mode: 'local' | 's3'
  max_bytes: number
  allowed_types: string[]
}

export type UploadResult = {
  upload: {
    key: string
    url: string
    backend: string
    original_name: string
    content_type: string
    size: number
  }
  asset?: unknown
  provider_error?: string
}
