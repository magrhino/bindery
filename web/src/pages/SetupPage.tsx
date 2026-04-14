import { FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { useAuth } from '../auth/AuthContext'
import { CardShell } from './LoginPage'

export default function SetupPage() {
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const { refresh } = useAuth()
  const navigate = useNavigate()

  // Read values from the form at submit time. See LoginPage for rationale —
  // controlled inputs break when browser autofill bypasses React onChange.
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const data = new FormData(e.currentTarget)
    const username = String(data.get('username') || '').trim()
    const password = String(data.get('password') || '')
    const confirm = String(data.get('confirm') || '')
    setError('')
    if (!username) {
      setError('Username is required')
      return
    }
    if (password !== confirm) {
      setError('Passwords do not match')
      return
    }
    if (password.length < 8) {
      setError('Password must be at least 8 characters')
      return
    }
    setSubmitting(true)
    try {
      await api.authSetup(username, password)
      await refresh()
      navigate('/', { replace: true })
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Setup failed'
      setError(msg)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <CardShell title="First-run setup" subtitle="Create the catalogue administrator">
      <p className="text-xs text-slate-500 dark:text-zinc-500 mb-4 leading-relaxed">
        Choose a username and password for the single administrator account.
        You can change either later in Settings.
      </p>
      <form onSubmit={submit} className="space-y-4">
        <label className="block">
          <span className="block text-xs font-medium text-slate-600 dark:text-zinc-400 mb-1">Username</span>
          <input
            type="text"
            name="username"
            id="username"
            autoComplete="username"
            defaultValue="admin"
            required
            className="w-full bg-white dark:bg-zinc-900 border border-slate-300 dark:border-zinc-700 rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </label>
        <label className="block">
          <span className="block text-xs font-medium text-slate-600 dark:text-zinc-400 mb-1">Password</span>
          <input
            type="password"
            name="password"
            id="password"
            autoComplete="new-password"
            required
            className="w-full bg-white dark:bg-zinc-900 border border-slate-300 dark:border-zinc-700 rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </label>
        <label className="block">
          <span className="block text-xs font-medium text-slate-600 dark:text-zinc-400 mb-1">Confirm password</span>
          <input
            type="password"
            name="confirm"
            id="confirm-password"
            autoComplete="new-password"
            required
            className="w-full bg-white dark:bg-zinc-900 border border-slate-300 dark:border-zinc-700 rounded-md px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </label>
        {error && (
          <div className="text-sm text-red-600 dark:text-red-400 py-1">{error}</div>
        )}
        <button
          type="submit"
          disabled={submitting}
          className="w-full bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed text-white font-medium rounded-md py-2 text-sm transition-colors"
        >
          {submitting ? 'Creating…' : 'Create account'}
        </button>
      </form>
    </CardShell>
  )
}
