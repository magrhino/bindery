import { describe, expect, it } from 'vitest'
import type { Author, Book } from '../api/client'
import { splitAuthorSearchResults } from './addAuthorTitleGuard'

function makeAuthor(overrides: Partial<Author>): Author {
  return {
    id: 1,
    foreignAuthorId: 'OL_AUTHOR_A',
    authorName: 'Author',
    sortName: 'Author',
    description: '',
    imageUrl: '',
    disambiguation: '',
    ratingsCount: 0,
    averageRating: 0,
    monitored: true,
    ...overrides,
  }
}

function makeBook(overrides: Partial<Book>): Book {
  return {
    id: 1,
    foreignBookId: 'OL_BOOK_W',
    authorId: 1,
    title: 'Book',
    description: '',
    imageUrl: '',
    genres: [],
    monitored: true,
    status: 'wanted',
    filePath: '',
    mediaType: 'ebook',
    ebookFilePath: '',
    audiobookFilePath: '',
    excluded: false,
    ...overrides,
  }
}

describe('splitAuthorSearchResults', () => {
  it('hides an exact book-title author inversion', () => {
    const candidate = makeAuthor({
      foreignAuthorId: 'OL_BAD_TITLE_A',
      authorName: 'Romeo and Juliet',
      disambiguation: 'William Shakespeare',
    })
    const book = makeBook({
      title: 'Romeo and Juliet',
      author: makeAuthor({
        foreignAuthorId: 'OL_SHAKESPEARE_A',
        authorName: 'William Shakespeare',
      }),
    })

    const split = splitAuthorSearchResults([candidate], [book], 'Romeo and Juliet')

    expect(split.visible).toEqual([])
    expect(split.hidden).toEqual([candidate])
  })

  it('preserves legitimate author results with exact-name searches', () => {
    const author = makeAuthor({
      foreignAuthorId: 'OL_SHAKESPEARE_A',
      authorName: 'William Shakespeare',
      disambiguation: 'Romeo and Juliet',
    })
    const book = makeBook({
      title: 'Romeo and Juliet',
      author,
    })

    const split = splitAuthorSearchResults([author], [book], 'William Shakespeare')

    expect(split.visible).toEqual([author])
    expect(split.hidden).toEqual([])
  })

  it('preserves results when book metadata lacks author data', () => {
    const candidate = makeAuthor({
      foreignAuthorId: 'OL_BAD_TITLE_A',
      authorName: 'Romeo and Juliet',
      disambiguation: 'William Shakespeare',
    })
    const book = makeBook({ title: 'Romeo and Juliet' })

    const split = splitAuthorSearchResults([candidate], [book], 'Romeo and Juliet')

    expect(split.visible).toEqual([candidate])
    expect(split.hidden).toEqual([])
  })

  it('preserves results when candidate and book author IDs match', () => {
    const candidate = makeAuthor({
      foreignAuthorId: 'OL_SHAKESPEARE_A',
      authorName: 'Romeo and Juliet',
      disambiguation: 'William Shakespeare',
    })
    const book = makeBook({
      title: 'Romeo and Juliet',
      author: makeAuthor({
        foreignAuthorId: 'OL_SHAKESPEARE_A',
        authorName: 'William Shakespeare',
      }),
    })

    const split = splitAuthorSearchResults([candidate], [book], 'Romeo and Juliet')

    expect(split.visible).toEqual([candidate])
    expect(split.hidden).toEqual([])
  })

  it('does not hide partial or merely similar title matches', () => {
    const candidate = makeAuthor({
      foreignAuthorId: 'OL_ROMEO_A',
      authorName: 'Romeo',
      disambiguation: 'William Shakespeare',
    })
    const book = makeBook({
      title: 'Romeo and Juliet',
      author: makeAuthor({
        foreignAuthorId: 'OL_SHAKESPEARE_A',
        authorName: 'William Shakespeare',
      }),
    })

    const split = splitAuthorSearchResults([candidate], [book], 'Romeo')

    expect(split.visible).toEqual([candidate])
    expect(split.hidden).toEqual([])
  })
})
