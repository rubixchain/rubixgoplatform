package wallet

import (
	"fmt"
	"strings"
	"time"

	"github.com/rubixchain/rubixgoplatform/core/model"
)

// AdvisoryURLStorage is defined in wallet.go

// AddAdvisoryURL adds a new advisory URL to the database
func (w *Wallet) AddAdvisoryURL(advisoryURL *model.AdvisoryURL) error {
	// Validate network type
	if advisoryURL.NetworkType != "mainnet" && advisoryURL.NetworkType != "testnet" {
		return fmt.Errorf("invalid network type: %s. Must be 'mainnet' or 'testnet'", advisoryURL.NetworkType)
	}

	// Check if URL already exists for this network
	existing, err := w.GetAdvisoryURLByURL(advisoryURL.URL, advisoryURL.NetworkType)
	if err == nil && existing != nil {
		return fmt.Errorf("advisory URL already exists for network %s: %s", advisoryURL.NetworkType, advisoryURL.URL)
	}

	// If this is marked as default, unset other defaults for the same network
	if advisoryURL.IsDefault {
		err := w.UnsetDefaultAdvisoryURL(advisoryURL.NetworkType)
		if err != nil {
			w.log.Error("Failed to unset existing default", "network", advisoryURL.NetworkType, "err", err)
		}
	}

	// Set default values
	advisoryURL.CreatedAt = time.Now()
	advisoryURL.UpdatedAt = time.Now()
	advisoryURL.IsHealthy = true

	err = w.s.Write(AdvisoryURLStorage, advisoryURL)
	if err != nil {
		w.log.Error("Failed to add advisory URL", "err", err)
		return err
	}

	w.log.Info("Advisory URL added successfully", "url", advisoryURL.URL, "network", advisoryURL.NetworkType)
	return nil
}

// GetAdvisoryURLByURL gets an advisory URL by URL and network type
func (w *Wallet) GetAdvisoryURLByURL(url, networkType string) (*model.AdvisoryURL, error) {
	var advisoryURL model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURL, "url=? AND network_type=?", url, networkType)
	if err != nil {
		return nil, err
	}
	return &advisoryURL, nil
}

// GetAllAdvisoryURLs gets all advisory URLs
func (w *Wallet) GetAllAdvisoryURLs() ([]model.AdvisoryURL, error) {
	var advisoryURLs []model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURLs, "id > ?", 0)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []model.AdvisoryURL{}, nil
		}
		w.log.Error("Failed to get advisory URLs", "err", err)
		return nil, err
	}
	return advisoryURLs, nil
}

// GetAdvisoryURLsByNetwork gets all advisory URLs for a specific network
func (w *Wallet) GetAdvisoryURLsByNetwork(networkType string) ([]model.AdvisoryURL, error) {
	var advisoryURLs []model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURLs, "network_type=? AND is_active=? ORDER BY priority ASC, is_default DESC", networkType, true)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return []model.AdvisoryURL{}, nil
		}
		w.log.Error("Failed to get advisory URLs for network", "network", networkType, "err", err)
		return nil, err
	}
	return advisoryURLs, nil
}

// GetDefaultAdvisoryURL gets the default advisory URL for a network
func (w *Wallet) GetDefaultAdvisoryURL(networkType string) (*model.AdvisoryURL, error) {
	var advisoryURL model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURL, "network_type=? AND is_default=? AND is_active=?", networkType, true, true)
	if err != nil {
		// If no default found, try to get the highest priority active URL
		if strings.Contains(err.Error(), "no records found") {
			err = w.s.Read(AdvisoryURLStorage, &advisoryURL, "network_type=? AND is_active=? ORDER BY priority ASC LIMIT 1", networkType, true)
			if err != nil {
				return nil, fmt.Errorf("no active advisory URLs found for network %s", networkType)
			}
		} else {
			return nil, err
		}
	}
	return &advisoryURL, nil
}

// UnsetDefaultAdvisoryURL removes default flag from all URLs in a network
func (w *Wallet) UnsetDefaultAdvisoryURL(networkType string) error {
	// Get all current defaults for this network
	var advisoryURLs []model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURLs, "network_type=? AND is_default=?", networkType, true)
	if err != nil {
		if strings.Contains(err.Error(), "no records found") {
			return nil // No defaults to unset
		}
		return err
	}

	// Update each to remove default flag
	for _, url := range advisoryURLs {
		url.IsDefault = false
		url.UpdatedAt = time.Now()
		err = w.s.Update(AdvisoryURLStorage, &url, "id=?", url.ID)
		if err != nil {
			w.log.Error("Failed to unset default flag", "id", url.ID, "err", err)
		}
	}

	return nil
}

// SetDefaultAdvisoryURL sets a URL as default for its network
func (w *Wallet) SetDefaultAdvisoryURL(id int) error {
	// Get the URL to be set as default
	var advisoryURL model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURL, "id=?", id)
	if err != nil {
		return fmt.Errorf("advisory URL not found: %d", id)
	}

	// Unset existing defaults for this network
	err = w.UnsetDefaultAdvisoryURL(advisoryURL.NetworkType)
	if err != nil {
		return err
	}

	// Set this URL as default
	advisoryURL.IsDefault = true
	advisoryURL.UpdatedAt = time.Now()
	err = w.s.Update(AdvisoryURLStorage, &advisoryURL, "id=?", id)
	if err != nil {
		w.log.Error("Failed to set default advisory URL", "id", id, "err", err)
		return err
	}

	w.log.Info("Advisory URL set as default", "id", id, "url", advisoryURL.URL, "network", advisoryURL.NetworkType)
	return nil
}

// UpdateAdvisoryURL updates an advisory URL
func (w *Wallet) UpdateAdvisoryURL(id int, updates *model.AdvisoryURL) error {
	// Get existing URL
	var existingURL model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &existingURL, "id=?", id)
	if err != nil {
		return fmt.Errorf("advisory URL not found: %d", id)
	}

	// If setting as default, unset others
	if updates.IsDefault && !existingURL.IsDefault {
		err = w.UnsetDefaultAdvisoryURL(existingURL.NetworkType)
		if err != nil {
			return err
		}
	}

	// Update fields
	if updates.URL != "" {
		existingURL.URL = updates.URL
	}
	if updates.NetworkType != "" {
		existingURL.NetworkType = updates.NetworkType
	}
	existingURL.IsDefault = updates.IsDefault
	existingURL.IsActive = updates.IsActive
	if updates.Priority > 0 {
		existingURL.Priority = updates.Priority
	}
	if updates.Description != "" {
		existingURL.Description = updates.Description
	}
	existingURL.UpdatedAt = time.Now()

	err = w.s.Update(AdvisoryURLStorage, &existingURL, "id=?", id)
	if err != nil {
		w.log.Error("Failed to update advisory URL", "id", id, "err", err)
		return err
	}

	w.log.Info("Advisory URL updated successfully", "id", id, "url", existingURL.URL)
	return nil
}

// DeleteAdvisoryURL deletes an advisory URL
func (w *Wallet) DeleteAdvisoryURL(id int) error {
	err := w.s.Delete(AdvisoryURLStorage, &model.AdvisoryURL{}, "id=?", id)
	if err != nil {
		w.log.Error("Failed to delete advisory URL", "id", id, "err", err)
		return err
	}

	w.log.Info("Advisory URL deleted successfully", "id", id)
	return nil
}

// UpdateAdvisoryURLHealth updates the health status of an advisory URL
func (w *Wallet) UpdateAdvisoryURLHealth(id int, isHealthy bool) error {
	var advisoryURL model.AdvisoryURL
	err := w.s.Read(AdvisoryURLStorage, &advisoryURL, "id=?", id)
	if err != nil {
		return err
	}

	advisoryURL.IsHealthy = isHealthy
	advisoryURL.LastTested = time.Now()
	advisoryURL.UpdatedAt = time.Now()

	err = w.s.Update(AdvisoryURLStorage, &advisoryURL, "id=?", id)
	if err != nil {
		w.log.Error("Failed to update advisory URL health", "id", id, "err", err)
		return err
	}

	return nil
}

// InitializeDefaultAdvisoryURLs creates default advisory URLs if none exist
func (w *Wallet) InitializeDefaultAdvisoryURLs() error {
	// Try to ensure table exists first
	err := w.s.Init(AdvisoryURLStorage, &model.AdvisoryURL{}, true)
	if err != nil {
		w.log.Error("Failed to ensure advisory URL table exists", "err", err)
		return err
	}

	// Check if any URLs exist
	existing, err := w.GetAllAdvisoryURLs()
	if err != nil {
		w.log.Error("Failed to check existing advisory URLs", "err", err)
		return err
	}

	if len(existing) > 0 {
		w.log.Info("Advisory URLs already exist, skipping initialization", "count", len(existing))
		return nil
	}

	// Default URLs for both networks
	defaultURLs := []model.AdvisoryURL{
		{
			URL:         "https://testnet-pool.universe.rubix.net",
			NetworkType: "testnet",
			IsDefault:   true,
			IsActive:    true,
			Priority:    1,
			Description: "Official TestNet Advisory Node Service",
		},
		{
			URL:         "https://mainnet-pool.universe.rubix.net",
			NetworkType: "mainnet",
			IsDefault:   true,
			IsActive:    true,
			Priority:    1,
			Description: "Official MainNet Advisory Node Service",
		},
	}

	// Add default URLs
	for _, url := range defaultURLs {
		err = w.AddAdvisoryURL(&url)
		if err != nil {
			w.log.Error("Failed to add default advisory URL", "url", url.URL, "network", url.NetworkType, "err", err)
			return err
		}
	}

	w.log.Info("Default advisory URLs initialized successfully")
	return nil
}
