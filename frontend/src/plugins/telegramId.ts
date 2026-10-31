/** Telegram user IDs fit safely in a JavaScript integer. Accept localized digits. */
export const telegramId = (raw: string): string => {
  const value = raw.trim().replace(/[۰-۹]/g, c => String(c.charCodeAt(0) - 0x6f0)).replace(/[٠-٩]/g, c => String(c.charCodeAt(0) - 0x660))
  if (!/^[1-9]\d*$/.test(value) || !Number.isSafeInteger(Number(value))) return ''
  return value
}
