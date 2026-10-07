import api from './api'
import { logout } from './httputil'
import { push } from 'notivue'
import { i18n } from '@/locales'

// The file name a Content-Disposition header gives, preferring the UTF-8 one,
// stripped of anything that could point outside the downloads folder.
export function filenameFromDisposition(header: string | null | undefined, fallback: string): string {
  let name = ''
  if (header) {
    const star = /filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/.exec(header)
    if (star) {
      try {
        name = decodeURIComponent(star[1].trim().replace(/^"|"$/g, ''))
      } catch {
        name = ''
      }
    }
    if (!name) {
      const plain = /filename\s*=\s*"([^"]*)"/i.exec(header) ?? /filename\s*=\s*([^;]+)/i.exec(header)
      name = plain?.[1]?.trim() ?? ''
    }
  }
  name = name.replace(/[/\\]/g, '_').replace(/^\.+/, '').trim()
  return name || fallback
}

interface ErrorBody {
  msg?: string
}

async function blobMessage(data: unknown): Promise<string> {
  if (!(data instanceof Blob)) return ''
  try {
    const body = JSON.parse(await data.text()) as ErrorBody
    return body?.msg ?? ''
  } catch {
    return ''
  }
}

// downloadFile fetches a file from the panel and saves it. The server answers
// with its usual JSON envelope instead of the file when it fails, which a
// plain link would show as a page; this shows the error instead.
export async function downloadFile(url: string, params: Record<string, string | number>, fallback: string): Promise<boolean> {
  try {
    const resp = await api.get(url, { params, responseType: 'blob' })
    const type = String(resp.headers['content-type'] ?? '')
    if (type.includes('application/json')) {
      push.error({ title: i18n.global.t('failed'), message: await blobMessage(resp.data) })
      return false
    }
    const name = filenameFromDisposition(resp.headers['content-disposition'] as string | undefined, fallback)
    const href = URL.createObjectURL(resp.data as Blob)
    const a = document.createElement('a')
    a.href = href
    a.download = name
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(href), 60_000)
    return true
  } catch (e: unknown) {
    const err = e as { response?: { status?: number; data?: unknown } }
    const status = err?.response?.status
    if (status === 401 || status === 403) {
      push.error({ title: i18n.global.t('invalidLogin') })
      logout()
      return false
    }
    push.error({ title: i18n.global.t('failed'), message: (await blobMessage(err?.response?.data)) || String(e) })
    return false
  }
}
