// 兼容后端两种错误格式: {error: "string"} 和 {error: {code, message}}
export function getApiError(err: unknown): string {
  if (!(err && typeof err === 'object')) return '未知错误';
  const data = (err as any)?.response?.data;
  if (!data) return (err as any)?.message || '请求失败';
  if (typeof data.error === 'string') return data.error;
  if (data.error?.message) return data.error.message;
  if (data.message) return data.message;
  return '请求失败';
}
