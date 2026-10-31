import { describe, expect, it } from 'vitest'
import { telegramId } from './telegramId'
describe('Telegram IDs', () => {
 it.each(['0', '-42', '+42', '42.1', '1e5', 'abc', '9007199254740992', ''])('rejects %s', id => expect(telegramId(id)).toBe(''))
 it.each(['123456789', ' 123456789 ', '۱۲۳۴۵۶۷۸۹', '١٢٣٤٥٦٧٨٩'])('normalizes %s', id => expect(telegramId(id)).toBe('123456789'))
})
