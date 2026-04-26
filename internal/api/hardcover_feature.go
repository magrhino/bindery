package api

import (
	"context"
	"strings"

	"github.com/vavallee/bindery/internal/db"
)

const (
	SettingHardcoverAPIToken              = "hardcover.api_token" //nolint:gosec // settings key name, not a credential value
	SettingHardcoverEnhancedSeriesEnabled = "hardcover.enhanced_series_enabled"

	HardcoverDisabledReasonEnvDisabled   = "env_disabled"
	HardcoverDisabledReasonMissingToken  = "missing_token"
	HardcoverDisabledReasonAdminDisabled = "admin_disabled"
)

type HardcoverFeatureState struct {
	EnhancedHardcoverAPI            bool   `json:"enhancedHardcoverApi"`
	HardcoverTokenConfigured        bool   `json:"hardcoverTokenConfigured"`
	EnhancedHardcoverDisabledReason string `json:"enhancedHardcoverDisabledReason,omitempty"`
}

func GetHardcoverAPIToken(ctx context.Context, settings *db.SettingsRepo) string {
	if settings == nil {
		return ""
	}
	setting, _ := settings.Get(ctx, SettingHardcoverAPIToken)
	if setting == nil {
		return ""
	}
	return strings.TrimSpace(setting.Value)
}

func HardcoverFeatureStateFor(ctx context.Context, settings *db.SettingsRepo, envEnabled bool) HardcoverFeatureState {
	tokenConfigured := GetHardcoverAPIToken(ctx, settings) != ""
	adminEnabled := false
	if settings != nil {
		if setting, _ := settings.Get(ctx, SettingHardcoverEnhancedSeriesEnabled); setting != nil {
			adminEnabled = strings.EqualFold(strings.TrimSpace(setting.Value), "true")
		}
	}

	state := HardcoverFeatureState{HardcoverTokenConfigured: tokenConfigured}
	switch {
	case !envEnabled:
		state.EnhancedHardcoverDisabledReason = HardcoverDisabledReasonEnvDisabled
	case !tokenConfigured:
		state.EnhancedHardcoverDisabledReason = HardcoverDisabledReasonMissingToken
	case !adminEnabled:
		state.EnhancedHardcoverDisabledReason = HardcoverDisabledReasonAdminDisabled
	default:
		state.EnhancedHardcoverAPI = true
	}
	return state
}
