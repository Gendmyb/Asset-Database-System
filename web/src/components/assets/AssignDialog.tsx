import { useState, useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import Modal from '../ui/Modal'
import Button from '../ui/Button'
import FormField from '../ui/FormField'
import * as assetsApi from '../../api/assets'
import * as usersApi from '../../api/users'
import { getApiError } from '../../lib/errors'

interface AssignFormData {
  assigned_to: string
  notes: string
  due_date: string
}

interface AssignDialogProps {
  asset: assetsApi.Asset
  open: boolean
  onClose: () => void
  mode: 'assign' | 'borrow'
}

export default function AssignDialog({
  asset,
  open,
  onClose,
  mode,
}: AssignDialogProps) {
  const [userOptions, setUserOptions] = useState<
    { value: string; label: string }[]
  >([])
  const { register, handleSubmit, reset } = useForm<AssignFormData>()
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      usersApi
        .list()
        .then((users) => {
          setUserOptions(
            users.map((u: any) => ({
              value: u.id,
              label: u.username,
            }))
          )
        })
        .catch(() => setUserOptions([]))
      reset()
    }
  }, [open, reset])

  const mutation = useMutation({
    mutationFn: (data: AssignFormData) =>
      mode === 'assign'
        ? assetsApi.assign(asset.id, {
            assigned_to: data.assigned_to,
            notes: data.notes || undefined,
          })
        : assetsApi.borrow(asset.id, {
            assigned_to: data.assigned_to,
            notes: data.notes || undefined,
            due_date: data.due_date,
          }),
    onSuccess: () => {
      toast.success(mode === 'assign' ? '领用成功' : '借用成功')
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      onClose()
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  const onSubmit = (data: AssignFormData) => {
    if (!data.assigned_to) {
      toast.error('请选择用户')
      return
    }
    if (mode === 'borrow' && !data.due_date) {
      toast.error('请选择归还日期')
      return
    }
    mutation.mutate(data)
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={mode === 'assign' ? '领用资产' : '借用资产'}
      width="440px"
    >
      <div
        style={{
          fontSize: 13,
          color: 'var(--text-secondary)',
          marginBottom: 16,
        }}
      >
        资产:{' '}
        <span
          style={{ color: 'var(--text-primary)', fontWeight: 500 }}
        >
          {asset.name}
        </span>
        <span
          style={{
            marginLeft: 8,
            fontFamily: 'JetBrains Mono, monospace',
            fontSize: 12,
            color: 'var(--text-quaternary)',
          }}
        >
          {asset.asset_tag}
        </span>
      </div>

      <form onSubmit={handleSubmit(onSubmit)}>
        <FormField label="使用人" required>
          <select
            {...register('assigned_to', { required: true })}
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
            {userOptions.map((u) => (
              <option key={u.value} value={u.value}>
                {u.label}
              </option>
            ))}
          </select>
        </FormField>

        <FormField label="备注">
          <textarea
            {...register('notes')}
            rows={3}
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
              resize: 'vertical',
            }}
          />
        </FormField>

        {mode === 'borrow' && (
          <FormField label="归还日期" required>
            <input
              {...register('due_date', { required: true })}
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
        )}

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
            {mode === 'assign' ? '确认领用' : '确认借用'}
          </Button>
        </div>
      </form>
    </Modal>
  )
}
