package server

import (
	"fmt"
	"strconv"

	"github.com/rubixchain/rubixgoplatform/core/model"
	"github.com/rubixchain/rubixgoplatform/wrapper/ensweb"
)

// GetAdvisoryURLs returns all advisory URLs
func (s *Server) GetAdvisoryURLs(req *ensweb.Request) *ensweb.Result {
	urls, err := s.c.GetAllAdvisoryURLs()
	if err != nil {
		s.log.Error("Failed to get advisory URLs", "err", err)
		return s.BasicResponse(req, false, "Failed to get advisory URLs", nil)
	}

	return s.BasicResponse(req, true, "Advisory URLs retrieved successfully", map[string]interface{}{
		"urls": urls,
	})
}

// GetAdvisoryURLsByNetwork returns advisory URLs for a specific network
func (s *Server) GetAdvisoryURLsByNetwork(req *ensweb.Request) *ensweb.Result {
	networkType := s.GetQuerry(req, "network")
	if networkType == "" {
		return s.BasicResponse(req, false, "Network type is required", nil)
	}

	if networkType != "mainnet" && networkType != "testnet" {
		return s.BasicResponse(req, false, "Invalid network type. Must be 'mainnet' or 'testnet'", nil)
	}

	urls, err := s.c.GetAdvisoryURLsByNetwork(networkType)
	if err != nil {
		s.log.Error("Failed to get advisory URLs for network", "network", networkType, "err", err)
		return s.BasicResponse(req, false, "Failed to get advisory URLs", nil)
	}

	return s.BasicResponse(req, true, "Advisory URLs retrieved successfully", map[string]interface{}{
		"network": networkType,
		"urls":    urls,
	})
}

// AddAdvisoryURL adds a new advisory URL
func (s *Server) AddAdvisoryURL(req *ensweb.Request) *ensweb.Result {
	var urlReq model.AdvisoryURLRequest
	err := s.ParseJSON(req, &urlReq)
	if err != nil {
		s.log.Error("Failed to parse advisory URL request", "err", err)
		return s.BasicResponse(req, false, "Invalid request format", nil)
	}

	// Validate required fields
	if urlReq.URL == "" {
		return s.BasicResponse(req, false, "URL is required", nil)
	}
	if urlReq.NetworkType == "" {
		return s.BasicResponse(req, false, "Network type is required", nil)
	}
	if urlReq.NetworkType != "mainnet" && urlReq.NetworkType != "testnet" {
		return s.BasicResponse(req, false, "Invalid network type. Must be 'mainnet' or 'testnet'", nil)
	}

	// Set default values
	if urlReq.Priority == 0 {
		urlReq.Priority = 1
	}

	advisoryURL := &model.AdvisoryURL{
		URL:         urlReq.URL,
		NetworkType: urlReq.NetworkType,
		IsDefault:   urlReq.IsDefault,
		IsActive:    urlReq.IsActive,
		Priority:    urlReq.Priority,
		Description: urlReq.Description,
	}

	err = s.c.AddAdvisoryURL(advisoryURL)
	if err != nil {
		s.log.Error("Failed to add advisory URL", "err", err)
		return s.BasicResponse(req, false, fmt.Sprintf("Failed to add advisory URL: %v", err), nil)
	}

	s.log.Info("Advisory URL added successfully", "url", urlReq.URL, "network", urlReq.NetworkType)
	return s.BasicResponse(req, true, "Advisory URL added successfully", map[string]interface{}{
		"url": advisoryURL,
	})
}

// UpdateAdvisoryURL updates an existing advisory URL
func (s *Server) UpdateAdvisoryURL(req *ensweb.Request) *ensweb.Result {
	idStr := s.GetQuerry(req, "id")
	if idStr == "" {
		return s.BasicResponse(req, false, "ID is required", nil)
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid ID format", nil)
	}

	var urlReq model.AdvisoryURLRequest
	err = s.ParseJSON(req, &urlReq)
	if err != nil {
		s.log.Error("Failed to parse advisory URL update request", "err", err)
		return s.BasicResponse(req, false, "Invalid request format", nil)
	}

	// Validate network type if provided
	if urlReq.NetworkType != "" && urlReq.NetworkType != "mainnet" && urlReq.NetworkType != "testnet" {
		return s.BasicResponse(req, false, "Invalid network type. Must be 'mainnet' or 'testnet'", nil)
	}

	updates := &model.AdvisoryURL{
		URL:         urlReq.URL,
		NetworkType: urlReq.NetworkType,
		IsDefault:   urlReq.IsDefault,
		IsActive:    urlReq.IsActive,
		Priority:    urlReq.Priority,
		Description: urlReq.Description,
	}

	err = s.c.UpdateAdvisoryURL(id, updates)
	if err != nil {
		s.log.Error("Failed to update advisory URL", "id", id, "err", err)
		return s.BasicResponse(req, false, fmt.Sprintf("Failed to update advisory URL: %v", err), nil)
	}

	s.log.Info("Advisory URL updated successfully", "id", id)
	return s.BasicResponse(req, true, "Advisory URL updated successfully", nil)
}

// SetDefaultAdvisoryURL sets an advisory URL as default
func (s *Server) SetDefaultAdvisoryURL(req *ensweb.Request) *ensweb.Result {
	idStr := s.GetQuerry(req, "id")
	if idStr == "" {
		return s.BasicResponse(req, false, "ID is required", nil)
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid ID format", nil)
	}

	err = s.c.SetDefaultAdvisoryURL(id)
	if err != nil {
		s.log.Error("Failed to set default advisory URL", "id", id, "err", err)
		return s.BasicResponse(req, false, fmt.Sprintf("Failed to set default advisory URL: %v", err), nil)
	}

	s.log.Info("Advisory URL set as default successfully", "id", id)
	return s.BasicResponse(req, true, "Advisory URL set as default successfully", nil)
}

// DeleteAdvisoryURL deletes an advisory URL
func (s *Server) DeleteAdvisoryURL(req *ensweb.Request) *ensweb.Result {
	idStr := s.GetQuerry(req, "id")
	if idStr == "" {
		return s.BasicResponse(req, false, "ID is required", nil)
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return s.BasicResponse(req, false, "Invalid ID format", nil)
	}

	err = s.c.DeleteAdvisoryURL(id)
	if err != nil {
		s.log.Error("Failed to delete advisory URL", "id", id, "err", err)
		return s.BasicResponse(req, false, fmt.Sprintf("Failed to delete advisory URL: %v", err), nil)
	}

	s.log.Info("Advisory URL deleted successfully", "id", id)
	return s.BasicResponse(req, true, "Advisory URL deleted successfully", nil)
}

// GetCurrentAdvisoryURL returns the currently active advisory URL
func (s *Server) GetCurrentAdvisoryURL(req *ensweb.Request) *ensweb.Result {
	networkType := s.GetQuerry(req, "network")
	if networkType == "" {
		// Determine current network type based on node configuration
		networkType = "testnet"
		if !s.c.IsTestNet() {
			networkType = "mainnet"
		}
	}

	if networkType != "mainnet" && networkType != "testnet" {
		return s.BasicResponse(req, false, "Invalid network type. Must be 'mainnet' or 'testnet'", nil)
	}

	currentURL, err := s.c.GetAdvisoryNodeURL()
	if err != nil {
		s.log.Error("Failed to get current advisory URL", "network", networkType, "err", err)
		return s.BasicResponse(req, false, "No advisory URL available", nil)
	}

	// Get the default URL details from database
	defaultURL, err := s.c.GetDefaultAdvisoryURL(networkType)
	if err != nil {
		return s.BasicResponse(req, true, "Current advisory URL retrieved", map[string]interface{}{
			"network":     networkType,
			"current_url": currentURL,
			"status":      "active_from_failover",
		})
	}

	return s.BasicResponse(req, true, "Current advisory URL retrieved", map[string]interface{}{
		"network":     networkType,
		"current_url": currentURL,
		"default_url": defaultURL,
		"status":      "active",
	})
}
