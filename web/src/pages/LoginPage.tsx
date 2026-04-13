import { FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [rememberMe, setRememberMe] = useState(true)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const { refresh } = useAuth()
  const navigate = useNavigate()

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      await api.authLogin(username, password, rememberMe)
      await refresh()
      navigate('/', { replace: true })
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Login failed'
      setError(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <CardShell title="Sign in" subtitle="Catalogue restricted — staff only">
      <form onSubmit={submit} className="space-y-4">
        <Field label="Username">
          <input
            type="text"
            autoComplete="username"
            autoFocus
            value={username}
            onChange={e => setUsername(e.target.value)}
            className="w-full bg-white dark:bg-zinc-900 border border-slate-300 dark:border-zinc-700 rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </Field>
        <Field label="Password">
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            className="w-full bg-white dark:bg-zinc-900 border border-slate-300 dark:border-zinc-700 rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </Field>
        <label className="flex items-center gap-2 text-sm text-slate-600 dark:text-zinc-400">
          <input
            type="checkbox"
            checked={rememberMe}
            onChange={e => setRememberMe(e.target.checked)}
            className="rounded"
          />
          Remember me on this device for 30 days
        </label>
        {error && (
          <div className="text-sm text-red-600 dark:text-red-400 py-1">{error}</div>
        )}
        <button
          type="submit"
          disabled={submitting || !username || !password}
          className="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white font-medium rounded-md py-2 text-sm transition-colors"
        >
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </CardShell>
  )
}

export function CardShell({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  return (
    <div className="min-h-screen flex items-center justify-center px-4 bg-slate-50 dark:bg-zinc-950 text-slate-900 dark:text-zinc-100">
      <div className="w-full max-w-sm">
        <div className="mb-6 text-center">
          <div className="flex items-center justify-center gap-2">
            <img src="/favicon.png" alt="Bindery" className="w-12 h-12 rounded-full" />
            <h1 className="text-2xl font-bold tracking-tight">Bindery</h1>
          </div>
          <div className="text-xs text-slate-500 dark:text-zinc-500 mt-2">{subtitle}</div>
        </div>
        <div className="border border-slate-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 rounded-lg p-6 shadow-sm">
          <h2 className="text-lg font-semibold mb-4">{title}</h2>
          {children}
        </div>
      </div>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="block text-xs font-medium text-slate-600 dark:text-zinc-400 mb-1">{label}</span>
      {children}
    </label>
  )
}
