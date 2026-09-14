import { describe, expect, it } from 'vitest'
import i18n from './index'
import en from './locales/en.json'

const locales = i18n.options.resources!
const placeholders = (text: string) => [...text.matchAll(/{{\s*(\w+)\s*}}/g)].map(match => match[1]).sort()

describe('book rebind translations', () => {
  it.each(Object.keys(locales))('covers the modal in %s without English fallback', language => {
    const resource = locales[language].translation as Record<string, Record<string, string>>
    const messages = resource.bookRebind
    expect(Object.keys(messages).sort()).toEqual(Object.keys(en.bookRebind).sort())
    const t = i18n.getFixedT(language)

    for (const [key, english] of Object.entries(en.bookRebind)) {
      expect(messages[key].trim(), `${language}.${key}`).not.toBe('')
      expect(placeholders(messages[key]), `${language}.${key}`).toEqual(placeholders(english))
      const rendered = t(`bookRebind.${key}`, {
        title: 'Example title', current: 'Current author', upstream: 'Upstream author',
        count: 1, fallbackLng: false,
      })
      expect(rendered).not.toContain('{{')
      expect(rendered).not.toBe(`bookRebind.${key}`)
    }

    for (const count of [0, 1, 2]) {
      expect(t('bookRebind.resultCount', { count, fallbackLng: false })).toContain(String(count))
    }
    for (const key of ['cancel', 'links', 'viewOnSource']) expect(resource.common[key]).toBeTruthy()
    expect(placeholders(resource.common.viewOnSource)).toEqual(['source'])
    expect(resource.bookDetail.rebind).toBeTruthy()
    if (language !== 'en') expect(resource.bookDetail.rebind).not.toBe(en.bookDetail.rebind)
  })
})
