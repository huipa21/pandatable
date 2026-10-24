import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { gzipSync } from 'node:zlib'

/** @type {Record<string, {file: string, imports?: string[], css?: string[]}>} */
const manifest = JSON.parse(readFileSync('dist/.vite/manifest.json', 'utf8'))
const initialFiles = new Set(['index.html'])
/** @param {string} key */
function include(key) {
  const chunk = manifest[key]
  if (!chunk) throw new Error(`Missing bundle entry: ${key}`)
  if (initialFiles.has(chunk.file)) return
  initialFiles.add(chunk.file)
  for (const css of chunk.css ?? []) initialFiles.add(css)
  for (const imported of chunk.imports ?? []) include(imported)
}
// Conservatively include both locales and the welcome route's async chunks.
for (const entry of [
  'index.html',
  'src/i18n/en.json',
  'src/i18n/zh-CN.json',
  'src/pages/LoginPage.vue',
]) {
  include(entry)
}
const initial = [...initialFiles].reduce(
  (n, file) => n + gzipSync(readFileSync(join('dist', file))).length,
  0,
)
const all = readdirSync('dist/assets').reduce(
  (n, file) => n + gzipSync(readFileSync(join('dist/assets', file))).length,
  0,
)
console.log(
  `Bundle gzip: conservative initial shell ${initial} bytes / <600000; all assets ${all} bytes (includes lazy grid and optional plugins)`,
)
if (initial >= 600_000) throw new Error('Initial bundle budget exceeded')
if ([...initialFiles].some((file) => /DataGrid|revo-grid|revogr-/.test(file))) {
  throw new Error('Grid engine leaked into the initial shell')
}
