import { describe, it, expect } from 'vitest'
import { defaultNode, nodesHealth, type Node, type NodeState, type NodeStatus } from './node'

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
