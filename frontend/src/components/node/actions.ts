import HttpUtils from '@/plugins/httputil'
import Data from '@/store/modules/data'
import { push } from 'notivue'
import { i18n } from '@/locales'
import { nodeLoginFailed, nodeLoginTotp, nodeNoUpdater, nodeUpdateTooOld, versionLabel, type NodeAction, type NodeActionResult } from '@/types/node'

// One request at a time: the HTTP client drops a request when an identical
// one starts, and every action posts to the same address.
let queue: Promise<unknown> = Promise.resolve()

// runNodeAction runs an action on some nodes, says how it went, and reloads
// the nodes. It answers null when the request itself failed, which the HTTP
// client has already reported.
// extra carries the login a panel update uses for the nodes older than v34.
export function runNodeAction(action: NodeAction, ids: number[], extra: Record<string, string> = {}): Promise<NodeActionResult[] | null> {
  const next = queue.then(async () => {
    // The language is the one a node writes the log of a panel update in.
    const msg = await HttpUtils.post<NodeActionResult[]>('api/nodeAction', { ...extra, ids: JSON.stringify(ids), action, lang: i18n.global.locale.value })
    if (!msg.success) return null
    const results = msg.obj ?? []
    reportNodeAction(action, results)
    const store = Data()
    store.lastLoad = 0
    await store.loadData()
    return results
  })
  queue = next.catch(() => undefined)
  return next
}

// nodeUpdateNote says how a node took a panel update that went well.
export function nodeUpdateNote(note?: string): string {
  const t = i18n.global.t
  if (note === 'upToDate') return t('node.update.upToDate')
  if (note === 'running') return t('node.update.alreadyRunning')
  return t('node.update.started', { version: versionLabel((note ?? '').replace(/^v/, '')) })
}

// nodeActionError is a node's error in the panel's language where it is known.
export function nodeActionError(error?: string): string {
  const e = (error ?? '').trim()
  if (e === nodeUpdateTooOld) return i18n.global.t('node.update.tooOld')
  if (e === nodeLoginTotp) return i18n.global.t('node.update.totp')
  if (e === nodeNoUpdater) return i18n.global.t('node.update.noUpdater')
  if (e.startsWith(nodeLoginFailed)) return i18n.global.t('node.update.loginFailed', { why: e.slice(nodeLoginFailed.length) })
  const unsupported = /^this panel cannot update itself: (\w+)$/.exec(e)
  if (unsupported) return i18n.global.t('node.update.unsupported', { why: i18n.global.t('panelUpdate.unsupported.' + unsupported[1]) })
  return e
}

export function reportNodeAction(action: NodeAction, results: NodeActionResult[]): void {
  const failed = results.filter(r => !r.ok)
  const title = i18n.global.t('node.action.' + action)
  // A panel update says what each node does: they do not all update.
  const notes = action === 'updatePanel' ? results.filter(r => r.ok).map(r => `${r.name}: ${nodeUpdateNote(r.note)}`) : []
  if (failed.length === 0) {
    push.success({
      title,
      message: [i18n.global.t('node.action.done', { n: results.length }), ...notes].join(' · '),
      duration: notes.length ? 10000 : 5000,
    })
    return
  }
  push.error({
    title,
    message: [
      i18n.global.t('node.action.partial', { ok: results.length - failed.length, failed: failed.length }),
      ...failed.map(f => `${f.name}: ${nodeActionError(f.error)}`),
      ...notes,
    ].join(' · '),
    duration: 12000,
  })
}
