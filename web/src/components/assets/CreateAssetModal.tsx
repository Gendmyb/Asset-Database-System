import { useForm } from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import Modal from '../ui/Modal'
import Button from '../ui/Button'
import FormField from '../ui/FormField'
import * as assetsApi from '../../api/assets'
import { getApiError } from '../../lib/errors'

const FIELD_STYLE: React.CSSProperties = {
  width: '100%',
  padding: '7px 10px',
  borderRadius: 5,
  border: '1px solid var(--border-default)',
  background: 'rgba(255,255,255,0.02)',
  color: 'var(--text-primary)',
  fontSize: 13,
  outline: 'none',
  fontFamily: 'inherit',
}

const SECTION_TITLE: React.CSSProperties = {
  fontSize: 12,
  fontWeight: 600,
  color: 'var(--text-tertiary)',
  textTransform: 'uppercase',
  letterSpacing: '0.5px',
  margin: '0 0 8px',
}

interface CreateAssetFormData {
  name: string
  asset_tag: string
  type_id: string
  serial_number: string
  manufacturer: string
  model: string
  managed_by: string
  location_id: string
  purchase_price: string
  purchase_date: string
  supplier: string
  warranty_until: string
  depreciation_method: string
  useful_life_months: string
  salvage: string
  count: string
}

interface CreateAssetModalProps {
  open: boolean
  onClose: () => void
  userOptions: { value: string; label: string }[]
  locationOptions: { value: string; label: string }[]
  typeOptions: { value: string; label: string }[]
}

export default function CreateAssetModal({
  open,
  onClose,
  userOptions,
  locationOptions,
  typeOptions,
}: CreateAssetModalProps) {
  const { register, handleSubmit, reset, watch } = useForm<CreateAssetFormData>({
    defaultValues: {
      type_id: '10000000-0000-4000-a000-000000000001',
      count: '1',
    },
  })
  const queryClient = useQueryClient()
  const count = Number(watch('count')) || 1

  const mutation = useMutation<unknown, Error, CreateAssetFormData>({
    mutationFn: async (data: CreateAssetFormData) => {
      const payload: assetsApi.CreateAssetData = {
        name: data.name,
        asset_tag: data.asset_tag || undefined,
        type_id: data.type_id,
        serial_number: data.serial_number || undefined,
        manufacturer: data.manufacturer || undefined,
        model: data.model || undefined,
        managed_by: data.managed_by || undefined,
        location_id: data.location_id || undefined,
        purchase_price: data.purchase_price ? Number(data.purchase_price) : undefined,
        purchase_date: data.purchase_date || undefined,
        supplier: data.supplier || undefined,
        warranty_until: data.warranty_until || undefined,
        depreciation_method: data.depreciation_method || undefined,
        useful_life_months: data.useful_life_months ? Number(data.useful_life_months) : undefined,
        salvage: data.salvage ? Number(data.salvage) : undefined,
      }
      const n = Number(data.count) || 1
      if (n > 1) {
        return assetsApi.batch(payload, n) as unknown
      }
      return assetsApi.create(payload) as unknown
    },
    onSuccess: () => {
      toast.success(count > 1 ? `批量入库成功 (${count}件)` : '资产创建成功')
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      reset()
      onClose()
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  const onSubmit = (data: CreateAssetFormData) => {
    if (!data.name.trim()) {
      toast.error('名称不能为空')
      return
    }
    mutation.mutate(data)
  }

  return (
    <Modal open={open} onClose={onClose} title="新建资产" width="520px">
      <form onSubmit={handleSubmit(onSubmit)}>
        {/* 基本信息 */}
        <fieldset style={{ border: 'none', padding: 0, margin: '0 0 16px' }}>
          <legend style={SECTION_TITLE}>基本信息</legend>
          <FormField label="名称" required>
            <input
              {...register('name', { required: '必填' })}
              style={FIELD_STYLE}
            />
          </FormField>
          <FormField label="资产编号">
            <input
              {...register('asset_tag')}
              placeholder="留空自动生成"
              style={FIELD_STYLE}
            />
          </FormField>
          <FormField label="资产类型">
            <select
              {...register('type_id')}
              style={{ ...FIELD_STYLE, cursor: 'pointer' }}
            >
              {typeOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </FormField>
          <FormField label="序列号">
            <input {...register('serial_number')} style={FIELD_STYLE} />
          </FormField>
          <FormField label="制造商">
            <input {...register('manufacturer')} style={FIELD_STYLE} />
          </FormField>
          <FormField label="型号">
            <input {...register('model')} style={FIELD_STYLE} />
          </FormField>
        </fieldset>

        {/* 采购信息 */}
        <fieldset style={{ border: 'none', padding: 0, margin: '0 0 16px' }}>
          <legend style={SECTION_TITLE}>采购信息</legend>
          <FormField label="采购价格">
            <input {...register('purchase_price')} type="number" style={FIELD_STYLE} />
          </FormField>
          <FormField label="采购日期">
            <input {...register('purchase_date')} type="date" style={FIELD_STYLE} />
          </FormField>
          <FormField label="供应商">
            <input {...register('supplier')} style={FIELD_STYLE} />
          </FormField>
          <FormField label="保修到期">
            <input {...register('warranty_until')} type="date" style={FIELD_STYLE} />
          </FormField>
          <FormField label="折旧方法">
            <select
              {...register('depreciation_method')}
              style={{ ...FIELD_STYLE, cursor: 'pointer' }}
            >
              <option value="">不折旧</option>
              <option value="straight_line">直线法</option>
              <option value="declining_balance">双倍余额递减</option>
              <option value="sum_of_years">年数总和</option>
            </select>
          </FormField>
          <FormField label="使用年限(月)">
            <input
              {...register('useful_life_months')}
              type="number"
              placeholder="如: 36"
              style={FIELD_STYLE}
            />
          </FormField>
          <FormField label="残值">
            <input
              {...register('salvage')}
              type="number"
              placeholder="0"
              style={FIELD_STYLE}
            />
          </FormField>
        </fieldset>

        {/* 管理归属 */}
        <fieldset style={{ border: 'none', padding: 0, margin: '0 0 16px' }}>
          <legend style={SECTION_TITLE}>管理归属</legend>
          <FormField label="管理人">
            <select
              {...register('managed_by')}
              style={{ ...FIELD_STYLE, cursor: 'pointer' }}
            >
              <option value="">选择用户...</option>
              {userOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </FormField>
          <FormField label="位置">
            <select
              {...register('location_id')}
              style={{ ...FIELD_STYLE, cursor: 'pointer' }}
            >
              <option value="">选择位置...</option>
              {locationOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </FormField>
        </fieldset>

        {/* 批量入库 */}
        <fieldset style={{ border: 'none', padding: 0, margin: '0 0 16px' }}>
          <legend style={SECTION_TITLE}>批量入库</legend>
          <FormField label="数量">
            <input
              {...register('count', {
                required: '必填',
                min: { value: 1, message: '最少1件' },
                max: { value: 100, message: '最多100件' },
              })}
              type="number"
              min={1}
              max={100}
              style={FIELD_STYLE}
            />
          </FormField>
        </fieldset>

        <div style={{ display: 'flex', gap: 10, marginTop: 20 }}>
          <Button
            variant="secondary"
            onClick={onClose}
            disabled={mutation.isPending}
            style={{ flex: 1 }}
          >
            取消
          </Button>
          <Button
            type="submit"
            loading={mutation.isPending}
            style={{ flex: 1 }}
          >
            {count > 1 ? `批量入库 (${count}件)` : '入库'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
