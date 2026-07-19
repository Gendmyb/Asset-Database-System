import { useState, useMemo } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { toast } from 'sonner'
import DataTable, { Column } from '../components/ui/DataTable'
import Badge from '../components/ui/Badge'
import Button from '../components/ui/Button'
import ConfirmDialog from '../components/ui/ConfirmDialog'
import Modal from '../components/ui/Modal'
import Select from '../components/ui/Select'
import Input from '../components/ui/Input'
import Spinner from '../components/ui/Spinner'
import EmptyState from '../components/ui/EmptyState'
import * as stocktakeApi from '../api/stocktake'
import * as lookupApi from '../api/lookup'
import { getApiError } from '../lib/errors'
import { useRole } from '../lib/roles'

function fmtDate(d?: string): string {
  if (!d) return '—'
  try {
    return new Date(d).toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  } catch {
    return d
  }
}

const RESULT_OPTIONS = [
  { value: '', label: '未核对' },
  { value: 'found', label: '已找到' },
  { value: 'missing', label: '缺失' },
  { value: 'moved', label: '已移动' },
]

const RESULT_LABELS: Record<string, string> = {
  found: '已找到',
  missing: '缺失',
  moved: '已移动',
}

export default function StocktakeDetail() {
  const { id } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const planId = id!
  const { canManage, canAdmin } = useRole()

  const [resultFilter, setResultFilter] = useState('')
  const [search, setSearch] = useState('')
  const [surplusOpen, setSurplusOpen] = useState(false)
  const [completeOpen, setCompleteOpen] = useState(false)
  const [applyMoves, setApplyMoves] = useState(false)

  const {
    register: registerSurplus,
    handleSubmit: handleSurplusSubmit,
    reset: resetSurplus,
    formState: { errors: surplusErrors },
  } = useForm<{ surplus_note: string }>()

  // Plan detail
  const {
    data: plan,
    isLoading: planLoading,
    isError: planError,
    error: planErr,
  } = useQuery({
    queryKey: ['stocktake', planId],
    queryFn: () => stocktakeApi.getPlan(planId),
    enabled: !!planId,
  })

  // Items list
  const itemParams = useMemo(() => {
    const p: Record<string, string | undefined> = {}
    if (resultFilter) p.result = resultFilter
    if (search) p.search = search
    return p
  }, [resultFilter, search])

  const {
    data: itemsData,
    isLoading: itemsLoading,
    isError: itemsError,
    error: itemsErr,
  } = useQuery({
    queryKey: ['stocktake-items', planId, itemParams],
    queryFn: () =>
      stocktakeApi.listItems(planId, {
        result: itemParams.result,
        search: itemParams.search,
      }),
    enabled: !!planId,
  })

  // Locations lookup for actual location select
  const { data: locations } = useQuery({
    queryKey: ['lookup-locations'],
    queryFn: () => lookupApi.locations(),
    staleTime: 60000,
  })

  const items = itemsData?.data ?? []

  // Summary stats
  const summary = useMemo(() => {
    const total = items.length
    const found = items.filter((it) => it.result === 'found').length
    const missing = items.filter((it) => it.result === 'missing').length
    const moved = items.filter((it) => it.result === 'moved').length
    const checked = found + missing + moved
    return { total, found, missing, moved, checked }
  }, [items])

  // Update item mutation
  const updateMutation = useMutation({
    mutationFn: ({
      itemId,
      data,
    }: {
      itemId: string
      data: { result?: string; actual_location_id?: string; notes?: string }
    }) => stocktakeApi.updateItem(planId, itemId, data),
    onError: (err) => {
      toast.error(getApiError(err))
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['stocktake-items', planId] })
    },
  })

  // Add surplus mutation
  const surplusMutation = useMutation({
    mutationFn: (data: { surplus_note: string }) =>
      stocktakeApi.addSurplus(planId, data),
    onSuccess: () => {
      toast.success('盘盈已登记')
      queryClient.invalidateQueries({ queryKey: ['stocktake-items', planId] })
      setSurplusOpen(false)
      resetSurplus()
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  // Complete mutation
  const completeMutation = useMutation({
    mutationFn: () =>
      stocktakeApi.completePlan(planId, { apply_moves: applyMoves }),
    onSuccess: () => {
      toast.success('盘点已完成')
      queryClient.invalidateQueries({ queryKey: ['stocktake', planId] })
      queryClient.invalidateQueries({ queryKey: ['stocktakes'] })
      setCompleteOpen(false)
    },
    onError: (err) => {
      toast.error(getApiError(err))
    },
  })

  function handleResultChange(itemId: string, newResult: string) {
    const payload: { result?: string; actual_location_id?: string } = {
      result: newResult || undefined,
    }
    // If result changed away from 'moved', clear location
    if (newResult !== 'moved') {
      payload.actual_location_id = ''
    }
    updateMutation.mutate({ itemId, data: payload })
  }

  function handleLocationChange(itemId: string, locationId: string) {
    updateMutation.mutate({
      itemId,
      data: { actual_location_id: locationId || undefined },
    })
  }

  const locationOptions = useMemo(
    () =>
      (locations ?? []).map((loc) => ({
        value: loc.id,
        label: loc.name,
      })),
    [locations]
  )

  const columns: Column<stocktakeApi.StocktakeItem>[] = [
    {
      key: 'asset_name',
      label: '资产名称',
      render: (row) => (
        <Link
          to={`/assets/${row.asset_id}`}
          style={{
            color: 'var(--brand)',
            textDecoration: 'none',
            fontWeight: 500,
            fontSize: 13,
          }}
        >
          {row.asset_name || row.asset_tag || row.asset_id}
        </Link>
      ),
    },
    {
      key: 'asset_tag',
      label: '编号',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {row.asset_tag || row.asset_id?.slice(0, 8) || '—'}
        </span>
      ),
    },
    {
      key: 'expected_location',
      label: '预期位置',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {row.expected_location || '—'}
        </span>
      ),
    },
    {
      key: 'expected_status',
      label: '预期状态',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {row.expected_status || '—'}
        </span>
      ),
    },
    {
      key: 'result',
      label: '盘点结果',
      render: (row) => (
        <select
          value={row.result || ''}
          onChange={(e) => handleResultChange(row.id, e.target.value)}
          disabled={updateMutation.isPending || !canManage}
          style={{
            padding: '4px 8px',
            borderRadius: 5,
            border: '1px solid var(--border-default)',
            background: 'rgba(255,255,255,0.02)',
            color: 'var(--text-primary)',
            fontSize: 12,
            fontFamily: 'inherit',
            cursor: 'pointer',
            minWidth: 90,
          }}
        >
          {RESULT_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      ),
    },
    {
      key: 'actual_location',
      label: '实际位置',
      render: (row) => {
        if (row.result !== 'moved') {
          return (
            <span style={{ fontSize: 12, color: 'var(--text-quaternary)' }}>
              —
            </span>
          )
        }
        if (!locationOptions.length) {
          return (
            <input
              disabled
              style={{
                padding: '4px 8px',
                borderRadius: 5,
                border: '1px solid var(--border-default)',
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-tertiary)',
                fontSize: 12,
                fontFamily: 'inherit',
                width: 120,
              }}
              placeholder="加载中..."
            />
          )
        }
        const currentVal = row.actual_location_id || ''
        return (
          <select
            value={currentVal}
            onChange={(e) => handleLocationChange(row.id, e.target.value)}
            disabled={updateMutation.isPending || !canManage}
            style={{
              padding: '4px 8px',
              borderRadius: 5,
              border: '1px solid var(--border-default)',
              background: 'rgba(255,255,255,0.02)',
              color: 'var(--text-primary)',
              fontSize: 12,
              fontFamily: 'inherit',
              cursor: 'pointer',
              minWidth: 120,
            }}
          >
            <option value="">请选择</option>
            {locationOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        )
      },
    },
    {
      key: 'checked_by',
      label: '核查人',
      render: (row) => (
        <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
          {row.checked_by || '—'}
        </span>
      ),
    },
    {
      key: 'notes',
      label: '备注',
      render: (row) => (
        <span
          style={{
            fontSize: 12,
            color: row.notes
              ? 'var(--text-secondary)'
              : 'var(--text-quaternary)',
            maxWidth: 120,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            display: 'inline-block',
          }}
          title={row.notes}
        >
          {row.notes || '—'}
        </span>
      ),
    },
  ]

  // ── Render ──────────────────────────────────────────

  if (planLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 80 }}>
        <Spinner size={24} />
      </div>
    )
  }

  if (planError || !plan) {
    return (
      <div style={{ padding: 32 }}>
        <div
          style={{
            padding: '12px 16px',
            borderRadius: 8,
            background: 'rgba(239,68,68,0.1)',
            border: '1px solid rgba(239,68,68,0.2)',
            color: '#f87171',
            fontSize: 13,
          }}
        >
          {planError ? getApiError(planErr) : '计划未找到'}
        </div>
        <div style={{ marginTop: 16 }}>
          <Link
            to="/stocktakes"
            style={{
              color: 'var(--brand)',
              textDecoration: 'none',
              fontSize: 13,
            }}
          >
            &larr; 返回盘点列表
          </Link>
        </div>
      </div>
    )
  }

  const progress = summary.total > 0 ? (summary.checked / summary.total) * 100 : 0

  return (
    <div style={{ padding: 32, maxWidth: 1400 }}>
      {/* Back link */}
      <Link
        to="/stocktakes"
        style={{
          display: 'inline-block',
          color: 'var(--text-tertiary)',
          textDecoration: 'none',
          fontSize: 13,
          marginBottom: 16,
        }}
      >
        &larr; 返回盘点列表
      </Link>

      {/* Plan Header */}
      <div
        style={{
          background: 'var(--bg-surface)',
          borderRadius: 10,
          border: '1px solid var(--border-subtle)',
          padding: 20,
          marginBottom: 24,
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'flex-start',
            justifyContent: 'space-between',
            flexWrap: 'wrap',
            gap: 16,
          }}
        >
          <div>
            <h1
              style={{
                fontSize: 20,
                fontWeight: 600,
                color: 'var(--text-primary)',
                margin: '0 0 8px',
              }}
            >
              {plan.name}
            </h1>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 16,
                flexWrap: 'wrap',
                fontSize: 13,
                color: 'var(--text-secondary)',
              }}
            >
              <span>
                状态：<Badge type="stocktake_status" status={plan.status} />
              </span>
              <span>创建：{fmtDate(plan.created_at)}</span>
              {plan.started_at && <span>开始：{fmtDate(plan.started_at)}</span>}
              {plan.completed_at && (
                <span>完成：{fmtDate(plan.completed_at)}</span>
              )}
            </div>
          </div>
        </div>

        {/* Progress */}
        <div style={{ marginTop: 20 }}>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              marginBottom: 6,
            }}
          >
            <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>
              进度：{summary.checked}/{summary.total}
            </span>
            <span style={{ fontSize: 12, color: 'var(--text-tertiary)' }}>
              {progress.toFixed(0)}%
            </span>
          </div>
          <div
            style={{
              height: 8,
              borderRadius: 4,
              background: 'rgba(255,255,255,0.05)',
              overflow: 'hidden',
              display: 'flex',
            }}
          >
            {summary.total > 0 && (
              <>
                <div
                  style={{
                    height: '100%',
                    background: '#4ade80',
                    width: `${(summary.found / summary.total) * 100}%`,
                    transition: 'width .3s',
                  }}
                />
                <div
                  style={{
                    height: '100%',
                    background: '#f87171',
                    width: `${(summary.missing / summary.total) * 100}%`,
                    transition: 'width .3s',
                  }}
                />
                <div
                  style={{
                    height: '100%',
                    background: '#fbbf24',
                    width: `${(summary.moved / summary.total) * 100}%`,
                    transition: 'width .3s',
                  }}
                />
              </>
            )}
          </div>
          <div
            style={{
              display: 'flex',
              gap: 16,
              marginTop: 8,
              fontSize: 12,
              color: 'var(--text-quaternary)',
            }}
          >
            <span>
              <span
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  borderRadius: 2,
                  background: '#4ade80',
                  marginRight: 4,
                  verticalAlign: 'middle',
                }}
              />
              已找到 {summary.found}
            </span>
            <span>
              <span
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  borderRadius: 2,
                  background: '#f87171',
                  marginRight: 4,
                  verticalAlign: 'middle',
                }}
              />
              缺失 {summary.missing}
            </span>
            <span>
              <span
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  borderRadius: 2,
                  background: '#fbbf24',
                  marginRight: 4,
                  verticalAlign: 'middle',
                }}
              />
              已移动 {summary.moved}
            </span>
            <span>
              <span
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  borderRadius: 2,
                  background: 'rgba(255,255,255,0.1)',
                  marginRight: 4,
                  verticalAlign: 'middle',
                }}
              />
              未核对 {summary.total - summary.checked}
            </span>
          </div>
        </div>
      </div>

      {/* Filters */}
      <div
        style={{
          display: 'flex',
          gap: 12,
          marginBottom: 16,
          flexWrap: 'wrap',
          alignItems: 'flex-end',
        }}
      >
        <div>
          <label
            style={{
              display: 'block',
              fontSize: 12,
              color: 'var(--text-quaternary)',
              marginBottom: 4,
            }}
          >
            结果筛选
          </label>
          <select
            value={resultFilter}
            onChange={(e) => setResultFilter(e.target.value)}
            style={{
              padding: '6px 10px',
              borderRadius: 5,
              border: '1px solid var(--border-default)',
              background: 'rgba(255,255,255,0.02)',
              color: 'var(--text-primary)',
              fontSize: 13,
              fontFamily: 'inherit',
              cursor: 'pointer',
            }}
          >
            <option value="">全部</option>
            {RESULT_OPTIONS.filter((o) => o.value).map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label
            style={{
              display: 'block',
              fontSize: 12,
              color: 'var(--text-quaternary)',
              marginBottom: 4,
            }}
          >
            搜索
          </label>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="资产名称/编号..."
            style={{
              padding: '6px 10px',
              borderRadius: 5,
              border: '1px solid var(--border-default)',
              background: 'rgba(255,255,255,0.02)',
              color: 'var(--text-primary)',
              fontSize: 13,
              fontFamily: 'inherit',
              outline: 'none',
              width: 200,
            }}
            onFocus={(e) => {
              e.currentTarget.style.borderColor = 'var(--brand)'
            }}
            onBlur={(e) => {
              e.currentTarget.style.borderColor = 'var(--border-default)'
            }}
          />
        </div>
      </div>

      {/* Items Error */}
      {itemsError && (
        <div
          style={{
            padding: '12px 16px',
            borderRadius: 8,
            background: 'rgba(239,68,68,0.1)',
            border: '1px solid rgba(239,68,68,0.2)',
            color: '#f87171',
            fontSize: 13,
            marginBottom: 16,
          }}
        >
          {getApiError(itemsErr)}
        </div>
      )}

      {/* Items Table */}
      <DataTable
        columns={columns}
        rows={items}
        loading={itemsLoading}
        emptyState={{
          title:
            items.length === 0 && plan.status === 'draft'
              ? '盘点尚未开始'
              : '暂无盘点项目',
          description:
            plan.status === 'draft'
              ? '请先开始盘点以生成待核对项目列表'
              : '当前筛选条件下没有匹配的项目',
        }}
      />

      {/* Bottom Actions — only for in_progress plans (盘盈 manager+, 完成 admin+) */}
      {plan.status === 'in_progress' && (canManage || canAdmin) && (
        <div
          style={{
            display: 'flex',
            gap: 12,
            justifyContent: 'flex-end',
            marginTop: 24,
          }}
        >
          {canManage && (
            <Button variant="secondary" onClick={() => setSurplusOpen(true)}>
              盘盈登记
            </Button>
          )}
          {canAdmin && (
            <Button
              variant="primary"
              onClick={() => {
                setApplyMoves(false)
                setCompleteOpen(true)
              }}
            >
              完成盘点
            </Button>
          )}
        </div>
      )}

      {/* Surplus Modal */}
      <Modal
        open={surplusOpen}
        onClose={() => {
          setSurplusOpen(false)
          resetSurplus()
        }}
        title="盘盈登记"
      >
        <form
          onSubmit={handleSurplusSubmit((values) =>
            surplusMutation.mutate(values)
          )}
        >
          <div style={{ marginBottom: 14 }}>
            <label
              style={{
                display: 'block',
                fontSize: 12,
                fontWeight: 500,
                color: surplusErrors.surplus_note
                  ? 'var(--danger)'
                  : 'var(--text-secondary)',
                marginBottom: 5,
              }}
            >
              盘盈说明<span style={{ color: 'var(--danger)' }}>*</span>
            </label>
            <textarea
              placeholder="描述额外发现的资产信息..."
              {...registerSurplus('surplus_note', {
                required: '请输入盘盈说明',
              })}
              style={{
                width: '100%',
                minHeight: 80,
                padding: '7px 10px',
                borderRadius: 5,
                border: `1px solid ${surplusErrors.surplus_note ? 'var(--danger)' : 'var(--border-default)'}`,
                background: 'rgba(255,255,255,0.02)',
                color: 'var(--text-primary)',
                fontSize: 13,
                fontFamily: 'inherit',
                outline: 'none',
                resize: 'vertical',
              }}
            />
            {surplusErrors.surplus_note && (
              <p
                style={{
                  fontSize: 11,
                  color: 'var(--danger)',
                  margin: '4px 0 0',
                }}
              >
                {surplusErrors.surplus_note.message}
              </p>
            )}
          </div>
          <div
            style={{
              display: 'flex',
              gap: 10,
              justifyContent: 'flex-end',
            }}
          >
            <Button
              variant="secondary"
              type="button"
              onClick={() => {
                setSurplusOpen(false)
                resetSurplus()
              }}
            >
              取消
            </Button>
            <Button
              variant="primary"
              type="submit"
              loading={surplusMutation.isPending}
            >
              登记
            </Button>
          </div>
        </form>
      </Modal>

      {/* Complete Confirm */}
      <ConfirmDialog
        open={completeOpen}
        onClose={() => setCompleteOpen(false)}
        title="完成盘点"
        description="确认完成盘点？完成后将不可再修改盘点结果。"
        confirmLabel="完成"
        onConfirm={() => completeMutation.mutate()}
        loading={completeMutation.isPending}
      >
        <label
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            fontSize: 13,
            color: 'var(--text-secondary)',
            cursor: 'pointer',
          }}
        >
          <input
            type="checkbox"
            checked={applyMoves}
            onChange={(e) => setApplyMoves(e.target.checked)}
            style={{ cursor: 'pointer' }}
          />
          应用位置变更（apply_moves）
        </label>
      </ConfirmDialog>
    </div>
  )
}
