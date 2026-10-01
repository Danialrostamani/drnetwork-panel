// A managed node is another S-UI panel reached through its token-authenticated API v2.
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
