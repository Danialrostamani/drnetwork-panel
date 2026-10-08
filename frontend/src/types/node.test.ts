import { describe, it, expect } from 'vitest'
import {
  alertValue, bytesToGb, capPercent, cloneNode, defaultNode, editableNode, emptyFilter, flagEmoji, gbToBytes, historySeries,
  installCommand, matchesFilter, needsAttention, newNode, nodeCountries, nodePayload, nodeTags, nodeVersion, nodeView,
  nodesHealth, nodesSummary, panelUrl, parseMultiAdd, randomToken, sortNodes, uniqueName, usage, validHost, validPath,
  versionLabel,
  type Node, type NodeHistory, type NodeState, type NodeStatus,
} from './node'

const node = (id: number, enable = true): Node => ({ ...defaultNode, id, enable, name: `n${id}` })
const status = (state: NodeState): NodeStatus => ({
  state, latency: 1, cpu: 0, mem: { current: 0, total: 0 }, appVersion: '', coreVersion: '', checkedAt: 0, lastOnline: 0,
})

describe('nodesHealth', () => {
  it('is green when every enabled node answers', () => {
    const health = nodesHealth([node(1), node(2)], { 1: status('online'), 2: status('online') })
    expect(health).toEqual({ total: 2, online: 2, color: 'success' })
  })

  it('turns red as soon as one node is down', () => {
    const health = nodesHealth([node(1), node(2), node(3)], { 1: status('online'), 2: status('offline'), 3: status('core-stopped') })
    expect(health).toEqual({ total: 3, online: 1, color: 'error' })
  })

  it('does not call a node down that is stopped or not probed yet', () => {
    const health = nodesHealth([node(1), node(2)], { 1: status('online'), 2: status('core-stopped') })
    expect(health).toEqual({ total: 2, online: 1, color: 'warning' })
    expect(nodesHealth([node(1)], {})).toEqual({ total: 1, online: 0, color: 'warning' })
  })

  it('leaves disabled nodes out, whatever they last reported', () => {
    const health = nodesHealth([node(1), node(2, false)], { 1: status('online'), 2: status('offline') })
    expect(health).toEqual({ total: 1, online: 1, color: 'success' })
  })

  it('has nothing to say without nodes', () => {
    expect(nodesHealth([], {}).total).toBe(0)
    expect(nodesHealth([node(1, false)], {}).total).toBe(0)
  })
})

const full = (state: NodeState, extra: Partial<NodeStatus> = {}): NodeStatus => ({ ...status(state), ...extra })
const named = (id: number, name: string, extra: Partial<Node> = {}): Node => ({ ...newNode(), id, name, ...extra })

describe('a node held in maintenance', () => {
  it('counts as resting, not as a problem, in the badge', () => {
    const health = nodesHealth([node(1), node(2)], { 1: status('online'), 2: full('core-stopped', { maintenance: true }) })
    expect(health).toEqual({ total: 2, online: 1, color: 'success' })
  })

  it('has its own view, and only on a stopped core', () => {
    expect(nodeView(node(1), full('core-stopped', { maintenance: true }))).toBe('maintenance')
    expect(nodeView(node(1), full('core-stopped'))).toBe('core-stopped')
    expect(nodeView(node(1), full('online', { maintenance: true }))).toBe('online')
    expect(nodeView(node(1, false), full('online'))).toBe('disabled')
    expect(nodeView(node(1), undefined)).toBe('pending')
  })
})

describe('the summary bar', () => {
  it('adds up the live numbers of the nodes online only, and today of every enabled node', () => {
    const nodes = [node(1), node(2), node(3), node(4, false), node(5)]
    const traffic = (up: number, down: number) => ({ todayUp: up, todayDown: down, monthUp: 0, monthDown: 0, totalUp: 0, totalDown: 0 })
    const s = nodesSummary(nodes, {
      1: full('online', { online: 3, netUp: 10, netDown: 20, traffic: traffic(1, 2), warnings: [{ key: 'cpu', value: 95, limit: 90 }] }),
      2: full('offline', { online: 7, netUp: 99, netDown: 99, traffic: traffic(4, 8), hidden: 'down' }),
      3: full('core-stopped', { maintenance: true }),
      4: full('online', { online: 50, netUp: 50, netDown: 50, traffic: traffic(100, 100), warnings: [{ key: 'cpu', value: 1, limit: 1 }] }),
    })
    expect(s).toEqual({
      total: 5, enabled: 4, online: 1, offline: 1, coreStopped: 0, maintenance: 1, disabled: 1, pending: 1,
      users: 3, netUp: 10, netDown: 20, todayUp: 5, todayDown: 10, warnings: 1, hidden: 1,
    })
  })
})

describe('filtering the nodes', () => {
  const de = named(1, 'Frankfurt-1', { country: 'DE', tags: ['Hetzner', 'fast'], desc: 'main' })
  const nl = named(2, 'ams', { country: 'nl', tags: ['ovh'] })
  const st = { 1: full('online', { hostName: 'fra-box', ipv4: ['1.2.3.4'] }), 2: full('offline') }

  it('matches every word of the search anywhere, without regard to case', () => {
    const f = (text: string) => [de, nl].filter(n => matchesFilter(n, st[n.id as 1 | 2], { ...emptyFilter, text })).map(n => n.id)
    expect(f('frank')).toEqual([1])
    expect(f('HETZNER main')).toEqual([1])
    expect(f('hetzner ovh')).toEqual([])
    expect(f('1.2.3')).toEqual([1])
    expect(f('fra-box')).toEqual([1])
    expect(f('  ')).toEqual([1, 2])
  })

  it('filters by state, tag and country', () => {
    const f = (o: Partial<typeof emptyFilter>) => [de, nl].filter(n => matchesFilter(n, st[n.id as 1 | 2], { ...emptyFilter, ...o })).map(n => n.id)
    expect(f({ state: 'offline' })).toEqual([2])
    expect(f({ state: 'problem' })).toEqual([2])
    expect(f({ tag: 'FAST' })).toEqual([1])
    expect(f({ country: 'NL' })).toEqual([2])
    expect(f({ country: 'de', state: 'online' })).toEqual([1])
  })

  it('calls for attention on what an operator would fix', () => {
    expect(needsAttention(node(1), full('online'))).toBe(false)
    expect(needsAttention(node(1), full('online', { warnings: [{ key: 'disk', value: 95, limit: 90 }] }))).toBe(true)
    expect(needsAttention(node(1), full('online', { hidden: 'cap' }))).toBe(true)
    expect(needsAttention(node(1), full('core-stopped'))).toBe(true)
    expect(needsAttention(node(1), full('core-stopped', { maintenance: true }))).toBe(false)
    expect(needsAttention(node(1, false), full('offline'))).toBe(false)
  })

  it('lists the tags and countries in use once', () => {
    expect(nodeTags([de, nl, named(3, 'x', { tags: ['FAST', 'b'] })])).toEqual(['b', 'fast', 'Hetzner', 'ovh'])
    expect(nodeCountries([de, nl, named(3, 'x', { country: 'DE' }), named(4, 'y')])).toEqual(['DE', 'NL'])
  })
})

describe('sorting the nodes', () => {
  const a = named(1, 'node-10', { sortOrder: 2 })
  const b = named(2, 'node-9', { sortOrder: 1 })
  const c = named(3, 'Alpha', { sortOrder: 1 })
  const statuses = {
    1: full('online', { cpu: 50, latency: 30, mem: { current: 1, total: 4 }, uptime24: 99 }),
    2: full('offline', { cpu: 99, latency: 1 }),
    3: full('online', { cpu: 10, latency: 80, mem: { current: 3, total: 4 }, uptime24: -1 }),
  }
  const ids = (key: Parameters<typeof sortNodes>[2], desc = false) => sortNodes([a, b, c], statuses, key, desc).map(n => n.id)

  it('keeps the set order, then the name, by default', () => {
    expect(ids('order')).toEqual([3, 2, 1])
    expect(ids('order', true)).toEqual([1, 3, 2])
  })

  it('sorts names the way people count', () => {
    expect(ids('name')).toEqual([3, 2, 1])
    expect(ids('name', true)).toEqual([1, 2, 3])
  })

  it('puts a node with nothing to show last, in either direction', () => {
    expect(ids('cpu')).toEqual([3, 1, 2])
    expect(ids('cpu', true)).toEqual([1, 3, 2])
    expect(ids('latency')).toEqual([1, 3, 2])
    expect(ids('mem', true)).toEqual([3, 1, 2])
    expect(ids('uptime')).toEqual([1, 3, 2])
  })

  it('shows the nodes in trouble first by status', () => {
    expect(ids('status')).toEqual([2, 3, 1])
  })

  it('does not reorder the list it was given', () => {
    const list = [a, b, c]
    sortNodes(list, statuses, 'name', false)
    expect(list.map(n => n.id)).toEqual([1, 2, 3])
  })
})

describe('numbers on the cards', () => {
  it('reads usage and the cap', () => {
    expect(usage({ current: 1, total: 4 })).toBe(25)
    expect(usage({ current: 1, total: 0 })).toBeNull()
    expect(usage(undefined)).toBeNull()
    expect(usage({ current: 9, total: 4 })).toBe(100)
    const t = { todayUp: 0, todayDown: 0, monthUp: 0, monthDown: 0, totalUp: 0, totalDown: 0 }
    expect(capPercent(t)).toBeNull()
    expect(capPercent({ ...t, capLimit: 200, capUsed: 50 })).toBe(25)
    expect(capPercent({ ...t, capLimit: 100, capUsed: 150 })).toBe(150)
  })

  it('turns gigabytes into bytes and back', () => {
    expect(gbToBytes(1)).toBe(1024 ** 3)
    expect(gbToBytes(0)).toBe(0)
    expect(gbToBytes(-5)).toBe(0)
    expect(bytesToGb(gbToBytes(1.5))).toBe(1.5)
    expect(bytesToGb(0)).toBe(0)
  })

  it('reads a threshold field, empty meaning the default', () => {
    expect(alertValue('')).toBeNull()
    expect(alertValue(null)).toBeNull()
    expect(alertValue(undefined)).toBeNull()
    expect(alertValue('85')).toBe(85)
    expect(alertValue(0)).toBe(0)
    expect(alertValue('abc')).toBeNull()
  })

  it('draws the flag of a country code', () => {
    expect(flagEmoji('de')).toBe('🇩🇪')
    expect(flagEmoji('NL')).toBe('🇳🇱')
    expect(flagEmoji('')).toBe('')
    expect(flagEmoji('DEU')).toBe('')
  })
})

describe('editing a node', () => {
  const stored: Node = {
    ...named(5, 'tokyo', { tags: ['a'], country: 'JP', cap: { limit: 10, day: 3, mode: 'up', hide: true } }),
    tokenSet: true, dirty: true, lastSync: 9, inboundCount: 2, clientCount: 7,
    syncReport: { at: 1, duration: 1, trigger: 'auto', added: [], edited: [], deleted: [], addedCount: 0, editedCount: 0, deletedCount: 0, unchanged: 0, skipped: 0 },
  }

  it('fills in every setting an older record lacks', () => {
    const old = { id: 1, enable: true, name: 'x', baseUrl: 'http://a:1', webPath: '/app/', insecure: false, lastSeen: 0 } as Node
    const e = editableNode(old)
    expect(e.tags).toEqual([])
    expect(e.cap).toEqual({ limit: 0, day: 1, mode: 'total', hide: false })
    expect(e.access).toEqual({ groups: [], clients: [] })
    expect(e.alerts?.cpu).toBeNull()
    expect(e.token).toBe('')
  })

  it('does not share objects with the stored node', () => {
    const e = editableNode(stored)
    e.tags!.push('b')
    e.cap!.limit = 1
    expect(stored.tags).toEqual(['a'])
    expect(stored.cap!.limit).toBe(10)
  })

  it('sends only the settings', () => {
    const p = nodePayload(editableNode(stored))
    for (const k of ['inboundCount', 'clientCount', 'syncReport', 'tokenSet', 'dirty', 'lastSync']) expect(p).not.toHaveProperty(k)
    expect(p.cap).toEqual({ limit: 10, day: 3, mode: 'up', hide: true })
  })

  it('clones the settings into a new node with a free name, without the address or the token', () => {
    const c = cloneNode(stored, ['tokyo', 'tokyo-copy'])
    expect(c.id).toBe(0)
    expect(c.name).toBe('tokyo-copy-2')
    expect(c.baseUrl).toBe('')
    expect(c.token).toBe('')
    expect(c.tags).toEqual(['a'])
    expect(c.country).toBe('JP')
    expect(c.cap).toEqual(stored.cap)
    expect(c).not.toHaveProperty('syncReport')
    expect(uniqueName('n', [])).toBe('n')
  })
})

describe('the add-node wizard', () => {
  it('makes tokens of letters and digits, every one equally likely', () => {
    const t = randomToken(32)
    expect(t).toMatch(/^[A-Za-z0-9]{32}$/)
    expect(randomToken(32)).not.toBe(t)
    // 248 and above would favour the first characters: they are skipped.
    let calls = 0
    const fill = (buf: Uint8Array) => { buf.fill(calls++ === 0 ? 250 : 61); return buf }
    expect(randomToken(4, fill)).toBe('9999')
    expect(randomToken(3, buf => buf.fill(0))).toBe('AAA')
  })

  it('writes the install command only from safe values', () => {
    const token = 'A'.repeat(32)
    expect(installCommand({ token, port: 2095, path: '/xYz/' })).toBe(
      `bash <(curl -Ls https://raw.githubusercontent.com/Danialrostamani/drnetwork-panel/main/install.sh) --node-token ${token} --port 2095 --path /xYz/`)
    expect(installCommand({ token: 'short', port: 2095, path: '/a/' })).toBe('')
    expect(installCommand({ token, port: 70000, path: '/a/' })).toBe('')
    expect(installCommand({ token, port: 2095, path: '/a/;rm -rf /' })).toBe('')
    expect(installCommand({ token, port: 2095, path: '$(id)' })).toBe('')
    expect(validPath('/../etc/')).toBe(false)
  })

  it('builds the panel address of a new server', () => {
    expect(panelUrl(' 1.2.3.4 ', 2095)).toBe('http://1.2.3.4:2095')
    expect(panelUrl('2001:db8::1', 2095)).toBe('http://[2001:db8::1]:2095')
    expect(panelUrl('node.example.com', 443, true)).toBe('https://node.example.com:443')
    expect(validHost('node.example.com')).toBe(true)
    expect(validHost('1.2.3.4')).toBe(true)
    expect(validHost('2001:db8::1')).toBe(true)
    expect(validHost('bad host')).toBe(false)
    expect(validHost('a;b')).toBe(false)
  })
})

describe('adding several nodes', () => {
  it('reads one node per line, with the web path in the URL or after the token', () => {
    const { nodes, errors } = parseMultiAdd([
      '# name url token [path]',
      'de-1 https://1.2.3.4:2095/panel/ tokenAAAAAAAAAAAAAAA',
      '',
      'nl-1,http://5.6.7.8:2095,tokenBBBBBBBBBBBBBBB,secret',
      'fi-1\thttps://fi.example.com tokenC',
    ].join('\n'), [], true)
    expect(errors).toEqual([])
    expect(nodes.map(n => [n.name, n.baseUrl, n.webPath, n.token, n.insecure])).toEqual([
      ['de-1', 'https://1.2.3.4:2095', '/panel/', 'tokenAAAAAAAAAAAAAAA', true],
      ['nl-1', 'http://5.6.7.8:2095', '/secret/', 'tokenBBBBBBBBBBBBBBB', false],
      ['fi-1', 'https://fi.example.com', '/app/', 'tokenC', true],
    ])
    expect(nodes[0].cap).toEqual({ limit: 0, day: 1, mode: 'total', hide: false })
  })

  it('names the line and the reason of every mistake', () => {
    const { nodes, errors } = parseMultiAdd([
      'only-two http://a:1',
      'bad[name] http://a:1 t',
      'x ftp://a:1 t',
      'y 1.2.3.4:2095 t',
      'old http://a:1 t',
      'dup http://a:1 t',
      'dup http://b:1 t',
      'a b c d e',
    ].join('\n'), ['old'])
    expect(errors.map(e => [e.line, e.reason])).toEqual([
      [1, 'fields'], [2, 'name'], [3, 'url'], [4, 'url'], [5, 'duplicate'], [7, 'duplicate'], [8, 'fields'],
    ])
    expect(nodes.map(n => n.name)).toEqual(['dup'])
  })
})

describe('the history charts', () => {
  it('puts every bucket on the time line, missing ones as gaps', () => {
    const p = (t: number) => ({ t, uptime: 100, latency: 1, cpu: 1, mem: 1, disk: 1, online: 1, sent: 1, recv: 1 })
    const h: NodeHistory = { since: 600, bucket: 300, points: [p(600), p(1200)] }
    const s = historySeries(h, 1500)
    expect(s.times).toEqual([600, 900, 1200, 1500])
    expect(s.points.map(x => x?.t ?? null)).toEqual([600, null, 1200, null])
    expect(historySeries({ since: 0, bucket: 0, points: [p(5)] }, 10).times).toEqual([5])
  })
})

describe('versions', () => {
  it('writes panel versions with a v, old and new numbering alike', () => {
    expect(versionLabel('32')).toBe('v32')
    expect(versionLabel('v32')).toBe('v32')
    expect(versionLabel('1.6.3-drnetwork.31')).toBe('v1.6.3-drnetwork.31')
    expect(versionLabel('')).toBe('')
    expect(nodeVersion({ appFull: '32', appVersion: '32' })).toBe('v32')
    expect(nodeVersion({ appVersion: '1.6.3' })).toBe('v1.6.3')
    expect(nodeVersion(undefined)).toBe('')
  })
})
