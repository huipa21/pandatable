import { readFile } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'

/** @param {unknown} value */
export function messageKeys(value, prefix = '') {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`Expected a message object at ${prefix || '<root>'}`)
  }
  /** @type {string[]} */
  const keys = []
  for (const [key, child] of Object.entries(value)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (!key || key.includes('.'))
      throw new Error(`Invalid message key: ${path}`)
    if (typeof child === 'string' && child.trim()) keys.push(path)
    else keys.push(...messageKeys(child, path))
  }
  return keys.sort()
}

/** @param {unknown} en @param {unknown} zh */
export function checkParity(en, zh) {
  const enKeys = messageKeys(en)
  const zhKeys = messageKeys(zh)
  const missingZh = enKeys.filter((key) => !zhKeys.includes(key))
  const missingEn = zhKeys.filter((key) => !enKeys.includes(key))
  if (missingZh.length || missingEn.length) {
    throw new Error(
      `i18n key mismatch: missing zh-CN [${missingZh.join(', ')}]; missing en [${missingEn.join(', ')}]`,
    )
  }
  return enKeys.length
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  const en = JSON.parse(
    await readFile(new URL('../src/i18n/en.json', import.meta.url), 'utf8'),
  )
  const zh = JSON.parse(
    await readFile(new URL('../src/i18n/zh-CN.json', import.meta.url), 'utf8'),
  )
  console.log(`i18n: ${checkParity(en, zh)} matching, non-empty message keys`)
}
