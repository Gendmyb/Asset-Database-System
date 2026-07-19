/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: { extend: {} },
  plugins: [],
  // 禁用 preflight 重置: 项目使用内联样式 + CSS 变量, 仅需 Tailwind 工具类 (响应式)。
  corePlugins: { preflight: false },
}
