import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import DataTable from '../../components/ui/DataTable'
import type { Column } from '../../components/ui/DataTable'
import Button from '../../components/ui/Button'
import Modal from '../../components/ui/Modal'
import Input from '../../components/ui/Input'
import FormField from '../../components/ui/FormField'
import * as settingsApi from '../../api/settings'
import * as lookupApi from '../../api/lookup'
import { getApiError } from '../../lib/errors'

interface LocationForm {
  name: string
}

export default function Settings() {
  const queryClient = useQueryClient()

  const { data: settings, isLoading: settingsLoading } = useQuery({
    queryKey: ['settings'],
    queryFn: () => settingsApi.get(),
  })

  const { data: assetTypes } = useQuery({
    queryKey: ['assetTypes'],
    queryFn: () => lookupApi.assetTypes(),
  })

  const { data: locations, isLoading: locationsLoading } = useQuery({
    queryKey: ['locations'],
    queryFn: () => lookupApi.locations(),
  })

  const [prefixInput, setPrefixInput] = useState('')
  const [orgNameInput, setOrgNameInput] = useState('')

  const setMutation = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      settingsApi.set(key, value),
    onSuccess: () => {
      toast.success('设置已保存')
      queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const settingsData = settings as Record<string, string> | null

  const locationList = Array.isArray(locations) ? locations : []
  const typeList = Array.isArray(assetTypes) ? assetTypes : []

  const locationColumns: Column<any>[] = [
    { key: 'name', label: '位置名称' },
    {
      key: 'id',
      label: 'ID',
      render: (row: any) => (
        <span
          style={{
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 11,
            color: 'var(--text-quaternary)',
          }}
        >
          {row.id?.substring(0, 8)}...
        </span>
      ),
    },
  ]

  const typeColumns: Column<any>[] = [
    { key: 'name', label: '类型名称' },
    {
      key: 'id',
      label: 'ID',
      render: (row: any) => (
        <span
          style={{
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 11,
            color: 'var(--text-quaternary)',
          }}
        >
          {row.id?.substring(0, 8)}...
        </span>
      ),
    },
  ]

  return (
    <div style={{ padding: 32, maxWidth: 1000 }}>
      <div style={{ marginBottom: 32 }}>
        <h1
          style={{
            fontSize: 20,
            fontWeight: 600,
            color: 'var(--text-primary)',
            letterSpacing: '-0.24px',
            margin: '0 0 4px',
          }}
        >
          系统设置
        </h1>
        <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
          管理系统配置和基础数据
        </p>
      </div>

      {/* System Settings */}
      <div
        style={{
          background: 'var(--bg-surface)',
          borderRadius: 10,
          border: '1px solid var(--border-subtle)',
          padding: 24,
          marginBottom: 24,
        }}
      >
        <h3
          style={{
            fontSize: 14,
            fontWeight: 600,
            color: 'var(--text-secondary)',
            margin: '0 0 16px',
          }}
        >
          系统配置
        </h3>

        {settingsLoading ? (
          <div style={{ color: 'var(--text-quaternary)', fontSize: 13 }}>
            加载中...
          </div>
        ) : (
          <>
            <Input
              label="资产编号前缀"
              value={prefixInput || settingsData?.asset_tag_prefix || ''}
              onChange={(e) => setPrefixInput(e.target.value)}
              placeholder={settingsData?.asset_tag_prefix || 'ASSET-'}
            />
            <div style={{ marginBottom: 14 }}>
              <Button
                onClick={() =>
                  setMutation.mutate({
                    key: 'asset_tag_prefix',
                    value: prefixInput,
                  })
                }
                loading={setMutation.isPending}
              >
                保存
              </Button>
            </div>

            <Input
              label="组织名称"
              value={orgNameInput || settingsData?.org_name || ''}
              onChange={(e) => setOrgNameInput(e.target.value)}
              placeholder={settingsData?.org_name || 'My Organization'}
            />
            <div style={{ marginBottom: 14 }}>
              <Button
                onClick={() =>
                  setMutation.mutate({
                    key: 'org_name',
                    value: orgNameInput,
                  })
                }
                loading={setMutation.isPending}
              >
                保存
              </Button>
            </div>
          </>
        )}
      </div>

      {/* Asset Types */}
      <div
        style={{
          background: 'var(--bg-surface)',
          borderRadius: 10,
          border: '1px solid var(--border-subtle)',
          padding: 24,
          marginBottom: 24,
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: 16,
          }}
        >
          <h3
            style={{
              fontSize: 14,
              fontWeight: 600,
              color: 'var(--text-secondary)',
              margin: 0,
            }}
          >
            资产类型
          </h3>
        </div>
        <DataTable
          columns={typeColumns}
          rows={typeList}
          emptyState={{ title: '暂无资产类型' }}
        />
      </div>

      {/* Locations */}
      <div
        style={{
          background: 'var(--bg-surface)',
          borderRadius: 10,
          border: '1px solid var(--border-subtle)',
          padding: 24,
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            marginBottom: 16,
          }}
        >
          <h3
            style={{
              fontSize: 14,
              fontWeight: 600,
              color: 'var(--text-secondary)',
              margin: 0,
            }}
          >
            位置列表
          </h3>
        </div>
        <DataTable
          columns={locationColumns}
          rows={locationList}
          loading={locationsLoading}
          emptyState={{ title: '暂无位置' }}
        />
      </div>
    </div>
  )
}
