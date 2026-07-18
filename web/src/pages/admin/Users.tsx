import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useForm } from 'react-hook-form'
import DataTable from '../../components/ui/DataTable'
import type { Column } from '../../components/ui/DataTable'
import Button from '../../components/ui/Button'
import Badge from '../../components/ui/Badge'
import Modal from '../../components/ui/Modal'
import Select from '../../components/ui/Select'
import FormField from '../../components/ui/FormField'
import ConfirmDialog from '../../components/ui/ConfirmDialog'
import * as usersApi from '../../api/users'
import { getApiError } from '../../lib/errors'

interface CreateUserForm {
  username: string
  role: string
  email: string
}

export default function Users() {
  const [showCreate, setShowCreate] = useState(false)
  const [resetTarget, setResetTarget] = useState<string | null>(null)
  const [newPassword, setNewPassword] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const { data: users, isLoading } = useQuery({
    queryKey: ['admin', 'users'],
    queryFn: () => usersApi.list(),
  })

  const createMutation = useMutation({
    mutationFn: (data: CreateUserForm) =>
      usersApi.create({
        username: data.username,
        role: data.role,
        email: data.email || undefined,
      }),
    onSuccess: () => {
      toast.success('用户创建成功')
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
      setShowCreate(false)
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const updateRoleMutation = useMutation({
    mutationFn: ({ id, role }: { id: string; role: string }) =>
      usersApi.update(id, { role }),
    onSuccess: () => {
      toast.success('角色已更新')
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, disabled }: { id: string; disabled: boolean }) =>
      usersApi.update(id, { disabled }),
    onSuccess: () => {
      toast.success('状态已更新')
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const resetMutation = useMutation({
    mutationFn: (id: string) => usersApi.resetPassword(id),
    onSuccess: (data: any) => {
      const pwd = data?.new_password || data?.data?.new_password || '已重置'
      setNewPassword(pwd)
      queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  const userList = Array.isArray(users) ? users : []

  const columns: Column<any>[] = [
    { key: 'username', label: '用户名' },
    {
      key: 'role',
      label: '角色',
      render: (row: any) => (
        <Select
          value={row.role}
          onChange={(e) =>
            updateRoleMutation.mutate({ id: row.id, role: e.target.value })
          }
          options={[
            { value: 'admin', label: '管理员' },
            { value: 'user', label: '用户' },
            { value: 'viewer', label: '只读' },
          ]}
          style={{
            padding: '4px 8px',
            border: '1px solid var(--border-default)',
            borderRadius: 4,
            background: 'transparent',
            color: 'var(--text-primary)',
            fontSize: 12,
            fontFamily: 'inherit',
            cursor: 'pointer',
            outline: 'none',
          }}
        />
      ),
    },
    {
      key: 'email',
      label: '邮箱',
      render: (row: any) => (
        <span style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
          {row.email || '—'}
        </span>
      ),
    },
    {
      key: 'disabled',
      label: '状态',
      render: (row: any) => (
        <Badge status={row.disabled ? 'retired' : 'available'} />
      ),
    },
    {
      key: 'actions',
      label: '操作',
      render: (row: any) => (
        <div style={{ display: 'flex', gap: 6 }}>
          <Button
            variant="ghost"
            onClick={() =>
              toggleMutation.mutate({ id: row.id, disabled: !row.disabled })
            }
            style={{ fontSize: 12, padding: '4px 8px' }}
          >
            {row.disabled ? '启用' : '禁用'}
          </Button>
          <Button
            variant="ghost"
            onClick={() => setResetTarget(row.id)}
            style={{ fontSize: 12, padding: '4px 8px' }}
          >
            重置密码
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div style={{ padding: 32, maxWidth: 1200 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          marginBottom: 24,
        }}
      >
        <div>
          <h1
            style={{
              fontSize: 20,
              fontWeight: 600,
              color: 'var(--text-primary)',
              letterSpacing: '-0.24px',
              margin: '0 0 4px',
            }}
          >
            用户管理
          </h1>
          <p style={{ fontSize: 13, color: 'var(--text-tertiary)' }}>
            管理系统用户和权限
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)}>+ 新建用户</Button>
      </div>

      <DataTable
        columns={columns}
        rows={userList}
        loading={isLoading}
        emptyState={{ title: '暂无用户' }}
      />

      {/* Create User Modal */}
      <CreateUserModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        loading={createMutation.isPending}
        onSubmit={(data) => createMutation.mutate(data)}
      />

      {/* Reset Password Confirm */}
      <ConfirmDialog
        open={!!resetTarget}
        onClose={() => {
          setResetTarget(null)
          setNewPassword(null)
        }}
        title="重置密码"
        description={
          newPassword
            ? `新密码: ${newPassword}（请妥善保存）`
            : '确认重置该用户的密码？密码将随机生成。'
        }
        confirmLabel={newPassword ? '已复制' : '确认重置'}
        onConfirm={() => {
          if (newPassword) {
            navigator.clipboard.writeText(newPassword)
            toast.success('密码已复制')
            setResetTarget(null)
            setNewPassword(null)
          } else if (resetTarget) {
            resetMutation.mutate(resetTarget)
          }
        }}
        loading={resetMutation.isPending}
      />
    </div>
  )
}

function CreateUserModal({
  open,
  onClose,
  loading,
  onSubmit,
}: {
  open: boolean
  onClose: () => void
  loading: boolean
  onSubmit: (data: CreateUserForm) => void
}) {
  const { register, handleSubmit, reset } = useForm<CreateUserForm>({
    defaultValues: { role: 'user' },
  })

  return (
    <Modal open={open} onClose={onClose} title="新建用户" width="440px">
      <form
        onSubmit={handleSubmit((data) => {
          onSubmit(data)
          reset()
        })}
      >
        <FormField label="用户名" required>
          <input
            {...register('username', { required: true })}
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
        <FormField label="角色" required>
          <select
            {...register('role')}
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
            <option value="admin">管理员</option>
            <option value="user">用户</option>
            <option value="viewer">只读</option>
          </select>
        </FormField>
        <FormField label="邮箱">
          <input
            {...register('email')}
            type="email"
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
        <div style={{ display: 'flex', gap: 10, marginTop: 20 }}>
          <Button variant="secondary" onClick={onClose} disabled={loading} style={{ flex: 1 }}>
            取消
          </Button>
          <Button type="submit" loading={loading} style={{ flex: 1 }}>
            创建
          </Button>
        </div>
      </form>
    </Modal>
  )
}
