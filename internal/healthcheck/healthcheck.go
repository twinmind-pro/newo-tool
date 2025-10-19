package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/twinmind/newo-tool/internal/config"
	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
)

// CheckConfig performs checks on the newo.toml configuration file.
func CheckConfig() error {
	path := config.DefaultTomlPath // "newo.toml"

	// 1. Check for newo.toml existence and readability, and format validity.
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration file '%s' not found. Please create it or ensure it's in the project root directory.", path)
		}
		return fmt.Errorf("failed to read configuration file '%s': %w", path, err)
	}

	var cfg config.TomlConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return fmt.Errorf("failed to parse configuration file '%s' (invalid TOML format): %w", path, err)
	}

	// 2. Check for presence of essential configuration fields.
	var missingFields []string

	if strings.TrimSpace(cfg.Defaults.BaseURL) == "" {
		missingFields = append(missingFields, "defaults.base_url")
	}
	if cfg.Defaults.OutputRoot == nil || strings.TrimSpace(*cfg.Defaults.OutputRoot) == "" {
		missingFields = append(missingFields, "defaults.output_root")
	}

	// Check for API keys in customers
	hasAPIKey := false
	for _, customer := range cfg.Customers {
		if strings.TrimSpace(customer.APIKey) != "" {
			hasAPIKey = true
			break
		}
	}
	if !hasAPIKey {
		missingFields = append(missingFields, "at least one customer.api_key")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("the following required fields are missing from configuration file '%s': %s", path, strings.Join(missingFields, ", "))
	}

	return nil
}

// CheckFilesystem performs checks on filesystem permissions and lock file capabilities.
func CheckFilesystem(env config.Env) error {
	// 1. Ensure output_root exists and is writable.
	outputRoot := env.OutputRoot
	if outputRoot == "" {
		outputRoot = fsutil.DefaultCustomersDir // Fallback to default if not set (should be caught by config check)
	}

	if err := fsutil.EnsureDir(outputRoot); err != nil {
		return fmt.Errorf("failed to create or access output_root directory '%s': %w", outputRoot, err)
	}

	// Test writability by creating a temporary file
	testFilePath := filepath.Join(outputRoot, fmt.Sprintf(".healthcheck_test_%d", os.Getpid()))
	if err := os.WriteFile(testFilePath, []byte("healthcheck"), fsutil.FilePerm); err != nil {
		return fmt.Errorf("output_root directory '%s' is not writable: %w", outputRoot, err)
	}
	_ = os.Remove(testFilePath) // Clean up the test file

	// 2. Check lock file creation/deletion capabilities.
	releaseLock, err := fsutil.AcquireLock("healthcheck_test")
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	if err := releaseLock(); err != nil {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}

	return nil
}

type apiKeyEntry struct {
	Key         string
	CustomerIDN string
}

// CheckPlatformConnectivity performs checks on NEWO platform connectivity and API key validity.
func CheckPlatformConnectivity(ctx context.Context, env config.Env) (string, error) {
	// 1. Check basic network connectivity to BaseURL.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(env.BaseURL)
	if err != nil {
		return "", fmt.Errorf("failed to connect to base URL '%s': %w", env.BaseURL, err)
	}
	resp.Body.Close()

	// 2. Test API key validity and platform connectivity with a lightweight API call.
	var apiKeysToTest []apiKeyEntry

	// Prioritize API key from environment variable
	if env.APIKey != "" {
		apiKeysToTest = append(apiKeysToTest, apiKeyEntry{Key: env.APIKey, CustomerIDN: "(from NEWO_API_KEY)"})
	}

	// Add API keys from file customers
	for _, customer := range env.FileCustomers {
		if customer.APIKey != "" {
			apiKeysToTest = append(apiKeysToTest, apiKeyEntry{Key: customer.APIKey, CustomerIDN: customer.IDN})
		}
	}

	if len(apiKeysToTest) == 0 {
		return "", fmt.Errorf("API key is not set in configuration (NEWO_API_KEY or customer.api_key). Cannot check platform connectivity.")
	}

	var lastErr error
	for _, entry := range apiKeysToTest {
		childCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		// Exchange API key for an access token
		tokenResp, err := platform.ExchangeAPIKeyForToken(childCtx, env.BaseURL, entry.Key)
		if err != nil {
			lastErr = fmt.Errorf("failed to exchange API key for access token for customer '%s': %w", entry.CustomerIDN, err)
			continue
		}
		accessToken := tokenResp.AccessToken

		platformClient, err := platform.NewClient(env.BaseURL, accessToken)
		if err != nil {
			lastErr = fmt.Errorf("failed to create platform client for customer '%s': %w", entry.CustomerIDN, err)
			continue
		}

		// Use ListProjects as a lightweight API call
		_, err = platformClient.ListProjects(childCtx)
		if err == nil {
			return entry.CustomerIDN, nil // Success with this API key
		}
		lastErr = fmt.Errorf("failed to perform test API call (ListProjects) with API key for customer '%s': %w", entry.CustomerIDN, err)
	}

	return "", fmt.Errorf("all attempts to connect to platform failed: %w", lastErr)
}

// CheckLocalState performs checks on the local state files (map.json and hashes.json).
func CheckLocalState(env config.Env) (string, error) {
	var customerIDNsToTest []string

	if env.DefaultCustomer != "" {
		customerIDNsToTest = append(customerIDNsToTest, env.DefaultCustomer)
	} else {
		for _, customer := range env.FileCustomers {
			customerIDNsToTest = append(customerIDNsToTest, customer.IDN)
		}
	}

	if len(customerIDNsToTest) == 0 {
		return "", fmt.Errorf("default customerIDN is not specified in configuration (defaults.default_customer or NEWO_DEFAULT_CUSTOMER), and no customers with defined IDNs in newo.toml. Cannot check local state.")
	}

	var lastErr error
	for _, customerIDN := range customerIDNsToTest {
		// Check map.json
		_, err := state.LoadProjectMap(customerIDN)
		if err != nil && !os.IsNotExist(err) {
			lastErr = fmt.Errorf("failed to load map.json for customer '%s': %w", customerIDN, err)
			continue
		}

		// Check hashes.json
		_, err = state.LoadHashes(customerIDN)
		if err != nil && !os.IsNotExist(err) {
			lastErr = fmt.Errorf("failed to load hashes.json for customer '%s': %w", customerIDN, err)
			continue
		}
		return customerIDN, nil // Success with this customer's local state
	}

	return "", fmt.Errorf("all attempts to check local state failed: %w", lastErr)
}

// CheckExternalTools performs checks for the availability of external tools.
func CheckExternalTools() error {
	// Check for Git availability
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("tool 'git' not found in PATH. Please install Git and ensure it's available in your system's PATH: %w", err)
	}

	return nil
}
