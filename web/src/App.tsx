import { BrowserRouter, Routes, Route, NavLink, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { api } from './api/client'
import AuthorsPage from './pages/AuthorsPage'
import BooksPage from './pages/BooksPage'
import WantedPage from './pages/WantedPage'
import QueuePage from './pages/QueuePage'
import SettingsPage from './pages/SettingsPage'
import HistoryPage from './pages/HistoryPage'
import SeriesPage from './pages/SeriesPage'
import CalendarPage from './pages/CalendarPage'
import BlocklistPage from './pages/BlocklistPage'

function App() {
  const [version, setVersion] = useState('')

  useEffect(() => {
    api.status().then(s => setVersion(s.version)).catch(() => {})
  }, [])

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `px-3 py-2 rounded-md text-sm font-medium transition-colors ${
      isActive ? 'bg-zinc-800 text-white' : 'text-zinc-400 hover:text-white hover:bg-zinc-800/50'
    }`

  return (
    <BrowserRouter>
      <div className="min-h-screen bg-zinc-950 text-zinc-100">
        <header className="border-b border-zinc-800">
          <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="flex items-center justify-between h-14">
              <div className="flex items-center gap-6 overflow-x-auto">
                <Link to="/" className="flex items-center gap-2 flex-shrink-0 group">
                  <img
                    src="/favicon.png"
                    alt="Bindery"
                    className="w-8 h-8 rounded-full transition-transform group-hover:scale-105"
                  />
                  <h1 className="text-lg font-bold tracking-tight">Bindery</h1>
                </Link>
                <nav className="flex gap-1 flex-shrink-0">
                  <NavLink to="/" end className={linkClass}>Authors</NavLink>
                  <NavLink to="/books" className={linkClass}>Books</NavLink>
                  <NavLink to="/wanted" className={linkClass}>Wanted</NavLink>
                  <NavLink to="/queue" className={linkClass}>Queue</NavLink>
                  <NavLink to="/history" className={linkClass}>History</NavLink>
                  <NavLink to="/series" className={linkClass}>Series</NavLink>
                  <NavLink to="/calendar" className={linkClass}>Calendar</NavLink>
                  <NavLink to="/blocklist" className={linkClass}>Blocklist</NavLink>
                  <NavLink to="/settings" className={linkClass}>Settings</NavLink>
                </nav>
              </div>
              {version && (
                <span className="text-xs text-zinc-600 flex-shrink-0 ml-4">v{version}</span>
              )}
            </div>
          </div>
        </header>

        <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
          <Routes>
            <Route path="/" element={<AuthorsPage />} />
            <Route path="/books" element={<BooksPage />} />
            <Route path="/wanted" element={<WantedPage />} />
            <Route path="/queue" element={<QueuePage />} />
            <Route path="/history" element={<HistoryPage />} />
            <Route path="/series" element={<SeriesPage />} />
            <Route path="/calendar" element={<CalendarPage />} />
            <Route path="/blocklist" element={<BlocklistPage />} />
            <Route path="/settings" element={<SettingsPage />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  )
}

export default App
