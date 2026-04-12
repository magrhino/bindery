import { useEffect, useState } from 'react'
import { api, Book } from '../api/client'

function getDaysInMonth(year: number, month: number) {
  return new Date(year, month + 1, 0).getDate()
}

function getFirstDayOfMonth(year: number, month: number) {
  return new Date(year, month, 1).getDay()
}

const MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

export default function CalendarPage() {
  const [books, setBooks] = useState<Book[]>([])
  const [loading, setLoading] = useState(true)
  const today = new Date()
  const [viewYear, setViewYear] = useState(today.getFullYear())
  const [viewMonth, setViewMonth] = useState(today.getMonth())

  useEffect(() => {
    api.listBooks().then(setBooks).catch(console.error).finally(() => setLoading(false))
  }, [])

  const prevMonth = () => {
    if (viewMonth === 0) { setViewMonth(11); setViewYear(y => y - 1) }
    else setViewMonth(m => m - 1)
  }

  const nextMonth = () => {
    if (viewMonth === 11) { setViewMonth(0); setViewYear(y => y + 1) }
    else setViewMonth(m => m + 1)
  }

  const goToToday = () => {
    setViewYear(today.getFullYear())
    setViewMonth(today.getMonth())
  }

  // Index books by day-of-month for the current view
  const booksByDay: Record<number, Book[]> = {}
  for (const book of books) {
    if (!book.releaseDate || !book.monitored) continue
    const d = new Date(book.releaseDate)
    if (d.getFullYear() === viewYear && d.getMonth() === viewMonth) {
      const day = d.getDate()
      if (!booksByDay[day]) booksByDay[day] = []
      booksByDay[day].push(book)
    }
  }

  const daysInMonth = getDaysInMonth(viewYear, viewMonth)
  const firstDay = getFirstDayOfMonth(viewYear, viewMonth)
  const isCurrentMonth = viewYear === today.getFullYear() && viewMonth === today.getMonth()

  // Build calendar grid cells (some are empty padding)
  const cells: Array<number | null> = []
  for (let i = 0; i < firstDay; i++) cells.push(null)
  for (let i = 1; i <= daysInMonth; i++) cells.push(i)
  // Pad to full weeks
  while (cells.length % 7 !== 0) cells.push(null)

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">Calendar</h2>
        <div className="flex items-center gap-2">
          {!isCurrentMonth && (
            <button
              onClick={goToToday}
              className="px-3 py-1.5 text-xs text-zinc-400 hover:text-white border border-zinc-700 rounded transition-colors"
            >
              Today
            </button>
          )}
          <button
            onClick={prevMonth}
            className="px-3 py-1.5 text-sm text-zinc-400 hover:text-white bg-zinc-800 hover:bg-zinc-700 rounded transition-colors"
          >
            ‹
          </button>
          <span className="text-sm font-medium w-36 text-center">
            {MONTH_NAMES[viewMonth]} {viewYear}
          </span>
          <button
            onClick={nextMonth}
            className="px-3 py-1.5 text-sm text-zinc-400 hover:text-white bg-zinc-800 hover:bg-zinc-700 rounded transition-colors"
          >
            ›
          </button>
        </div>
      </div>

      {loading ? (
        <div className="text-zinc-500">Loading...</div>
      ) : (
        <>
          <div className="border border-zinc-800 rounded-lg overflow-hidden">
            {/* Day headers */}
            <div className="grid grid-cols-7 bg-zinc-900 border-b border-zinc-800">
              {DAY_NAMES.map(d => (
                <div key={d} className="py-2 text-center text-xs font-medium text-zinc-500 uppercase tracking-wider">
                  {d}
                </div>
              ))}
            </div>

            {/* Calendar grid */}
            <div className="grid grid-cols-7">
              {cells.map((day, idx) => {
                const isToday = isCurrentMonth && day === today.getDate()
                const dayBooks = day ? (booksByDay[day] ?? []) : []
                return (
                  <div
                    key={idx}
                    className={`min-h-[100px] p-2 border-b border-r border-zinc-800 last:border-r-0 ${
                      day ? 'bg-zinc-900/50' : 'bg-zinc-900/20'
                    } ${idx % 7 === 6 ? 'border-r-0' : ''}`}
                  >
                    {day && (
                      <>
                        <div className={`text-xs font-medium mb-1 w-6 h-6 flex items-center justify-center rounded-full ${
                          isToday
                            ? 'bg-emerald-600 text-white'
                            : 'text-zinc-400'
                        }`}>
                          {day}
                        </div>
                        <div className="space-y-1">
                          {dayBooks.map(book => (
                            <div
                              key={book.id}
                              title={book.title}
                              className="text-[10px] leading-tight px-1.5 py-1 bg-emerald-500/20 text-emerald-300 rounded truncate cursor-default"
                            >
                              {book.title}
                            </div>
                          ))}
                        </div>
                      </>
                    )}
                  </div>
                )
              })}
            </div>
          </div>

          {/* Legend / summary */}
          {Object.keys(booksByDay).length > 0 && (
            <div className="mt-4 p-3 bg-zinc-900 border border-zinc-800 rounded-lg">
              <p className="text-xs text-zinc-400 font-medium mb-2">
                Books releasing in {MONTH_NAMES[viewMonth]} {viewYear}:
              </p>
              <div className="space-y-1">
                {Object.entries(booksByDay)
                  .sort(([a], [b]) => Number(a) - Number(b))
                  .map(([day, dayBooks]) => (
                    <div key={day} className="flex items-start gap-2 text-xs">
                      <span className="text-zinc-500 w-14 flex-shrink-0">
                        {MONTH_NAMES[viewMonth].slice(0, 3)} {day}
                      </span>
                      <span className="text-zinc-300">
                        {dayBooks.map(b => b.title).join(', ')}
                      </span>
                    </div>
                  ))}
              </div>
            </div>
          )}

          {Object.keys(booksByDay).length === 0 && (
            <p className="mt-4 text-center text-sm text-zinc-600">
              No monitored books releasing this month
            </p>
          )}
        </>
      )}
    </div>
  )
}
