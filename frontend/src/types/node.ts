// A managed node is another DrNetwork panel reached through its token-authenticated API v2.
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
}

export type NodeState = 'online' | 'offline' | 'core-stopped'

export interface NodeStatus {
  state: NodeState
  latency: number
  cpu: number
  mem: { current: number; total: number }
  appVersion: string
  coreVersion: string
  error?: string
  checkedAt: number
  lastOnline: number
}

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
// counted as down either.
export function nodesHealth(nodes: Node[], statuses: Record<number, NodeStatus | undefined>): NodesHealth {
  const enabled = nodes.filter(n => n.enable)
  let online = 0
  let down = 0
  for (const node of enabled) {
    const state = statuses[node.id]?.state
    if (state === 'online') online++
    else if (state === 'offline') down++
  }
  const color = down > 0 ? 'error' : online === enabled.length ? 'success' : 'warning'
  return { total: enabled.length, online, color }
}
