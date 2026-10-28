import { createI18n } from 'vue-i18n'
import en from './en'
import fa from './fa'

// The panel speaks English and Persian; a language saved by an older
// version that had more falls back to English.
const saved = localStorage.getItem("locale")
export const panelLocale = saved === 'fa' || saved === 'en' ? saved : 'en'

export const i18n = createI18n({
  legacy: false,
  locale: panelLocale,
  fallbackLocale: 'en',
  messages: {
    en: en,
    fa: fa,
  },
})

export const locale = i18n.global.locale.value

export const languages = [
  { title: 'English', value: 'en' },
  { title: 'فارسی', value: 'fa' },
]
