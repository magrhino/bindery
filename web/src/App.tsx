import { useEffect, useState } from 'react'

interface SystemStatus {
  version: string
  commit: string
  buildDate: string
}

function App() {
  const [status, setStatus] = useState<SystemStatus | null>(null)

  useEffect(() => {
    fetch('/api/v1/system/status')
      .then(res => res.json())
      .then(setStatus)
      .catch(console.error)
  }, [])

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-100">
      <header className="border-b border-zinc-800 px-6 py-4">
        <div className="flex items-center justify-between max-w-7xl mx-auto">
          <h1 className="text-2xl font-bold tracking-tight">Bindery</h1>
          <nav className="flex gap-6 text-sm text-zinc-400">
            <a href="#" className="hover:text-zinc-100 transition-colors">Authors</a>
            <a href="#" className="hover:text-zinc-100 transition-colors">Books</a>
            <a href="#" className="hover:text-zinc-100 transition-colors">Wanted</a>
            <a href="#" className="hover:text-zinc-100 transition-colors">Activity</a>
            <a href="#" className="hover:text-zinc-100 transition-colors">Settings</a>
          </nav>
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-6 py-12">
        <div className="text-center">
          <h2 className="text-4xl font-bold tracking-tight mb-4">Welcome to Bindery</h2>
          <p className="text-zinc-400 text-lg mb-8">
            Automated book download manager for Usenet
          </p>

          {status ? (
            <div className="inline-flex items-center gap-2 rounded-full bg-emerald-500/10 border border-emerald-500/20 px-4 py-2 text-sm text-emerald-400">
              <span className="w-2 h-2 rounded-full bg-emerald-500" />
              Connected &mdash; v{status.version}
            </div>
          ) : (
            <div className="inline-flex items-center gap-2 rounded-full bg-zinc-800 border border-zinc-700 px-4 py-2 text-sm text-zinc-400">
              <span className="w-2 h-2 rounded-full bg-zinc-500 animate-pulse" />
              Connecting...
            </div>
          )}

          <div className="mt-16 grid grid-cols-1 md:grid-cols-3 gap-6 text-left">
            <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-6">
              <h3 className="font-semibold mb-2">1. Add Indexers</h3>
              <p className="text-sm text-zinc-400">Configure your Newznab indexers in Settings to start searching for books.</p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-6">
              <h3 className="font-semibold mb-2">2. Connect SABnzbd</h3>
              <p className="text-sm text-zinc-400">Add your SABnzbd download client to enable automated downloads.</p>
            </div>
            <div className="rounded-lg border border-zinc-800 bg-zinc-900 p-6">
              <h3 className="font-semibold mb-2">3. Add Authors</h3>
              <p className="text-sm text-zinc-400">Search for your favorite authors and Bindery will track all their books.</p>
            </div>
          </div>
        </div>
      </main>

      <footer className="border-t border-zinc-800 px-6 py-4 text-center text-xs text-zinc-600">
        Bindery &mdash; Open source book automation
      </footer>
    </div>
  )
}

export default App
