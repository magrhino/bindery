package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/vavallee/bindery/internal/abs"
	"github.com/vavallee/bindery/internal/db"
)

const (
	SettingABSBaseURL   = "abs.base_url"
	SettingABSAPIKey    = "abs.api_key" //nolint:gosec // settings key name, not a credential value
	SettingABSLibraryID = "abs.library_id"
	SettingABSEnabled   = "abs.enabled"
	SettingABSLabel     = "abs.label"
	SettingABSPathRemap = "abs.path_remap"
)

type absClient interface {
	Authorize(ctx context.Context) (*abs.AuthorizeResponse, error)
	ListLibraries(ctx context.Context) ([]abs.Library, error)
	GetLibrary(ctx context.Context, id string) (*abs.Library, error)
}

type absClientFactory func(baseURL, apiKey string) (absClient, error)

type ABSHandler struct {
	settings       *db.SettingsRepo
	newFn          absClientFactory
	featureEnabled bool
}

type ABSConfigResponse struct {
	FeatureEnabled   bool   `json:"featureEnabled"`
	BaseURL          string `json:"baseUrl"`
	Label            string `json:"label"`
	Enabled          bool   `json:"enabled"`
	LibraryID        string `json:"libraryId"`
	PathRemap        string `json:"pathRemap"`
	APIKeyConfigured bool   `json:"apiKeyConfigured"`
}

type absConfigRequest struct {
	BaseURL   *string `json:"baseUrl"`
	Label     *string `json:"label"`
	Enabled   *bool   `json:"enabled"`
	LibraryID *string `json:"libraryId"`
	PathRemap *string `json:"pathRemap"`
	APIKey    *string `json:"apiKey"`
}

type absProbeRequest struct {
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
}

type absLibraryResponse struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	MediaType string              `json:"mediaType"`
	Icon      string              `json:"icon"`
	Provider  string              `json:"provider"`
	Folders   []abs.LibraryFolder `json:"folders"`
}

func NewABSHandler(settings *db.SettingsRepo) *ABSHandler {
	return &ABSHandler{
		settings:       settings,
		featureEnabled: true,
		newFn: func(baseURL, apiKey string) (absClient, error) {
			return abs.NewClient(baseURL, apiKey)
		},
	}
}

func (h *ABSHandler) WithFeatureEnabled(enabled bool) *ABSHandler {
	h.featureEnabled = enabled
	return h
}

func (h *ABSHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.loadConfig())
}

func (h *ABSHandler) SetConfig(w http.ResponseWriter, r *http.Request) {
	current := h.loadStoredConfig()

	var req absConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	baseURL := current.BaseURL
	if req.BaseURL != nil {
		baseURL = strings.TrimSpace(*req.BaseURL)
	}
	if baseURL != "" {
		normalized, err := abs.NormalizeBaseURL(baseURL)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		baseURL = normalized
	}

	apiKey := current.APIKey
	if req.APIKey != nil {
		apiKey = strings.TrimSpace(*req.APIKey)
	}
	label := current.Label
	if req.Label != nil {
		label = strings.TrimSpace(*req.Label)
	}
	if label == "" {
		label = "Audiobookshelf"
	}
	libraryID := current.LibraryID
	if req.LibraryID != nil {
		libraryID = strings.TrimSpace(*req.LibraryID)
	}
	pathRemap := current.PathRemap
	if req.PathRemap != nil {
		pathRemap = strings.TrimSpace(*req.PathRemap)
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if libraryID != "" {
		client, err := h.newConfiguredClient(baseURL, apiKey)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		lib, err := client.GetLibrary(r.Context(), libraryID)
		if err != nil {
			h.writeProbeError(w, "abs save validation failed", baseURL, err)
			return
		}
		if lib.MediaType != "book" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("library %q is %q, expected book", lib.Name, lib.MediaType)})
			return
		}
	}

	if err := h.settings.Set(r.Context(), SettingABSBaseURL, baseURL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.settings.Set(r.Context(), SettingABSLabel, label); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.settings.Set(r.Context(), SettingABSEnabled, boolString(enabled)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.settings.Set(r.Context(), SettingABSLibraryID, libraryID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.settings.Set(r.Context(), SettingABSPathRemap, pathRemap); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if req.APIKey != nil && strings.TrimSpace(*req.APIKey) != "" {
		if err := h.settings.Set(r.Context(), SettingABSAPIKey, strings.TrimSpace(*req.APIKey)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, http.StatusOK, ABSConfigResponse{
		BaseURL:          baseURL,
		Label:            label,
		Enabled:          enabled,
		LibraryID:        libraryID,
		PathRemap:        pathRemap,
		APIKeyConfigured: apiKey != "",
	})
}

func (h *ABSHandler) Test(w http.ResponseWriter, r *http.Request) {
	client, err := h.clientFromProbe(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	authz, err := client.Authorize(r.Context())
	if err != nil {
		h.writeProbeError(w, "abs test failed", "", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":          "connected",
		"username":         authz.User.Username,
		"userType":         authz.User.Type,
		"defaultLibraryId": authz.UserDefaultLibraryID,
		"serverVersion":    authz.ServerSettings.Version,
		"source":           authz.Source,
	})
}

func (h *ABSHandler) Libraries(w http.ResponseWriter, r *http.Request) {
	client, err := h.clientFromProbe(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	libraries, err := client.ListLibraries(r.Context())
	if err != nil {
		h.writeProbeError(w, "abs list libraries failed", "", err)
		return
	}

	out := make([]absLibraryResponse, 0, len(libraries))
	for _, lib := range libraries {
		if lib.MediaType != "book" {
			continue
		}
		out = append(out, absLibraryResponse{
			ID:        lib.ID,
			Name:      lib.Name,
			MediaType: lib.MediaType,
			Icon:      lib.Icon,
			Provider:  lib.Provider,
			Folders:   lib.Folders,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type ABSStoredConfig struct {
	BaseURL   string
	APIKey    string
	Label     string
	LibraryID string
	PathRemap string
	Enabled   bool
}

func LoadABSConfig(settings *db.SettingsRepo) ABSStoredConfig {
	get := func(key string) string {
		s, _ := settings.Get(contextBackground(), key)
		if s == nil {
			return ""
		}
		return s.Value
	}
	label := get(SettingABSLabel)
	if label == "" {
		label = "Audiobookshelf"
	}
	return ABSStoredConfig{
		BaseURL:   get(SettingABSBaseURL),
		APIKey:    get(SettingABSAPIKey),
		Label:     label,
		LibraryID: get(SettingABSLibraryID),
		PathRemap: get(SettingABSPathRemap),
		Enabled:   strings.EqualFold(get(SettingABSEnabled), "true"),
	}
}

func (h *ABSHandler) loadStoredConfig() ABSStoredConfig {
	return LoadABSConfig(h.settings)
}

func (h *ABSHandler) loadConfig() ABSConfigResponse {
	cfg := h.loadStoredConfig()
	return ABSConfigResponse{
		FeatureEnabled:   h.featureEnabled,
		BaseURL:          cfg.BaseURL,
		Label:            cfg.Label,
		Enabled:          cfg.Enabled,
		LibraryID:        cfg.LibraryID,
		PathRemap:        cfg.PathRemap,
		APIKeyConfigured: cfg.APIKey != "",
	}
}

func (h *ABSHandler) clientFromProbe(r *http.Request) (absClient, error) {
	current := h.loadStoredConfig()
	req := absProbeRequest{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			return nil, errors.New("invalid request body")
		}
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		baseURL = current.BaseURL
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = current.APIKey
	}
	return h.newConfiguredClient(baseURL, apiKey)
}

func (h *ABSHandler) newConfiguredClient(baseURL, apiKey string) (absClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("base_url is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api_key is required")
	}
	return h.newFn(baseURL, apiKey)
}

func (h *ABSHandler) writeProbeError(w http.ResponseWriter, logMsg, baseURL string, err error) {
	var apiErr *abs.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "ABS rejected the API key"})
			return
		case http.StatusNotFound:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "library not found or not accessible"})
			return
		default:
			slog.Warn(logMsg, "status", apiErr.StatusCode, "host", redactABSHost(baseURL), "error", apiErr.Message)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": apiErr.Message})
			return
		}
	}
	slog.Warn(logMsg, "host", redactABSHost(baseURL), "error", err)
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
}

func redactABSHost(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
