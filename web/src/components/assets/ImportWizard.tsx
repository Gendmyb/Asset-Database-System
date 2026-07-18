import { useState, useRef } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import Modal from '../ui/Modal'
import Button from '../ui/Button'
import DataTable, { Column } from '../ui/DataTable'
import * as assetsApi from '../../api/assets'
import { downloadBlob } from '../../lib/download'
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

const STEP_INDICATOR: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: 8,
  marginBottom: 24,
}

function StepDot({ active, done }: { active: boolean; done: boolean }) {
  const bg = done ? 'var(--brand)' : active ? 'var(--brand)' : 'var(--border-default)'
  return (
    <div
      style={{
        width: 10,
        height: 10,
        borderRadius: '50%',
        background: bg,
        transition: 'background .2s',
      }}
    />
  )
}

function StepLine() {
  return <div style={{ width: 40, height: 1, background: 'var(--border-default)' }} />
}

interface ImportWizardProps {
  open: boolean
  onClose: () => void
}

export default function ImportWizard({ open, onClose }: ImportWizardProps) {
  const [step, setStep] = useState(1)
  const [file, setFile] = useState<File | null>(null)
  const [preview, setPreview] = useState<assetsApi.ImportPreview | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  // Step 1: download template
  const templateMutation = useMutation({
    mutationFn: () => assetsApi.importTemplate(),
    onSuccess: (blob) => {
      downloadBlob(blob, 'asset_import_template.csv')
      toast.success('模板下载成功')
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  // Step 2: preview
  const previewMutation = useMutation({
    mutationFn: (formData: FormData) => assetsApi.previewImport(formData),
    onSuccess: (data) => {
      setPreview(data)
      setStep(2)
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  // Step 3: execute
  const executeMutation = useMutation({
    mutationFn: (formData: FormData) => assetsApi.executeImport(formData),
    onSuccess: (data) => {
      toast.success(`成功导入 ${data.imported} 条资产`)
      queryClient.invalidateQueries({ queryKey: ['assets'] })
      resetAndClose()
    },
    onError: (err) => toast.error(getApiError(err)),
  })

  function resetAndClose() {
    setStep(1)
    setFile(null)
    setPreview(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
    onClose()
  }

  function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (f) {
      if (!f.name.endsWith('.csv')) {
        toast.error('仅支持 .csv 文件')
        return
      }
      setFile(f)
      const formData = new FormData()
      formData.append('file', f)
      previewMutation.mutate(formData)
    }
  }

  function handleConfirmImport() {
    if (!file) return
    const formData = new FormData()
    formData.append('file', file)
    executeMutation.mutate(formData)
  }

  const errorColumns: Column<assetsApi.ImportPreviewError>[] = [
    { key: 'row', label: '行号' },
    { key: 'field', label: '字段' },
    { key: 'message', label: '错误信息' },
  ]

  return (
    <Modal open={open} onClose={resetAndClose} title="导入资产" width="600px">
      {/* Step Indicator */}
      <div style={STEP_INDICATOR}>
        <StepDot active={step === 1} done={step > 1} />
        <StepLine />
        <StepDot active={step === 2} done={step > 2} />
        <StepLine />
        <StepDot active={step === 3} done={step > 3} />
      </div>

      {/* Step 1: Download template & Upload */}
      {step === 1 && (
        <div>
          <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 16 }}>
            请先下载模板文件，按格式填写资产数据后上传。
          </p>
          <div style={{ display: 'flex', gap: 10, marginBottom: 20 }}>
            <Button
              variant="secondary"
              loading={templateMutation.isPending}
              onClick={() => templateMutation.mutate()}
              style={{ flex: 1 }}
            >
              下载模板
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                fileInputRef.current?.click()
              }}
              style={{ flex: 1 }}
            >
              选择文件
            </Button>
          </div>
          <input
            ref={fileInputRef}
            type="file"
            accept=".csv"
            style={{ display: 'none' }}
            onChange={handleFileChange}
          />
          {file && (
            <div
              style={{
                padding: '8px 12px',
                borderRadius: 6,
                background: 'rgba(255,255,255,0.03)',
                border: '1px solid var(--border-subtle)',
                fontSize: 13,
                color: 'var(--text-secondary)',
              }}
            >
              已选择: {file.name} ({(file.size / 1024).toFixed(1)} KB)
            </div>
          )}
          {previewMutation.isPending && (
            <div style={{ display: 'flex', justifyContent: 'center', padding: '16px 0' }}>
              <div
                style={{
                  width: 20,
                  height: 20,
                  border: '2px solid var(--border-default)',
                  borderTopColor: 'var(--brand)',
                  borderRadius: '50%',
                  animation: 'spin 0.6s linear infinite',
                }}
              />
              <span style={{ marginLeft: 10, fontSize: 13, color: 'var(--text-tertiary)' }}>预览中...</span>
              <style>{`@keyframes spin { to { transform: rotate(360deg); } }`}</style>
            </div>
          )}
        </div>
      )}

      {/* Step 2: Preview */}
      {step === 2 && preview && (
        <div>
          <div style={{ marginBottom: 16 }}>
            <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>
              预览结果：共{' '}
              <strong style={{ color: 'var(--text-primary)' }}>{preview.total_count}</strong>{' '}
              条，其中{' '}
              <strong style={{ color: '#4ade80' }}>{preview.valid_count}</strong>{' '}
              条有效
              {preview.errors.length > 0 && (
                <>
                  ，<strong style={{ color: '#f87171' }}>{preview.errors.length}</strong> 条错误
                </>
              )}
              。
            </span>
          </div>
          {preview.errors.length > 0 && (
            <div style={{ marginBottom: 16 }}>
              <h4 style={{ fontSize: 12, fontWeight: 600, color: 'var(--text-tertiary)', margin: '0 0 8px' }}>
                错误详情
              </h4>
              <DataTable
                columns={errorColumns}
                rows={preview.errors}
                emptyState={{ title: '无错误' }}
              />
            </div>
          )}
          {preview.valid_count === 0 && (
            <p style={{ fontSize: 13, color: '#f87171', marginBottom: 16 }}>
              没有有效数据可以导入，请检查文件后重试。
            </p>
          )}
          <div style={{ display: 'flex', gap: 10, marginTop: 16 }}>
            <Button variant="secondary" onClick={resetAndClose} style={{ flex: 1 }}>
              取消
            </Button>
            <Button
              variant="secondary"
              onClick={() => {
                setStep(1)
                setPreview(null)
              }}
              style={{ flex: 1 }}
            >
              重新选择
            </Button>
            <Button
              onClick={() => {
                setStep(3)
              }}
              disabled={preview.valid_count === 0}
              style={{ flex: 1 }}
            >
              继续
            </Button>
          </div>
        </div>
      )}

      {/* Step 3: Confirm */}
      {step === 3 && preview && (
        <div>
          <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 16 }}>
            确认导入 <strong>{preview.valid_count}</strong> 条有效资产？
            {preview.errors.length > 0 && (
              <>
                {' '}另有 <strong>{preview.errors.length}</strong> 条错误将被跳过。
              </>
            )}
          </p>
          <div style={{ display: 'flex', gap: 10, marginTop: 16 }}>
            <Button
              variant="secondary"
              onClick={() => setStep(2)}
              disabled={executeMutation.isPending}
              style={{ flex: 1 }}
            >
              返回
            </Button>
            <Button
              loading={executeMutation.isPending}
              onClick={handleConfirmImport}
              style={{ flex: 1 }}
            >
              确认导入
            </Button>
          </div>
        </div>
      )}
    </Modal>
  )
}
