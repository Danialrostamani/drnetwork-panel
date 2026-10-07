import { i18n } from '@/locales'
import type { NodeWarning } from '@/types/node'

// The sentence for a threshold a node is past.
export function warningText(w: NodeWarning): string {
  const value = Math.round(w.value)
  const limit = Math.round(w.limit)
  switch (w.key) {
    case 'cpu':
    case 'mem':
    case 'disk':
    case 'ping':
      return i18n.global.t(`node.warn.${w.key}`, { value, limit })
    case 'cert':
      if (w.value < 0) return i18n.global.t('node.warn.certExpired')
      if (w.value < 1) return i18n.global.t('node.warn.certToday')
      return i18n.global.t('node.warn.cert', { days: Math.floor(w.value) })
    case 'version':
      return i18n.global.t('node.warn.version', { version: w.info ?? '' })
    default:
      return w.key
  }
}
