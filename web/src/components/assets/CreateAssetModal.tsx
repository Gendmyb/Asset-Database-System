import { useForm } from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import Modal from '../ui/Modal'
import Input from '../ui/Input'
import Select from '../ui/Select'
import Button from '../ui/Button'
import FormField from '../ui/FormField'
import * as assetsApi from '../../api/assets'
import { getApiError } from '../../lib/errors'

interface CreateAssetFormData {
  name: string
  asset_tag: string
  type_id: string
  serial_number: string
  manufacturer: string
  model: string
  managed_by: string
  location_id: string
  price: string
  purchase_date: string
  supplier: string
  warranty_expiry: string
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
  const { register, handleSubmit, reset } = useForm<CreateAssetFormData>({
    defaultValues: {
      type_id: '10000000-0000-4000-a000-000000000001',
    },
  })
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: (data: CreateAssetFormData) =>
      assetsApi.create({
        name: data.name,
        asset_tag: data.asset_tag || undefined,
        type_id: data.type_id,
        serial_number: data.serial_number || undefined,
        manufacturer: data.manufacturer || undefined,
        model: data.model || undefined,
        managed_by: data.managed_by || undefined,
        location_id: data.location_id || undefined,
        price: data.price ? Number(data.price) : undefined,
        purchase_date: data.purchase_date || undefined,
        supplier: data.supplier || undefined,
        warranty_expiry: data.warranty_expiry || undefined,
      }),
    onSuccess: () => {
      toast.success('资产创建成功')
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
    <Modal open={open} onClose={onClose} title="新建资产" width="480px">
      <form onSubmit={handleSubmit(onSubmit)}>
        {/* 基本信息 */}
        <div style={{ marginBottom: 16 }}>
          <h4
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: 'var(--text-tertiary)',
              textTransform: 'uppercase',
              letterSpacing: '0.5px',
              margin: '0 0 8px',
            }}
          >
            基本信息
          </h4>
          <FormField label="名称" required>
            <input
              {...register('name', { required: true })}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="资产编号">
            <input
              {...register('asset_tag')}
              placeholder="留空自动生成"
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="资产类型">
            <select
              {...register('type_id')}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
                cursor: 'pointer',
              }}
            >
              {typeOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </FormField>
          <FormField label="序列号">
            <input
              {...register('serial_number')}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="制造商">
            <input
              {...register('manufacturer')}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="型号">
            <input
              {...register('model')}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
        </div>

        {/* 采购信息 */}
        <div style={{ marginBottom: 16 }}>
          <h4
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: 'var(--text-tertiary)',
              textTransform: 'uppercase',
              letterSpacing: '0.5px',
              margin: '0 0 8px',
            }}
          >
            采购信息
          </h4>
          <FormField label="采购价格">
            <input
              {...register('price')}
              type="number"
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="采购日期">
            <input
              {...register('purchase_date')}
              type="date"
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="供应商">
            <input
              {...register('supplier')}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
          <FormField label="保修到期">
            <input
              {...register('warranty_expiry')}
              type="date"
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
              }}
            />
          </FormField>
        </div>

        {/* 管理归属 */}
        <div style={{ marginBottom: 16 }}>
          <h4
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: 'var(--text-tertiary)',
              textTransform: 'uppercase',
              letterSpacing: '0.5px',
              margin: '0 0 8px',
            }}
          >
            管理归属
          </h4>
          <FormField label="管理人">
            <select
              {...register('managed_by')}
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
                cursor: 'pointer',
              }}
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
              style={{
                width: '100%',
                padding: '7px 10px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                outline: 'none',
                fontFamily: 'inherit',
                cursor: 'pointer',
              }}
            >
              <option value="">选择位置...</option>
              {locationOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </FormField>
        </div>

        <div
          style={{ display: 'flex', gap: 10, marginTop: 20 }}
        >
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
            创建
          </Button>
        </div>
      </form>
    </Modal>
  )
}
