// A managed node is another DrNetwork panel reached through its token-authenticated API v2.

// Telegram alert thresholds. A missing (null) value uses the default; zero
// turns the alert off.
export interface NodeAlerts {
  cpu?: number | null
  mem?: number | null
  disk?: number | null
  ping?: number | null
  certDays?: number | null
  version?: boolean | null
}

export type AlertKey = 'cpu' | 'mem' | 'disk' | 'ping' | 'certDays'

// What the master uses for a threshold left empty, and the largest it takes.
export const alertDefaults: Record<AlertKey, number> = { cpu: 90, mem: 90, disk: 90, ping: 0, certDays: 7 }
export const alertMax: Record<AlertKey, number> = { cpu: 100, mem: 100, disk: 100, ping: 60000, certDays: 365 }

export type CapMode = 'total' | 'up' | 'down'

// The traffic a node's server may move in one monthly cycle; a zero limit
// means no cap.
export interface NodeCap {
  limit: number
  day: number
  mode: CapMode
  hide: boolean
}

// The clients a node serves; both lists empty serves every client.
export interface NodeAccess {
  groups: string[]
  clients: number[]
}

// What the last sync of a node did.
export interface NodeSyncReport {
  at: number
  duration: number
  trigger: string
  added: string[]
  edited: string[]
  deleted: string[]
  addedCount: number
  editedCount: number
  deletedCount: number
  unchanged: number
  skipped: number
  error?: string
}

export interface Node {
  id: number
  enable: boolean
  name: string
  baseUrl: string
  webPath: string
  token?: string
  tokenSet?: boolean
  insecure: boolean
  certPin?: string
  desc?: string
  lastSeen: number
  dirty?: boolean
  lastSync?: number
  tags?: string[]
  country?: string
  sortOrder?: number
  alerts?: NodeAlerts
  cap?: NodeCap
  hideDown?: boolean
  access?: NodeAccess
  // Filled in by the master, never saved.
  inboundCount?: number
  clientCount?: number
  syncReport?: NodeSyncReport
}

export type NodeState = 'online' | 'offline' | 'core-stopped'

export interface NodeMem {
  current: number
  total: number
}

export interface NodeWarning {
  // cpu, mem, disk, ping, cert or version.
  key: string
  value: number
  limit: number
  info?: string
}

// What a node's server moved, as its network interfaces count it: up is what
// it sent, down what it received.
export interface NodeTrafficSummary {
  todayUp: number
  todayDown: number
  monthUp: number
  monthDown: number
  totalUp: number
  totalDown: number
  capLimit?: number
  capUsed?: number
  capStart?: number
  capEnd?: number
}

// What the last probe found on a node. Everything past the first fields is
// optional: older nodes do not report it, and a node not probed yet has
// nothing.
export interface NodeStatus {
  state: NodeState
  latency: number
  cpu: number
  mem: NodeMem
  appVersion: string
  coreVersion: string
  error?: string
  checkedAt: number
  lastOnline: number
  disk?: NodeMem
  swap?: NodeMem
  appFull?: string
  hostName?: string
  cpuType?: string
  cpuCount?: number
  ipv4?: string[]
  ipv6?: string[]
  bootTime?: number
  coreUptime?: number
  // Bytes per second.
  netUp?: number
  netDown?: number
  online?: number
  maintenance?: boolean
  certExpiry?: number
  downSince?: number
  // Why the node's links are out of the subscriptions.
  hidden?: '' | 'down' | 'cap' | 'filtered'
  filtered?: boolean
  warnings?: NodeWarning[]
  // Percent online; -1 without history.
  uptime24?: number
  uptime7d?: number
  traffic?: NodeTrafficSummary
}

export const defaultCap: NodeCap = { limit: 0, day: 1, mode: 'total', hide: false }

export const defaultNode: Node = {
  id: 0,
  enable: true,
  name: '',
  baseUrl: '',
  webPath: '/app/',
  token: '',
  insecure: false,
  certPin: '',
  desc: '',
  lastSeen: 0,
  tags: [],
  country: '',
  sortOrder: 0,
  alerts: { cpu: null, mem: null, disk: null, ping: null, certDays: null, version: null },
  cap: { ...defaultCap },
  hideDown: false,
  access: { groups: [], clients: [] },
}

// A fresh copy of defaultNode, sharing no object with it.
export function newNode(): Node {
  return JSON.parse(JSON.stringify(defaultNode)) as Node
}

// A node with every setting present, as the editor holds it.
export type EditableNode = Node & { tags: string[]; alerts: NodeAlerts; cap: NodeCap; access: NodeAccess }

// The node as the editor holds it: every setting present, defaults filled in.
export function editableNode(item?: Node | null): EditableNode {
  const base = newNode() as EditableNode
  if (!item) return base
  const copy = JSON.parse(JSON.stringify(item)) as Node
  return {
    ...base,
    ...copy,
    token: '',
    tags: copy.tags ?? [],
    alerts: { ...base.alerts, ...(copy.alerts ?? {}) },
    cap: { ...defaultCap, ...(copy.cap ?? {}) },
    access: { groups: copy.access?.groups ?? [], clients: copy.access?.clients ?? [] },
  }
}

// What a save sends: the settings, without what the master fills in itself.
export function nodePayload(node: Node): Node {
  const out = JSON.parse(JSON.stringify(node)) as Node
  delete out.inboundCount
  delete out.clientCount
  delete out.syncReport
  delete out.tokenSet
  delete out.dirty
  delete out.lastSync
  return out
}

// A new node that starts from the settings of another: a new server needs its
// own address, token and certificate pin.
export function cloneNode(node: Node, takenNames: string[]): EditableNode {
  const copy = editableNode(node)
  copy.id = 0
  copy.baseUrl = ''
  copy.certPin = ''
  copy.lastSeen = 0
  delete copy.tokenSet
  delete copy.dirty
  delete copy.lastSync
  delete copy.inboundCount
  delete copy.clientCount
  delete copy.syncReport
  copy.name = uniqueName(`${node.name}-copy`, takenNames)
  return copy
}

export function uniqueName(name: string, taken: string[]): string {
  const used = new Set(taken)
  if (!used.has(name)) return name
  for (let i = 2; ; i++) {
    if (!used.has(`${name}-${i}`)) return `${name}-${i}`
  }
}

export interface NodesHealth {
  // Enabled nodes: only those are probed.
  total: number
  online: number
  // Red when a node is down, amber when one is not (yet) known to be up, else green.
  color: 'success' | 'warning' | 'error'
}

// How many of the enabled nodes answer, for the badge in the top bar. A node
// nobody has probed yet, or whose core is stopped, is not online but is not
// counted as down either. A node held in maintenance is stopped on purpose,
// so it does not turn the badge amber.
export function nodesHealth(nodes: Node[], statuses: Record<number, NodeStatus | undefined>): NodesHealth {
  const enabled = nodes.filter(n => n.enable)
  let online = 0
  let down = 0
  let resting = 0
  for (const node of enabled) {
    const status = statuses[node.id]
    if (status?.state === 'online') online++
    else if (status?.state === 'offline') down++
    else if (status?.state === 'core-stopped' && status.maintenance) resting++
  }
  const color = down > 0 ? 'error' : online + resting === enabled.length ? 'success' : 'warning'
  return { total: enabled.length, online, color }
}

// How the Nodes page shows a node.
export type NodeView = NodeState | 'maintenance' | 'disabled' | 'pending'

export function nodeView(node: Node, status?: NodeStatus): NodeView {
  if (!node.enable) return 'disabled'
  if (!status) return 'pending'
  if (status.state === 'core-stopped' && status.maintenance) return 'maintenance'
  return status.state
}

export const viewColor: Record<NodeView, string> = {
  online: 'success', offline: 'error', 'core-stopped': 'warning', maintenance: 'info', disabled: 'default', pending: 'default',
}
export const viewIcon: Record<NodeView, string> = {
  online: 'mdi-check-circle', offline: 'mdi-alert-circle', 'core-stopped': 'mdi-pause-circle',
  maintenance: 'mdi-wrench-clock', disabled: 'mdi-cancel', pending: 'mdi-clock-outline',
}
// The locale key of a view's name.
export function viewLabelKey(view: NodeView): string {
  switch (view) {
    case 'disabled': return 'disable'
    case 'core-stopped': return 'node.status.coreStopped'
    default: return `node.status.${view}`
  }
}

// Percent of a memory, a disk or a swap in use; null when unknown.
export function usage(m?: NodeMem | null): number | null {
  if (!m || !(m.total > 0)) return null
  return Math.min(100, Math.max(0, (m.current * 100) / m.total))
}

// Percent of the monthly cap used; null without a cap. It may pass 100.
export function capPercent(t?: NodeTrafficSummary | null): number | null {
  if (!t || !t.capLimit || t.capLimit <= 0) return null
  return Math.max(0, ((t.capUsed ?? 0) * 100) / t.capLimit)
}

// Uptime as the master reports it: null while there is no history.
export function uptimeOf(v?: number | null): number | null {
  return v == null || v < 0 ? null : v
}

export const GB = 1024 ** 3
export function bytesToGb(bytes: number): number {
  return Math.round(((bytes || 0) / GB) * 100) / 100
}
export function gbToBytes(gb: number): number {
  return gb > 0 ? Math.round(gb * GB) : 0
}

// A threshold typed in the editor: empty uses the default.
export function alertValue(v: unknown): number | null {
  if (v === '' || v == null) return null
  const n = Number(v)
  return Number.isFinite(n) ? Math.round(n) : null
}

export interface NodesSummary {
  total: number
  enabled: number
  online: number
  offline: number
  coreStopped: number
  maintenance: number
  disabled: number
  pending: number
  // Of the nodes online: users connected and bytes per second.
  users: number
  netUp: number
  netDown: number
  // What the enabled nodes moved today.
  todayUp: number
  todayDown: number
  warnings: number
  hidden: number
}

export function nodesSummary(nodes: Node[], statuses: Record<number, NodeStatus | undefined>): NodesSummary {
  const s: NodesSummary = {
    total: nodes.length, enabled: 0, online: 0, offline: 0, coreStopped: 0, maintenance: 0, disabled: 0, pending: 0,
    users: 0, netUp: 0, netDown: 0, todayUp: 0, todayDown: 0, warnings: 0, hidden: 0,
  }
  for (const node of nodes) {
    const status = statuses[node.id]
    const view = nodeView(node, status)
    switch (view) {
      case 'online': s.online++; break
      case 'offline': s.offline++; break
      case 'core-stopped': s.coreStopped++; break
      case 'maintenance': s.maintenance++; break
      case 'disabled': s.disabled++; break
      case 'pending': s.pending++; break
    }
    if (!node.enable) continue
    s.enabled++
    if (view === 'online' && status) {
      s.users += status.online ?? 0
      s.netUp += status.netUp ?? 0
      s.netDown += status.netDown ?? 0
    }
    s.todayUp += status?.traffic?.todayUp ?? 0
    s.todayDown += status?.traffic?.todayDown ?? 0
    if ((status?.warnings?.length ?? 0) > 0) s.warnings++
    if (status?.hidden) s.hidden++
  }
  return s
}

export type NodeStateFilter = NodeView | 'all' | 'problem'

export interface NodeFilter {
  text: string
  state: NodeStateFilter
  tag: string
  country: string
}

export const emptyFilter: NodeFilter = { text: '', state: 'all', tag: '', country: '' }

// A node that wants a look: down, stopped without maintenance, over a
// threshold or out of the subscriptions.
export function needsAttention(node: Node, status?: NodeStatus): boolean {
  if (!node.enable) return false
  const view = nodeView(node, status)
  return view === 'offline' || view === 'core-stopped' || (status?.warnings?.length ?? 0) > 0 || !!status?.hidden
}

export function matchesFilter(node: Node, status: NodeStatus | undefined, f: NodeFilter): boolean {
  if (f.state === 'problem') {
    if (!needsAttention(node, status)) return false
  } else if (f.state !== 'all' && nodeView(node, status) !== f.state) {
    return false
  }
  if (f.tag && !(node.tags ?? []).some(t => t.toLowerCase() === f.tag.toLowerCase())) return false
  if (f.country && (node.country ?? '').toUpperCase() !== f.country.toUpperCase()) return false
  const words = f.text.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length > 0) {
    const hay = [
      node.name, node.baseUrl, node.webPath, node.desc ?? '', node.country ?? '', ...(node.tags ?? []),
      status?.hostName ?? '', ...(status?.ipv4 ?? []), ...(status?.ipv6 ?? []), status?.appFull || status?.appVersion || '',
    ].join('\n').toLowerCase()
    if (!words.every(w => hay.includes(w))) return false
  }
  return true
}

export type NodeSortKey = 'order' | 'name' | 'status' | 'latency' | 'cpu' | 'mem' | 'disk' | 'online' | 'traffic' | 'uptime'
export const nodeSortKeys: NodeSortKey[] = ['order', 'name', 'status', 'latency', 'cpu', 'mem', 'disk', 'online', 'traffic', 'uptime']

// Nodes that need a look come first when sorting by status.
const viewRank: Record<NodeView, number> = { offline: 0, 'core-stopped': 1, pending: 2, maintenance: 3, online: 4, disabled: 5 }

function sortValue(key: NodeSortKey, node: Node, status?: NodeStatus): number | string | null {
  const live = status?.state === 'online' ? status : undefined
  switch (key) {
    case 'name': return node.name
    case 'status': return viewRank[nodeView(node, status)]
    case 'latency': return live ? live.latency : null
    case 'cpu': return live ? live.cpu : null
    case 'mem': return live ? usage(live.mem) : null
    case 'disk': return live ? usage(live.disk) : null
    case 'online': return live ? (live.online ?? 0) : null
    case 'traffic': return status?.traffic ? status.traffic.todayUp + status.traffic.todayDown : null
    case 'uptime': return uptimeOf(status?.uptime24)
    default: return node.sortOrder ?? 0
  }
}

// The nodes in the order asked for. A node with nothing to sort by (offline,
// say, when sorting by CPU) goes last either way; ties fall back to the order
// set on the nodes, then to the name.
export function sortNodes(nodes: Node[], statuses: Record<number, NodeStatus | undefined>, key: NodeSortKey, desc: boolean): Node[] {
  return [...nodes].sort((a, b) => {
    const va = sortValue(key, a, statuses[a.id])
    const vb = sortValue(key, b, statuses[b.id])
    if (va === null && vb !== null) return 1
    if (vb === null && va !== null) return -1
    if (va !== null && vb !== null) {
      const c = typeof va === 'string' || typeof vb === 'string'
        ? String(va).localeCompare(String(vb), undefined, { numeric: true, sensitivity: 'base' })
        : va - vb
      if (c !== 0) return desc ? -c : c
    }
    const o = (a.sortOrder ?? 0) - (b.sortOrder ?? 0)
    if (o !== 0) return o
    const n = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' })
    return n !== 0 ? n : a.id - b.id
  })
}

// Every tag in use, once, whatever its case.
export function nodeTags(nodes: Node[]): string[] {
  const seen = new Map<string, string>()
  for (const n of nodes) {
    for (const t of n.tags ?? []) {
      if (!seen.has(t.toLowerCase())) seen.set(t.toLowerCase(), t)
    }
  }
  return [...seen.values()].sort((a, b) => a.localeCompare(b))
}

export function nodeCountries(nodes: Node[]): string[] {
  return [...new Set(nodes.map(n => (n.country ?? '').toUpperCase()).filter(Boolean))].sort()
}

// The flag of a two-letter country code, as emoji.
export function flagEmoji(code?: string): string {
  const c = (code ?? '').trim().toUpperCase()
  if (!/^[A-Z]{2}$/.test(c)) return ''
  return String.fromCodePoint(...[...c].map(ch => 0x1f1e6 + ch.charCodeAt(0) - 65))
}

const tokenAlphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'

// A random string of letters and digits. Bytes past the largest multiple of
// the alphabet are thrown away, so every character is equally likely.
export function randomToken(length = 32, fill: (buf: Uint8Array) => Uint8Array = buf => crypto.getRandomValues(buf)): string {
  const limit = 256 - (256 % tokenAlphabet.length)
  let out = ''
  while (out.length < length) {
    for (const b of fill(new Uint8Array(length * 2))) {
      if (b >= limit) continue
      out += tokenAlphabet[b % tokenAlphabet.length]
      if (out.length === length) break
    }
  }
  return out
}

export const installScriptUrl = 'https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh'

// What the install script accepts; anything else must not reach a root shell.
export const validToken = (t: string) => /^[A-Za-z0-9]{16,128}$/.test(t)
export const validPort = (p: number) => Number.isInteger(p) && p >= 1 && p <= 65535
export const validPath = (p: string) => /^\/[A-Za-z0-9._~/-]*$/.test(p) && !p.includes('..')
export const validHost = (h: string) =>
  /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$/.test(h) ||
  (/^\[?[0-9A-Fa-f:.]+\]?$/.test(h) && h.includes(':'))

export function webPathOf(p: string): string {
  let out = p.trim()
  if (!out.startsWith('/')) out = '/' + out
  if (!out.endsWith('/')) out += '/'
  return out
}

// The command that installs a node for this master on a new server, or ''
// when a value would not be safe to run.
export function installCommand(o: { token: string; port: number; path: string }): string {
  if (!validToken(o.token) || !validPort(o.port) || !validPath(o.path)) return ''
  return `bash <(curl -Ls ${installScriptUrl}) --node-token ${o.token} --port ${o.port} --path ${o.path}`
}

// The address of a panel on host:port. A new panel answers over plain HTTP
// until it gets a certificate.
export function panelUrl(host: string, port: number, https = false): string {
  let h = host.trim()
  if (h.includes(':') && !h.startsWith('[')) h = `[${h}]`
  return `${https ? 'https' : 'http'}://${h}:${port}`
}

export interface MultiAddError {
  line: number
  // fields, name, url or duplicate.
  reason: string
  text: string
}

// Nodes typed one per line: name, URL, token and an optional web path,
// separated by spaces, tabs, commas or semicolons. A path in the URL is taken
// as the web path. Empty lines and lines starting with # are skipped.
export function parseMultiAdd(text: string, takenNames: string[], insecure = false): { nodes: Node[]; errors: MultiAddError[] } {
  const nodes: Node[] = []
  const errors: MultiAddError[] = []
  const names = new Set(takenNames)
  text.split(/\r?\n/).forEach((raw, i) => {
    const line = raw.trim()
    if (!line || line.startsWith('#')) return
    const fail = (reason: string) => errors.push({ line: i + 1, reason, text: line })
    const fields = line.split(/[\s,;]+/).filter(Boolean)
    if (fields.length < 3 || fields.length > 4) return fail('fields')
    const [name, address, token, path] = fields
    if (/[[\]]/.test(name)) return fail('name')
    let url: URL
    try {
      url = new URL(address)
    } catch {
      return fail('url')
    }
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return fail('url')
    if (names.has(name)) return fail('duplicate')
    names.add(name)
    const node = newNode()
    node.name = name
    node.baseUrl = url.origin
    node.webPath = webPathOf(path ?? (url.pathname && url.pathname !== '/' ? url.pathname : '/app/'))
    node.token = token
    node.insecure = insecure && url.protocol === 'https:'
    nodes.push(node)
  })
  return { nodes, errors }
}

export interface NodeHistoryPoint {
  t: number
  // Percent of the probes that found the node online; -1 when none counts.
  uptime: number
  latency: number
  cpu: number
  mem: number
  disk: number
  online: number
  sent: number
  recv: number
}

export interface NodeHistory {
  since: number
  bucket: number
  points: NodeHistoryPoint[]
}

// The history on a full time line: a bucket with no record is null, so the
// charts show a gap rather than a line drawn across it.
export function historySeries(h: NodeHistory, now: number): { times: number[]; points: (NodeHistoryPoint | null)[] } {
  if (!(h.bucket > 0)) return { times: h.points.map(p => p.t), points: [...h.points] }
  const byTime = new Map(h.points.map(p => [p.t, p]))
  const times: number[] = []
  const points: (NodeHistoryPoint | null)[] = []
  for (let t = h.since; t <= now && times.length < 5000; t += h.bucket) {
    times.push(t)
    points.push(byTime.get(t) ?? null)
  }
  return { times, points }
}

export interface NodeTrafficPoint {
  t: number
  up: number
  down: number
}

export interface NodeTrafficReport {
  period: string
  points: NodeTrafficPoint[]
  summary: NodeTrafficSummary
  clients: { name: string; up: number; down: number }[]
  clientsNote?: string
}

export interface NodeOutage {
  id: number
  start: number
  end: number
  state: string
  reason: string
}

export interface NodeOutageReport {
  outages: NodeOutage[]
  uptime24: number
  uptime7d: number
  down24: number
  down7d: number
  down30d: number
  count24: number
  count7d: number
  count30d: number
}

export interface NodeSyncPreview {
  add: string[]
  edit: string[]
  del: string[]
  same: number
  replicas: number
  missing: string[]
  restricted: boolean
  skipped: number
}

export interface NodeOnlines {
  user: string[]
  inbound: string[]
  outbound: string[]
  checkedAt: number
}

export interface NodeActionResult {
  id: number
  name: string
  ok: boolean
  error?: string
}

export type NodeAction = 'probe' | 'restartSb' | 'restartApp' | 'maintenanceOn' | 'maintenanceOff' | 'enable' | 'disable' | 'sync' | 'fullSync'

// Actions that stop something or rewrite a node ask first.
export const confirmActions: NodeAction[] = ['restartSb', 'restartApp', 'maintenanceOn', 'disable', 'fullSync']
