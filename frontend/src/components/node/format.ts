import { HumanReadable } from '@/plugins/utils'
import { locale } from '@/locales'

// Formatting shared by the node views. Zero reads as zero here: the panel's
// size formatter shows a dash for it, which on a traffic counter looks like
// "unknown".
export const fmtBytes = (n?: number | null): string => (n && n > 0 ? HumanReadable.sizeFormat(n) : '0')
export const fmtSpeed = (n?: number | null): string => (n && n > 0 ? HumanReadable.sizeFormat(n) + '/s' : '0')
export const fmtPercent = (v?: number | null, digits = 0): string => (v == null ? '-' : v.toFixed(digits) + '%')
export const fmtUptime = (v?: number | null): string => (v == null || v < 0 ? '-' : v >= 99.95 ? '100%' : v.toFixed(1) + '%')
export const fmtTime = (ts?: number | null): string => (ts ? new Date(ts * 1000).toLocaleString(locale) : '-')
export const fmtDate = (ts?: number | null): string => (ts ? new Date(ts * 1000).toLocaleDateString(locale) : '-')
export const fmtDuration = (sec?: number | null): string => (sec && sec > 0 ? HumanReadable.formatSecond(sec) : '0')

// The colour of a bar that fills up: CPU, memory, disk, a cap.
export const loadColor = (v?: number | null): string => (v == null ? 'grey' : v >= 90 ? 'error' : v >= 75 ? 'warning' : 'success')
