import { WgPeer } from '@/types/endpoints'

// What a WireGuard .conf file (wg-quick format) carries that a sing-box
// WireGuard endpoint can use. DNS has no place in the endpoint; it is returned
// so the form can show it, not written into the endpoint.
export interface WgConf {
  private_key: string
  address: string[]
  listen_port?: number
  mtu?: number
  dns?: string[]
  peers: WgPeer[]
}

// The reasons a text is not a usable .conf, as locale keys under types.wg.
export class WgConfError extends Error {
  constructor(public key: 'importNoInterface' | 'importNoKey' | 'importNoAddress' | 'importBadPeer' | 'importBadEndpoint') {
    super(key)
  }
}

const list = (v: string) => v.split(',').map(s => s.trim()).filter(s => s.length > 0)

// "1,2,3", "[1, 2, 3]" or the three bytes in base64, as Cloudflare WARP
// configurations write them.
function parseReserved(v: string): number[] | undefined {
  const s = v.trim().replace(/^\[|\]$/g, '')
  if (/^\d+(\s*,\s*\d+)*$/.test(s)) {
    const nums = s.split(',').map(n => parseInt(n.trim()))
    return nums.every(n => n >= 0 && n <= 255) ? nums : undefined
  }
  try {
    const bytes = atob(s)
    return bytes.length > 0 ? [...bytes].map(c => c.charCodeAt(0)) : undefined
  } catch {
    return undefined
  }
}

// "host:port", "[v6]:port" or "v6" without a port is not accepted: a peer to
// dial needs both.
function parseEndpoint(v: string): { address: string, port: number } {
  const s = v.trim()
  let host: string
  let port: string
  const v6 = /^\[([^\]]+)\]:(\d+)$/.exec(s)
  if (v6) {
    host = v6[1]
    port = v6[2]
  } else {
    const i = s.lastIndexOf(':')
    if (i <= 0 || s.indexOf(':') != i) throw new WgConfError('importBadEndpoint')
    host = s.slice(0, i)
    port = s.slice(i + 1)
  }
  const n = parseInt(port)
  if (!host || !/^\d+$/.test(port) || n < 1 || n > 65535) throw new WgConfError('importBadEndpoint')
  return { address: host, port: n }
}

// parseWgConf reads a wg-quick .conf: one [Interface] and any number of
// [Peer] sections. Keys are matched without regard to case, comments (# and ;)
// and unknown keys are skipped.
export function parseWgConf(text: string): WgConf {
  const out: WgConf = { private_key: '', address: [], peers: [] }
  let section = ''
  let sawInterface = false
  let peer: Record<string, string> | null = null
  const peers: Record<string, string>[] = []
  for (const raw of text.replace(/^\uFEFF/, '').split(/\r?\n/)) {
    const line = raw.replace(/\s[#;].*$/, '').trim()
    if (line.length == 0 || line.startsWith('#') || line.startsWith(';')) continue
    const head = /^\[(\w+)\]$/.exec(line)
    if (head) {
      section = head[1].toLowerCase()
      if (section == 'interface') sawInterface = true
      if (section == 'peer') {
        peer = {}
        peers.push(peer)
      }
      continue
    }
    const eq = line.indexOf('=')
    if (eq <= 0) continue
    const key = line.slice(0, eq).trim().toLowerCase()
    const value = line.slice(eq + 1).trim()
    if (section == 'interface') {
      switch (key) {
        case 'privatekey': out.private_key = value; break
        case 'address': out.address.push(...list(value)); break
        case 'listenport': if (/^\d+$/.test(value)) out.listen_port = parseInt(value); break
        case 'mtu': if (/^\d+$/.test(value)) out.mtu = parseInt(value); break
        case 'dns': out.dns = [...(out.dns ?? []), ...list(value)]; break
      }
    } else if (section == 'peer' && peer) {
      peer[key] = value
    }
  }
  if (!sawInterface) throw new WgConfError('importNoInterface')
  if (!out.private_key) throw new WgConfError('importNoKey')
  if (out.address.length == 0) throw new WgConfError('importNoAddress')
  for (const p of peers) {
    if (!p.publickey) throw new WgConfError('importBadPeer')
    const entry = <WgPeer>{ public_key: p.publickey }
    if (p.endpoint) Object.assign(entry, parseEndpoint(p.endpoint))
    if (p.presharedkey) entry.pre_shared_key = p.presharedkey
    if (p.allowedips) entry.allowed_ips = list(p.allowedips)
    if (p.persistentkeepalive && /^\d+$/.test(p.persistentkeepalive) && parseInt(p.persistentkeepalive) > 0) {
      entry.persistent_keepalive_interval = parseInt(p.persistentkeepalive)
    }
    if (p.reserved) {
      const r = parseReserved(p.reserved)
      if (r) entry.reserved = r
    }
    out.peers.push(entry)
  }
  return out
}
