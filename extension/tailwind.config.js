/** @type {import('tailwindcss').Config} */
export default {
  content: [
    './entrypoints/**/*.{ts,tsx,html}',
    './components/**/*.{ts,tsx}',
  ],
  // 不使用前缀，通过 CSS 作用域和 important 来隔离
  theme: {
    extend: {},
  },
  plugins: [],
}
