import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import SettingsPage from './SettingsPage'
import { api } from '../api/client'

vi.mock('../settings/AuthSettings', () => ({ default: () => <div data-testid="auth-settings" /> }))
vi.mock('../components/ThemeToggle', () => ({ default: () => <button type="button">Theme</button> }))
vi.mock('../components/LanguageSwitcher', () => ({ default: () => <select aria-label="Language" /> }))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ isAdmin: true }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
    i18n: { changeLanguage: vi.fn() },
  }),
}))
vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listIndexers: vi.fn(),
      listDownloadClients: vi.fn(),
      listProwlarr: vi.fn(),
      absConfig: vi.fn(),
      listSettings: vi.fn(),
      listBackups: vi.fn(),
      libraryScanStatus: vi.fn(),
      getStorage: vi.fn(),
      listRootFolders: vi.fn(),
      status: vi.fn(),
      setSetting: vi.fn(),
      testHardcover: vi.fn(),
      authConfig: vi.fn(),
    },
  }
})

describe('SettingsPage Hardcover API keys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listIndexers).mockResolvedValue([])
    vi.mocked(api.listDownloadClients).mockResolvedValue([])
    vi.mocked(api.listProwlarr).mockResolvedValue([])
    vi.mocked(api.absConfig).mockResolvedValue({ featureEnabled: false, baseUrl: '', label: '', enabled: false, libraryId: '', pathRemap: '', apiKeyConfigured: false })
    vi.mocked(api.listSettings).mockResolvedValue([{ key: 'hardcover.enhanced_series_enabled', value: 'false' }])
    vi.mocked(api.listBackups).mockResolvedValue([])
    vi.mocked(api.libraryScanStatus).mockRejectedValue(new Error('no scan'))
    vi.mocked(api.getStorage).mockResolvedValue({ downloadDir: '/downloads', libraryDir: '/books', audiobookDir: '' })
    vi.mocked(api.listRootFolders).mockResolvedValue([])
    vi.mocked(api.status).mockResolvedValue({
      version: 'dev',
      commit: 'unknown',
      buildDate: '',
      enhancedHardcoverApi: false,
      hardcoverTokenConfigured: false,
      enhancedHardcoverDisabledReason: 'env_disabled',
    })
    vi.mocked(api.setSetting).mockResolvedValue(undefined)
    vi.mocked(api.testHardcover).mockResolvedValue({
      ok: true,
      tokenConfigured: true,
      searchResults: 2,
      sampleSeriesId: 'hc-series:1150',
      sampleTitle: 'Dune',
      catalogOk: true,
      catalogBookCount: 8,
      message: 'Found 2 series; catalog "Dune" has 8 books',
    })
    vi.mocked(api.authConfig).mockResolvedValue({ mode: 'disabled', apiKey: 'key', username: 'admin' })
  })

  it('adds a write-only Hardcover token field with API link', async () => {
    render(<SettingsPage />)

    expect(await screen.findByText('Hardcover API Token')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: 'Create or copy a Hardcover API token' })
    expect(link).toHaveAttribute('href', 'https://hardcover.app/account/api')

    fireEvent.change(screen.getByPlaceholderText('Paste a Hardcover API token'), { target: { value: 'hc-secret' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save Hardcover API token' }))

    await waitFor(() => {
      expect(api.setSetting).toHaveBeenCalledWith('hardcover.api_token', 'hc-secret')
    })
  })

  it('persists the enhanced Hardcover admin toggle separately from effective status', async () => {
    render(<SettingsPage />)

    fireEvent.click(await screen.findByRole('button', { name: 'Toggle enhanced Hardcover series' }))

    await waitFor(() => {
      expect(api.setSetting).toHaveBeenCalledWith('hardcover.enhanced_series_enabled', 'true')
    })
  })

  it('tests the configured Hardcover token without exposing it', async () => {
    vi.mocked(api.status).mockResolvedValue({
      version: 'dev',
      commit: 'unknown',
      buildDate: '',
      enhancedHardcoverApi: true,
      hardcoverTokenConfigured: true,
    })

    render(<SettingsPage />)

    fireEvent.click(await screen.findByRole('button', { name: 'Test Hardcover API' }))

    await waitFor(() => {
      expect(api.testHardcover).toHaveBeenCalled()
    })
    expect(await screen.findByText('Found 2 series; catalog "Dune" has 8 books')).toBeInTheDocument()
    expect(screen.queryByText('hc-secret')).not.toBeInTheDocument()
  })
})
