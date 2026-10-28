import { describe, expect, it } from 'vitest'
import en from './en'
import fa from './fa'

type Tree = { [key: string]: string | Tree }

function flat(tree: Tree, prefix = '', out: Record<string, string> = {}): Record<string, string> {
  for (const [k, v] of Object.entries(tree)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out[key] = v
    else flat(v, key, out)
  }
  return out
}

const english = flat(en as Tree)
const placeholders = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map(m => m[1]).sort()

describe('locales', () => {
  const others: Record<string, Tree> = { fa }
  for (const [name, tree] of Object.entries({ en, ...others } as Record<string, Tree>)) {
    it(`${name} has no bare @, which vue-i18n reads as a link and fails on`, () => {
      for (const [key, value] of Object.entries(flat(tree))) {
        expect(value.replaceAll("{'@'}", ''), `${name}: ${key}`).not.toContain('@')
      }
    })
  }
  for (const [name, tree] of Object.entries(others)) {
    const messages = flat(tree)
    it(`${name} translates every node message with the same placeholders`, () => {
      for (const [key, value] of Object.entries(english)) {
        if (!key.startsWith('node.')) continue
        expect(messages[key], `${name}: ${key}`).toBeTypeOf('string')
        expect(placeholders(messages[key]), `${name}: ${key}`).toEqual(placeholders(value))
      }
    })
    it(`${name} node messages avoid characters vue-i18n treats as syntax`, () => {
      for (const [key, value] of Object.entries(messages)) {
        if (key.startsWith('node.')) expect(value, `${name}: ${key}`).not.toMatch(/[@|$]/)
      }
    })
  }
})
