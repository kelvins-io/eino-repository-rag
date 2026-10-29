// 与 internal/service/link_import.go 的扩展名和长度限制保持一致。
// 运行时优先使用 /api/v1/system/upload-limits 返回的 allowed_extensions。

export const FALLBACK_IMPORT_EXTENSIONS = [
  '.pdf',
  '.docx',
  '.xlsx',
  '.pptx',
  '.html',
  '.htm',
  '.md',
  '.markdown',
  '.txt',
  '.csv',
  '.json',
  '.png',
  '.jpg',
  '.jpeg',
  '.webp',
  '.tif',
  '.tiff',
  '.bmp',
  '.gif',
]

export const TITLE_MAX_BYTES = 256
export const URL_MAX_BYTES = 1024
export const DEFAULT_MAX_IMPORT_BATCH = 20

function byteLength(value) {
  return new TextEncoder().encode(value).length
}

function normalizeExtensions(extensions) {
  const list = Array.isArray(extensions) && extensions.length ? extensions : FALLBACK_IMPORT_EXTENSIONS
  return new Set(
    list
      .map((ext) => {
        const value = String(ext || '').trim().toLowerCase()
        if (!value) return ''
        return value.startsWith('.') ? value : `.${value}`
      })
      .filter(Boolean),
  )
}

function pathExt(name) {
  const index = name.lastIndexOf('.')
  if (index < 0) return ''
  return name.slice(index).toLowerCase()
}

function isYoudaoNoteHost(hostname) {
  const host = String(hostname || '').toLowerCase()
  return (
    host === 'note.youdao.com' ||
    host === 'share.note.youdao.com' ||
    host === 'note.youdao.cn' ||
    host === 'share.note.youdao.cn' ||
    host.endsWith('.note.youdao.com') ||
    host.endsWith('.note.youdao.cn')
  )
}

/** 有道云笔记公开分享链接：短链 /s/{id}，或带 id 的 noteshare / share 页面。 */
export function parseYoudaoNoteLink(raw) {
  const value = typeof raw === 'string' ? raw.trim() : ''
  if (!value || /\s/.test(value)) return null
  let url
  try {
    url = new URL(value)
  } catch {
    return null
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
  if (!isYoudaoNoteHost(url.hostname)) return null

  const queryID = (url.searchParams.get('id') || url.searchParams.get('shareKey') || url.searchParams.get('noteid') || '').trim()
  const path = (url.pathname || '').replace(/\/+$/, '') || '/'
  const short = path.match(/^\/s\/([A-Za-z0-9_-]{4,64})$/i)
  const lowerPath = path.toLowerCase()
  const sharePage =
    lowerPath.includes('noteshare') ||
    lowerPath.includes('ynoteshare') ||
    lowerPath.endsWith('/share') ||
    lowerPath.includes('/share/')

  let key = ''
  if (short) {
    key = short[1]
  } else if (/^[a-f0-9]{32}$/i.test(queryID) || /^[A-Za-z0-9_-]{4,64}$/.test(queryID)) {
    key = queryID
  }

  if (short) {
    return { fileName: `youdao-${key}.md`, youdao: true }
  }
  if (sharePage || queryID) {
    if (!key) return { error: 'invalid' }
    return { fileName: `youdao-${key}.md`, youdao: true }
  }
  return { error: 'invalid' }
}

/** 校验文档链接。扩展名只取路径，查询参数和片段不参与判断。有道云笔记分享链接除外。 */
export function parseDocumentLink(raw, extensions) {
  const youdao = parseYoudaoNoteLink(raw)
  if (youdao) {
    if (youdao.error) return { error: 'invalid' }
    return { fileName: youdao.fileName }
  }
  const allowed = normalizeExtensions(extensions)
  const value = typeof raw === 'string' ? raw.trim() : ''
  if (!value || /\s/.test(value)) return { error: 'invalid' }
  let url
  try {
    url = new URL(value)
  } catch {
    return { error: 'invalid' }
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return { error: 'invalid' }
  if (!url.hostname) return { error: 'invalid' }
  let pathname = url.pathname || ''
  try {
    pathname = decodeURIComponent(pathname)
  } catch {
    return { error: 'invalid' }
  }
  const base = pathname.split('/').filter(Boolean).pop() || ''
  if (!base || base === '.' || base === '..') return { error: 'invalid' }
  const ext = pathExt(base)
  if (!ext || ext === '.') return { error: 'invalid' }
  if (!allowed.has(ext)) return { error: 'type', ext }
  return { fileName: base }
}

export function validateDocumentLinks(rows, { extensions, maxFiles } = {}) {
  const list = Array.isArray(rows) ? rows : []
  const limit = maxFiles > 0 ? maxFiles : DEFAULT_MAX_IMPORT_BATCH
  if (!list.length) return { code: 'empty' }
  if (list.length > limit) return { code: 'tooMany', count: limit }
  const seen = new Map()
  for (let i = 0; i < list.length; i += 1) {
    const title = String(list[i]?.title || '').trim()
    const link = String(list[i]?.url || '').trim()
    const index = i + 1
    if (!title) return { code: 'needTitle', index }
    if (byteLength(title) > TITLE_MAX_BYTES) return { code: 'titleLong', index }
    if (!link) return { code: 'needUrl', index }
    if (byteLength(link) > URL_MAX_BYTES) return { code: 'urlLong', index }
    const parsed = parseDocumentLink(link, extensions)
    if (parsed.error === 'type') return { code: 'type', index, ext: parsed.ext }
    if (parsed.error) return { code: 'invalid', index }
    if (seen.has(link)) return { code: 'duplicate', index, other: seen.get(link) }
    seen.set(link, index)
  }
  return null
}
