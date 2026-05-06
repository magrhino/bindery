import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import SettingsPage from './SettingsPage'
import { api, type DownloadClient, type Indexer, type ProwlarrInstance, type SystemStatus } from '../api/client'

vi.mock('../settings/AuthSettings', () => ({ default: () => <div data-testid="auth-settings" /> }))
vi.mock('../components/ThemeToggle', () => ({ default: () => <button type="button">Theme</button> }))
vi.mock('../components/LanguageSwitcher', () => ({ default: () => <select aria-label="Language" /> }))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ isAdmin: true }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => typeof fallback === 'string' ? fallback : key,
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
      addIndexer: vi.fn(),
      updateIndexer: vi.fn(),
      deleteIndexer: vi.fn(),
      testIndexer: vi.fn(),
      listDownloadClients: vi.fn(),
      addDownloadClient: vi.fn(),
      updateDownloadClient: vi.fn(),
      deleteDownloadClient: vi.fn(),
      testDownloadClient: vi.fn(),
      listProwlarr: vi.fn(),
      addProwlarr: vi.fn(),
      syncProwlarr: vi.fn(),
      testProwlarr: vi.fn(),
      deleteProwlarr: vi.fn(),
      absConfig: vi.fn(),
      absSetConfig: vi.fn(),
      absLibraries: vi.fn(),
      absImportStart: vi.fn(),
      absImportStatus: vi.fn(),
      absImportRuns: vi.fn(),
      absReviewItems: vi.fn(),
      absConflicts: vi.fn(),
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

const defaultStatus: SystemStatus = {
  version: 'dev',
  commit: 'unknown',
  buildDate: '',
  enhancedHardcoverApi: false,
  hardcoverTokenConfigured: false,
  enhancedHardcoverDisabledReason: 'env_disabled',
}

function makeIndexer(overrides: Partial<Indexer> = {}): Indexer {
  return {
    id: 1,
    name: 'NZBGeek',
    type: 'newznab',
    url: 'https://nzbgeek.example.com',
    apiKey: 'indexer-key',
    categories: [7020],
    enabled: true,
    ...overrides,
  }
}

function makeProwlarr(overrides: Partial<ProwlarrInstance> = {}): ProwlarrInstance {
  return {
    id: 10,
    name: 'Prowlarr',
    url: 'http://prowlarr:9696',
    apiKey: 'prowlarr-key',
    syncOnStartup: true,
    enabled: true,
    ...overrides,
  }
}

function makeClient(overrides: Partial<DownloadClient> = {}): DownloadClient {
  return {
    id: 20,
    name: 'SABnzbd',
    type: 'sabnzbd',
    host: 'sabnzbd',
    port: 8080,
    apiKey: 'sab-key',
    username: '',
    password: '',
    useSsl: false,
    urlBase: '',
    category: 'books',
    enabled: true,
    ...overrides,
  }
}

function seedSettingsMocks(options: {
  indexers?: Indexer[]
  clients?: DownloadClient[]
  prowlarr?: ProwlarrInstance[]
  status?: SystemStatus
} = {}) {
  vi.mocked(api.listIndexers).mockResolvedValue(options.indexers ?? [])
  vi.mocked(api.addIndexer).mockImplementation(async data => makeIndexer({ id: 100, ...data }))
  vi.mocked(api.updateIndexer).mockImplementation(async (id, data) => makeIndexer({ ...data, id }))
  vi.mocked(api.deleteIndexer).mockResolvedValue(undefined)
  vi.mocked(api.testIndexer).mockResolvedValue({ ok: true, status: 200, categories: 12, bookSearch: true, latencyMs: 34, searchResults: 2 })

  vi.mocked(api.listDownloadClients).mockResolvedValue(options.clients ?? [])
  vi.mocked(api.addDownloadClient).mockImplementation(async data => makeClient({ id: 200, ...data }))
  vi.mocked(api.updateDownloadClient).mockImplementation(async (id, data) => makeClient({ ...data, id }))
  vi.mocked(api.deleteDownloadClient).mockResolvedValue(undefined)
  vi.mocked(api.testDownloadClient).mockResolvedValue({ message: 'ok' })

  vi.mocked(api.listProwlarr).mockResolvedValue(options.prowlarr ?? [])
  vi.mocked(api.addProwlarr).mockImplementation(async data => makeProwlarr({ id: 300, ...data }))
  vi.mocked(api.syncProwlarr).mockResolvedValue({ added: 0, updated: 0, removed: 0 })
  vi.mocked(api.testProwlarr).mockResolvedValue({ ok: 'true', version: '1.0.0' })
  vi.mocked(api.deleteProwlarr).mockResolvedValue(undefined)

    vi.mocked(api.absConfig).mockResolvedValue({ featureEnabled: false, baseUrl: '', label: '', enabled: false, libraryId: '', pathRemap: '', apiKeyConfigured: false })
    vi.mocked(api.absSetConfig).mockResolvedValue({ featureEnabled: false, baseUrl: '', label: '', enabled: false, libraryId: '', pathRemap: '', apiKeyConfigured: false })
    vi.mocked(api.absLibraries).mockResolvedValue([])
    vi.mocked(api.absImportStart).mockResolvedValue({ running: true, dryRun: true, processed: 0 })
    vi.mocked(api.absImportStatus).mockResolvedValue({ running: false, processed: 0 })
    vi.mocked(api.absImportRuns).mockResolvedValue([])
    vi.mocked(api.absReviewItems).mockResolvedValue({ items: [], total: 0, limit: 50, offset: 0 })
    vi.mocked(api.absConflicts).mockResolvedValue({ items: [], total: 0, limit: 50, offset: 0 })
    vi.mocked(api.listSettings).mockResolvedValue([{ key: 'hardcover.enhanced_series_enabled', value: 'false' }])
    vi.mocked(api.listBackups).mockResolvedValue([])
    vi.mocked(api.libraryScanStatus).mockRejectedValue(new Error('no scan'))
    vi.mocked(api.getStorage).mockResolvedValue({ downloadDir: '/downloads', libraryDir: '/books', audiobookDir: '' })
    vi.mocked(api.listRootFolders).mockResolvedValue([])
    vi.mocked(api.status).mockResolvedValue(options.status ?? defaultStatus)
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
}

function renderSettings(options?: Parameters<typeof seedSettingsMocks>[0]) {
  if (options) seedSettingsMocks(options)
  return render(<SettingsPage />)
}

async function openIndexersTab() {
  fireEvent.click(await screen.findByRole('button', { name: 'settings.tabs.indexers' }))
}

async function openClientsTab() {
  fireEvent.click(await screen.findByRole('button', { name: 'settings.tabs.clients' }))
}

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    seedSettingsMocks()
  })

  it('adds a write-only Hardcover token field with API link', async () => {
    renderSettings()

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
    renderSettings()

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

    renderSettings()

    fireEvent.click(await screen.findByRole('button', { name: 'Test Hardcover API' }))

    await waitFor(() => {
      expect(api.testHardcover).toHaveBeenCalled()
    })
    expect(await screen.findByText('Found 2 series; catalog "Dune" has 8 books')).toBeInTheDocument()
    expect(screen.queryByText('hc-secret')).not.toBeInTheDocument()
  })

  it('requires saved Audiobookshelf settings before starting an import', async () => {
    vi.mocked(api.absConfig).mockResolvedValue({
      featureEnabled: true,
      baseUrl: 'https://abs.example.com',
      label: 'Shelf',
      enabled: true,
      libraryId: 'lib-books',
      pathRemap: '/abs:/books',
      apiKeyConfigured: true,
    })
    vi.mocked(api.absSetConfig).mockImplementation(async data => ({
      featureEnabled: true,
      baseUrl: data.baseUrl,
      label: data.label,
      enabled: data.enabled,
      libraryId: data.libraryId,
      pathRemap: data.pathRemap,
      apiKeyConfigured: true,
    }))

    renderSettings()

    fireEvent.click(await screen.findByRole('button', { name: 'settings.tabs.abs' }))
    const preview = await screen.findByRole('button', { name: 'Preview changes' })
    expect(preview).toBeEnabled()

    fireEvent.change(screen.getByPlaceholderText('/audiobookshelf:/books/audiobookshelf,/abs:/books'), { target: { value: '/draft:/books' } })
    expect(preview).toBeDisabled()
    expect(screen.getByText('Save Audiobookshelf settings before starting an import so the run uses the stored source configuration.')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Save source' }))
    await waitFor(() => {
      expect(api.absSetConfig).toHaveBeenCalledWith({
        baseUrl: 'https://abs.example.com',
        apiKey: undefined,
        label: 'Shelf',
        enabled: true,
        libraryId: 'lib-books',
        pathRemap: '/draft:/books',
      })
    })
    await waitFor(() => expect(preview).toBeEnabled())

    fireEvent.click(preview)
    await waitFor(() => {
      expect(api.absImportStart).toHaveBeenCalledWith({ dryRun: true })
    })
  })

  it('adds an indexer with parsed categories', async () => {
    renderSettings()
    await openIndexersTab()

    fireEvent.click(screen.getByRole('button', { name: 'settings.indexers.addButton' }))
    fireEvent.change(screen.getByPlaceholderText('Name (e.g. NZBGeek)'), { target: { value: 'SceneNZBs' } })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'torznab' } })
    fireEvent.change(screen.getByPlaceholderText('URL (e.g. https://api.nzbgeek.info or http://prowlarr:9696/1/api)'), { target: { value: 'http://prowlarr:9696/1/api' } })
    fireEvent.change(screen.getByPlaceholderText('API Key'), { target: { value: 'scene-key' } })
    fireEvent.change(screen.getByDisplayValue('7020'), { target: { value: '7020, 7120, bad, 3030' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(api.addIndexer).toHaveBeenCalledWith({
        name: 'SceneNZBs',
        url: 'http://prowlarr:9696/1/api',
        apiKey: 'scene-key',
        type: 'torznab',
        categories: [7020, 7120, 3030],
        enabled: true,
      })
    })
    expect(await screen.findByText('SceneNZBs')).toBeInTheDocument()
  })

  it('edits an indexer while preserving existing fields', async () => {
    const indexer = makeIndexer({ id: 7, name: 'Old Indexer', categories: [7020, 3030], enabled: true })

    renderSettings({ indexers: [indexer] })
    await openIndexersTab()

    fireEvent.click(screen.getByRole('button', { name: 'common.edit' }))
    fireEvent.change(screen.getByPlaceholderText('Name'), { target: { value: 'DrunkenSlug' } })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'torznab' } })
    fireEvent.change(screen.getByPlaceholderText('URL'), { target: { value: 'https://slug.example.com/api' } })
    fireEvent.change(screen.getByPlaceholderText('API Key'), { target: { value: 'slug-key' } })
    fireEvent.change(screen.getByDisplayValue('7020, 3030'), { target: { value: '7020, bad, 3030' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(api.updateIndexer).toHaveBeenCalledWith(7, {
        ...indexer,
        name: 'DrunkenSlug',
        type: 'torznab',
        url: 'https://slug.example.com/api',
        apiKey: 'slug-key',
        categories: [7020, 3030],
      })
    })
    expect(await screen.findByText('DrunkenSlug')).toBeInTheDocument()
  })

  it('toggles and deletes an indexer', async () => {
    const indexer = makeIndexer({ id: 8, name: 'Toggle Indexer', enabled: true })
    vi.mocked(api.updateIndexer).mockResolvedValue({ ...indexer, enabled: false })

    renderSettings({ indexers: [indexer] })
    await openIndexersTab()

    fireEvent.click(screen.getByTitle('common.disable'))
    await waitFor(() => {
      expect(api.updateIndexer).toHaveBeenCalledWith(8, { ...indexer, enabled: false })
    })
    expect(await screen.findByTitle('common.enable')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'common.delete' }))
    await waitFor(() => expect(api.deleteIndexer).toHaveBeenCalledWith(8))
    await waitFor(() => expect(screen.queryByText('Toggle Indexer')).not.toBeInTheDocument())
  })

  it('renders indexer test success, warning, and failure states', async () => {
    const indexer = makeIndexer({ id: 9, name: 'Probe Indexer' })

    renderSettings({ indexers: [indexer] })
    await openIndexersTab()

    vi.mocked(api.testIndexer).mockResolvedValueOnce({ ok: true, status: 200, categories: 12, bookSearch: true, latencyMs: 20, searchResults: 3 })
    fireEvent.click(screen.getByRole('button', { name: 'common.test' }))
    await waitFor(() => expect(api.testIndexer).toHaveBeenCalledWith(9))
    expect(await screen.findByText('settings.indexers.testOk')).toBeInTheDocument()

    vi.mocked(api.testIndexer).mockResolvedValueOnce({ ok: true, status: 200, categories: 12, bookSearch: true, latencyMs: 20, searchResults: 0, searchError: 'no book results' })
    fireEvent.click(screen.getByRole('button', { name: 'common.test' }))
    expect(await screen.findByText(/settings\.indexers\.testWarn/)).toBeInTheDocument()
    expect(screen.getByText(/no book results/)).toBeInTheDocument()

    vi.mocked(api.testIndexer).mockRejectedValueOnce(new Error('bad key'))
    fireEvent.click(screen.getByRole('button', { name: 'common.test' }))
    expect(await screen.findByText('settings.indexers.testFail')).toBeInTheDocument()
  })

  it('adds Prowlarr and immediately syncs refreshed indexers', async () => {
    const added = makeProwlarr({ id: 31, name: 'Main Prowlarr', url: 'http://prowlarr:9696' })
    vi.mocked(api.addProwlarr).mockResolvedValue(added)
    vi.mocked(api.syncProwlarr).mockResolvedValue({ added: 2, updated: 1, removed: 0 })
    vi.mocked(api.listProwlarr).mockResolvedValueOnce([]).mockResolvedValueOnce([{ ...added, lastSyncAt: '2026-05-06T12:00:00Z' }])

    renderSettings()
    await openIndexersTab()

    fireEvent.click(screen.getByRole('button', { name: 'Add Prowlarr' }))
    fireEvent.change(screen.getByPlaceholderText('Prowlarr'), { target: { value: 'Main Prowlarr' } })
    fireEvent.change(screen.getByPlaceholderText('http://prowlarr:9696'), { target: { value: 'http://prowlarr:9696' } })
    fireEvent.change(screen.getByPlaceholderText('API Key'), { target: { value: 'prowlarr-secret' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save & sync' }))

    await waitFor(() => {
      expect(api.addProwlarr).toHaveBeenCalledWith({
        name: 'Main Prowlarr',
        url: 'http://prowlarr:9696',
        apiKey: 'prowlarr-secret',
        syncOnStartup: true,
        enabled: true,
      })
    })
    await waitFor(() => expect(api.syncProwlarr).toHaveBeenCalledWith(31))
    await waitFor(() => expect(api.listProwlarr).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(api.listIndexers).toHaveBeenCalledTimes(2))
  })

  it('keeps a newly added Prowlarr instance when immediate sync fails', async () => {
    const added = makeProwlarr({ id: 32, name: 'Fallback Prowlarr' })
    vi.mocked(api.addProwlarr).mockResolvedValue(added)
    vi.mocked(api.syncProwlarr).mockRejectedValue(new Error('sync failed'))

    renderSettings()
    await openIndexersTab()

    fireEvent.click(screen.getByRole('button', { name: 'Add Prowlarr' }))
    fireEvent.change(screen.getByPlaceholderText('Prowlarr'), { target: { value: 'Fallback Prowlarr' } })
    fireEvent.change(screen.getByPlaceholderText('http://prowlarr:9696'), { target: { value: 'http://prowlarr:9696' } })
    fireEvent.change(screen.getByPlaceholderText('API Key'), { target: { value: 'prowlarr-secret' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save & sync' }))

    await waitFor(() => expect(api.syncProwlarr).toHaveBeenCalledWith(32))
    expect(await screen.findByText('Fallback Prowlarr')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save & sync' })).not.toBeInTheDocument()
  })

  it('tests, syncs, and deletes an existing Prowlarr instance', async () => {
    const prowlarr = makeProwlarr({ id: 33, name: 'Library Prowlarr', lastSyncAt: '2026-05-06T12:00:00Z' })
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

    try {
      renderSettings({ prowlarr: [prowlarr], indexers: [makeIndexer({ id: 11, prowlarrInstanceId: 33 })] })
      vi.mocked(api.syncProwlarr).mockResolvedValue({ added: 1, updated: 2, removed: 3 })
      vi.mocked(api.listProwlarr).mockResolvedValue([{ ...prowlarr, lastSyncAt: '2026-05-06T13:00:00Z' }])
      await openIndexersTab()

      fireEvent.click(screen.getByRole('button', { name: 'Test' }))
      await waitFor(() => expect(api.testProwlarr).toHaveBeenCalledWith(33))
      expect(alertSpy).toHaveBeenCalledWith('Connected — Prowlarr 1.0.0')

      fireEvent.click(screen.getByRole('button', { name: 'Sync now' }))
      await waitFor(() => expect(api.syncProwlarr).toHaveBeenCalledWith(33))
      expect(await screen.findByText(/Synced.*added 1, updated 2, removed 3/)).toBeInTheDocument()
      await waitFor(() => expect(api.listIndexers).toHaveBeenCalledTimes(2))
      await waitFor(() => expect(api.listProwlarr).toHaveBeenCalledTimes(2))

      fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
      await waitFor(() => expect(api.deleteProwlarr).toHaveBeenCalledWith(33))
      expect(confirmSpy).toHaveBeenCalledWith('Delete Prowlarr instance "Library Prowlarr" and all its synced indexers?')
      await waitFor(() => expect(screen.queryByText('Library Prowlarr')).not.toBeInTheDocument())
    } finally {
      alertSpy.mockRestore()
      confirmSpy.mockRestore()
    }
  })

  it('adds a SABnzbd download client with API key, SSL, URL Base, and category mapping', async () => {
    renderSettings()
    await openClientsTab()

    fireEvent.click(screen.getByRole('button', { name: 'settings.clients.addButton' }))
    fireEvent.change(screen.getByPlaceholderText('Name'), { target: { value: 'SAB Books' } })
    fireEvent.change(screen.getByPlaceholderText('Host'), { target: { value: 'sabnzbd' } })
    fireEvent.change(screen.getByPlaceholderText('Port'), { target: { value: '8085' } })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Use SSL' }))
    fireEvent.change(screen.getByPlaceholderText('/sabnzbd'), { target: { value: ' /sab ' } })
    fireEvent.change(screen.getByPlaceholderText('API Key'), { target: { value: 'sab-secret' } })
    fireEvent.change(screen.getByDisplayValue('books'), { target: { value: 'ebooks' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(api.addDownloadClient).toHaveBeenCalledWith({
        name: 'SAB Books',
        host: 'sabnzbd',
        port: 8085,
        apiKey: 'sab-secret',
        username: '',
        password: '',
        category: 'ebooks',
        type: 'sabnzbd',
        enabled: true,
        useSsl: true,
        urlBase: '/sab',
      })
    })
    expect(await screen.findByText('SAB Books')).toBeInTheDocument()
  })

  it('updates add-client defaults and clears stale credentials when switching types', async () => {
    renderSettings()
    await openClientsTab()

    fireEvent.click(screen.getByRole('button', { name: 'settings.clients.addButton' }))
    fireEvent.change(screen.getByPlaceholderText('API Key'), { target: { value: 'stale-key' } })

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'nzbget' } })
    expect(screen.getByPlaceholderText('Name')).toHaveValue('NZBGet')
    expect(screen.getByPlaceholderText('Port')).toHaveValue('6789')
    expect(screen.getByPlaceholderText('Username')).toBeInTheDocument()
    expect(screen.getByPlaceholderText('Password')).toHaveValue('')

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'qbittorrent' } })
    expect(screen.getByPlaceholderText('Name')).toHaveValue('qBittorrent')
    expect(screen.getByPlaceholderText('Port')).toHaveValue('8080')

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'transmission' } })
    expect(screen.getByPlaceholderText('Name')).toHaveValue('Transmission')
    expect(screen.getByPlaceholderText('Port')).toHaveValue('9091')
    expect(screen.getByText('Download Directory')).toBeInTheDocument()

    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'deluge' } })
    expect(screen.getByPlaceholderText('Name')).toHaveValue('Deluge')
    expect(screen.getByPlaceholderText('Port')).toHaveValue('8112')
    expect(screen.getByText('Category / Label')).toBeInTheDocument()
    expect(screen.queryByPlaceholderText('Username')).not.toBeInTheDocument()
  })

  it.each([
    { type: 'nzbget', name: 'NZBGet', port: 6789, username: 'nzb-user', password: 'nzb-pass', category: 'books' },
    { type: 'qbittorrent', name: 'qBittorrent', port: 8080, username: 'qbit-user', password: 'qbit-pass', category: 'ebooks' },
    { type: 'transmission', name: 'Transmission', port: 9091, username: 'tr-user', password: 'tr-pass', category: '/downloads/books' },
    { type: 'deluge', name: 'Deluge', port: 8112, username: '', password: 'deluge-pass', category: 'books-audio' },
  ])('maps $name download client credentials on add', async ({ type, name, port, username, password, category }) => {
    renderSettings()
    await openClientsTab()

    fireEvent.click(screen.getByRole('button', { name: 'settings.clients.addButton' }))
    fireEvent.change(screen.getByRole('combobox'), { target: { value: type } })
    fireEvent.change(screen.getByPlaceholderText('Host'), { target: { value: `${type}.local` } })
    if (username) {
      fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: username } })
    }
    fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: password } })
    fireEvent.change(screen.getByDisplayValue('books'), { target: { value: category } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(api.addDownloadClient).toHaveBeenCalledWith({
        name,
        host: `${type}.local`,
        port,
        username,
        password,
        apiKey: '',
        category,
        type,
        enabled: true,
        useSsl: false,
        urlBase: '',
      })
    })
  })

  it('edits a download client with credential remapping, SSL, URL Base, and category updates', async () => {
    const client = makeClient({ id: 44, name: 'Old SAB', host: 'sab-old', apiKey: 'old-api' })

    renderSettings({ clients: [client] })
    await openClientsTab()

    fireEvent.click(screen.getByRole('button', { name: 'common.edit' }))
    fireEvent.change(screen.getByPlaceholderText('Name'), { target: { value: 'qBit Books' } })
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'qbittorrent' } })
    fireEvent.change(screen.getByPlaceholderText('Host'), { target: { value: 'qbittorrent' } })
    fireEvent.change(screen.getByPlaceholderText('Port'), { target: { value: '8081' } })
    fireEvent.click(screen.getByRole('checkbox', { name: 'Use SSL' }))
    fireEvent.change(screen.getByPlaceholderText('/sabnzbd'), { target: { value: ' /qbittorrent ' } })
    fireEvent.change(screen.getByPlaceholderText('Username'), { target: { value: 'qbit-user' } })
    fireEvent.change(screen.getByPlaceholderText('Password'), { target: { value: 'qbit-pass' } })
    fireEvent.change(screen.getByDisplayValue('books'), { target: { value: 'ebooks' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(api.updateDownloadClient).toHaveBeenCalledWith(44, {
        ...client,
        name: 'qBit Books',
        type: 'qbittorrent',
        host: 'qbittorrent',
        port: 8081,
        username: 'qbit-user',
        password: 'qbit-pass',
        apiKey: '',
        category: 'ebooks',
        useSsl: true,
        urlBase: '/qbittorrent',
      })
    })
    expect(await screen.findByText('qBit Books')).toBeInTheDocument()
  })

  it('toggles, tests, and deletes a download client', async () => {
    const client = makeClient({ id: 45, name: 'Client Actions', enabled: true })
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    vi.mocked(api.updateDownloadClient).mockResolvedValue({ ...client, enabled: false })

    try {
      renderSettings({ clients: [client] })
      await openClientsTab()

      fireEvent.click(screen.getByTitle('common.disable'))
      await waitFor(() => {
        expect(api.updateDownloadClient).toHaveBeenCalledWith(45, { ...client, enabled: false })
      })
      expect(await screen.findByTitle('common.enable')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('button', { name: 'common.test' }))
      await waitFor(() => expect(api.testDownloadClient).toHaveBeenCalledWith(45))
      expect(alertSpy).toHaveBeenCalledWith('common.connOk')

      fireEvent.click(screen.getByRole('button', { name: 'common.delete' }))
      await waitFor(() => expect(api.deleteDownloadClient).toHaveBeenCalledWith(45))
      await waitFor(() => expect(screen.queryByText('Client Actions')).not.toBeInTheDocument())
    } finally {
      alertSpy.mockRestore()
    }
  })
})
