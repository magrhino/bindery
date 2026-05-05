// Package abscontract provides the pinned Audiobookshelf contract harness and fixtures.
package abscontract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const PinnedBaselineVersion = "2.33.2"
const (
	defaultContractAPIKey        = "fixture-key"
	defaultLimitedContractAPIKey = "limited-key"
	defaultContractLibraryID     = "lib-books"
)

type Baseline struct {
	Version         string
	FixtureManifest string
}

type HarnessConfig struct {
	BaseURL        string
	APIKey         string
	LimitedAPIKey  string
	LibraryID      string
	UseExternalABS bool
	Baseline       Baseline
}

type FixtureManifest struct {
	BaselineVersion string            `json:"baselineVersion"`
	Scenarios       []FixtureScenario `json:"scenarios"`
}

type FixtureScenario struct {
	ID                         string `json:"id"`
	SeedPath                   string `json:"seedPath"`
	ExpectedMediaType          string `json:"expectedMediaType"`
	ExpectedItemID             string `json:"expectedItemId,omitempty"`
	RequiresDetail             bool   `json:"requiresDetail"`
	AccessibleToLimitedAccount bool   `json:"accessibleToLimitedAccount"`
	ExpectsEbook               bool   `json:"expectsEbook,omitempty"`
	ExpectsSeries              bool   `json:"expectsSeries,omitempty"`
}

func LoadHarnessConfig() HarnessConfig {
	baseURL := strings.TrimSpace(os.Getenv("BINDERY_ABS_CONTRACT_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("BINDERY_ABS_CONTRACT_API_KEY"))
	if apiKey == "" {
		apiKey = defaultContractAPIKey
	}
	limitedAPIKey := strings.TrimSpace(os.Getenv("BINDERY_ABS_CONTRACT_LIMITED_API_KEY"))
	if limitedAPIKey == "" {
		limitedAPIKey = defaultLimitedContractAPIKey
	}
	libraryID := strings.TrimSpace(os.Getenv("BINDERY_ABS_CONTRACT_LIBRARY_ID"))
	if libraryID == "" {
		libraryID = defaultContractLibraryID
	}
	return HarnessConfig{
		BaseURL:        baseURL,
		APIKey:         apiKey,
		LimitedAPIKey:  limitedAPIKey,
		LibraryID:      libraryID,
		UseExternalABS: baseURL != "",
		Baseline: Baseline{
			Version:         PinnedBaselineVersion,
			FixtureManifest: filepath.Join("testdata", "fixtures", "manifest.json"),
		},
	}
}

func LoadFixtureManifest(cfg HarnessConfig) (*FixtureManifest, error) {
	data, err := os.ReadFile(cfg.Baseline.FixtureManifest)
	if err != nil {
		return nil, fmt.Errorf("read fixture manifest: %w", err)
	}
	var manifest FixtureManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode fixture manifest: %w", err)
	}
	return &manifest, nil
}
