package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	ipfsnode "github.com/ipfs/go-ipfs-api"
	"github.com/rubixchain/rubixgoplatform/core/config"
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// IPFSRecoveryManager handles IPFS crash recovery and automatic restart
type IPFSRecoveryManager struct {
	core *Core
	log  logger.Logger
	cfg  *config.Config

	// Recovery state
	mu            sync.RWMutex
	isRecovering  bool
	recoveryCount int
	maxRecoveries int
	lastCrashTime time.Time

	// Process management
	ipfsProcess *exec.Cmd
	processMu   sync.Mutex

	// Recovery control
	recoveryCtx    context.Context
	recoveryCancel context.CancelFunc
	recoveryWg     sync.WaitGroup

	// Configuration
	restartDelay  time.Duration
	healthTimeout time.Duration
	maxRetries    int
}

// NewIPFSRecoveryManager creates a new IPFS recovery manager
func NewIPFSRecoveryManager(core *Core) *IPFSRecoveryManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Set default values
	maxRecoveries := 10
	restartDelay := 5 * time.Second
	healthTimeout := 30 * time.Second
	monitorInterval := 10 * time.Second

	// Override with config if available
	if core.cfg.CfgData.IPFSRecovery != nil {
		if core.cfg.CfgData.IPFSRecovery.MaxRecoveries > 0 {
			maxRecoveries = core.cfg.CfgData.IPFSRecovery.MaxRecoveries
		}
		if core.cfg.CfgData.IPFSRecovery.RestartDelay > 0 {
			restartDelay = core.cfg.CfgData.IPFSRecovery.RestartDelay
		}
		if core.cfg.CfgData.IPFSRecovery.HealthTimeout > 0 {
			healthTimeout = core.cfg.CfgData.IPFSRecovery.HealthTimeout
		}
		if core.cfg.CfgData.IPFSRecovery.MonitorInterval > 0 {
			monitorInterval = core.cfg.CfgData.IPFSRecovery.MonitorInterval
		}
	}

	rm := &IPFSRecoveryManager{
		core:           core,
		log:            core.log.Named("IPFSRecovery"),
		cfg:            core.cfg,
		isRecovering:   false,
		recoveryCount:  0,
		maxRecoveries:  maxRecoveries,
		recoveryCtx:    ctx,
		recoveryCancel: cancel,
		restartDelay:   restartDelay,
		healthTimeout:  healthTimeout,
		maxRetries:     3,
	}

	// Start recovery monitor
	rm.startRecoveryMonitor(monitorInterval)

	return rm
}

// startRecoveryMonitor monitors IPFS health and triggers recovery if needed
func (rm *IPFSRecoveryManager) startRecoveryMonitor(interval time.Duration) {
	rm.recoveryWg.Add(1)
	go func() {
		defer rm.recoveryWg.Done()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-rm.recoveryCtx.Done():
				rm.log.Info("IPFS recovery monitor stopped")
				return
			case <-ticker.C:
				rm.checkAndRecover()
			}
		}
	}()

	rm.log.Info("IPFS recovery monitor started", "interval", interval)
}

// checkAndRecover checks IPFS health and triggers recovery if needed
func (rm *IPFSRecoveryManager) checkAndRecover() {
	// Skip if already recovering
	rm.mu.RLock()
	if rm.isRecovering {
		rm.mu.RUnlock()
		return
	}
	rm.mu.RUnlock()

	// Check if IPFS is healthy
	if rm.core.ipfsHealth != nil && rm.core.ipfsHealth.IsHealthy() {
		return
	}

	// Check if we've exceeded max recoveries
	rm.mu.RLock()
	if rm.recoveryCount >= rm.maxRecoveries {
		rm.mu.RUnlock()
		rm.log.Error("Maximum IPFS recovery attempts exceeded", "max_recoveries", rm.maxRecoveries)
		return
	}
	rm.mu.RUnlock()

	// Check if enough time has passed since last crash
	rm.mu.RLock()
	if time.Since(rm.lastCrashTime) < rm.restartDelay {
		rm.mu.RUnlock()
		return
	}
	rm.mu.RUnlock()

	// Trigger recovery
	rm.log.Warn("IPFS appears to be down, initiating recovery")
	rm.triggerRecovery()
}

// triggerRecovery initiates the IPFS recovery process
func (rm *IPFSRecoveryManager) triggerRecovery() {
	rm.mu.Lock()
	if rm.isRecovering {
		rm.mu.Unlock()
		return
	}
	rm.isRecovering = true
	rm.recoveryCount++
	rm.lastCrashTime = time.Now()
	rm.mu.Unlock()

	rm.log.Info("Starting IPFS recovery", "attempt", rm.recoveryCount, "max_attempts", rm.maxRecoveries)

	// Perform recovery in background
	go func() {
		defer func() {
			rm.mu.Lock()
			rm.isRecovering = false
			rm.mu.Unlock()
		}()

		if err := rm.performRecovery(); err != nil {
			rm.log.Error("IPFS recovery failed", "attempt", rm.recoveryCount, "error", err)
		} else {
			rm.log.Info("IPFS recovery completed successfully", "attempt", rm.recoveryCount)
		}
	}()
}

// performRecovery performs the actual IPFS recovery
func (rm *IPFSRecoveryManager) performRecovery() error {
	// Step 1: Stop current IPFS process if running
	rm.stopIPFSProcess()

	// Step 2: Wait a bit before restarting
	time.Sleep(rm.restartDelay)

	// Step 3: Restart IPFS
	if err := rm.restartIPFS(); err != nil {
		return fmt.Errorf("failed to restart IPFS: %w", err)
	}

	// Step 4: Wait for IPFS to become healthy
	if err := rm.waitForHealth(); err != nil {
		return fmt.Errorf("IPFS failed to become healthy after restart: %w", err)
	}

	// Step 5: Reinitialize IPFS shell and operations
	if err := rm.reinitializeIPFS(); err != nil {
		return fmt.Errorf("failed to reinitialize IPFS: %w", err)
	}

	// Step 6: Restore bootstrap peers
	if err := rm.restoreBootstrapPeers(); err != nil {
		rm.log.Warn("Failed to restore bootstrap peers", "error", err)
	}

	// Step 7: Reset recovery count on successful recovery
	rm.mu.Lock()
	rm.recoveryCount = 0
	rm.mu.Unlock()

	rm.log.Info("IPFS recovery completed successfully")
	return nil
}

// stopIPFSProcess stops the current IPFS process
func (rm *IPFSRecoveryManager) stopIPFSProcess() {
	rm.processMu.Lock()
	defer rm.processMu.Unlock()

	// Send stop signal via channel if available
	if rm.core.ipfsChan != nil {
		select {
		case rm.core.ipfsChan <- true:
			rm.log.Info("Sent stop signal to IPFS")
		default:
			rm.log.Warn("IPFS stop channel full")
		}
	}

	// Kill process if we have a reference
	if rm.ipfsProcess != nil && rm.ipfsProcess.Process != nil {
		rm.log.Info("Stopping IPFS process")
		rm.ipfsProcess.Process.Kill()
		rm.ipfsProcess.Wait()
		rm.ipfsProcess = nil
	}
}

// restartIPFS restarts the IPFS daemon
func (rm *IPFSRecoveryManager) restartIPFS() error {
	rm.log.Info("Restarting IPFS daemon")

	// Set environment variables
	os.Setenv("IPFS_PATH", rm.cfg.DirPath+".ipfs")
	os.Setenv("LIBP2P_FORCE_PNET", "1")

	// Create command
	ipfsApp := "./ipfs"
	if runtime.GOOS == "windows" {
		ipfsApp = "./ipfs.exe"
	}

	cmd := exec.Command(ipfsApp, "daemon", "--enable-pubsub-experiment")

	// Set up process management
	rm.processMu.Lock()
	rm.ipfsProcess = cmd
	rm.processMu.Unlock()

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start IPFS daemon: %w", err)
	}

	// Update core state
	rm.core.ipfsLock.Lock()
	rm.core.ipfsState = true
	rm.core.ipfsLock.Unlock()

	rm.log.Info("IPFS daemon started, waiting for readiness")
	
	// Wait for IPFS daemon to initialize
	time.Sleep(5 * time.Second)
	
	return nil
}

// waitForHealth waits for IPFS to become healthy
func (rm *IPFSRecoveryManager) waitForHealth() error {
	timeout := time.After(rm.healthTimeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for IPFS to become healthy")
		case <-ticker.C:
			if rm.core.ipfsHealth != nil && rm.core.ipfsHealth.IsHealthy() {
				rm.log.Info("IPFS is healthy after restart")
				return nil
			}
		}
	}
}

// reinitializeIPFS reinitializes the IPFS shell and operations
func (rm *IPFSRecoveryManager) reinitializeIPFS() error {
	rm.log.Info("Reinitializing IPFS shell and operations")

	// Create new IPFS shell
	newShell := ipfsnode.NewShell(fmt.Sprintf("localhost:%d", rm.cfg.CfgData.Ports.IPFSPort))
	if newShell == nil {
		return fmt.Errorf("failed to create new IPFS shell")
	}

	// Update core IPFS shell
	rm.core.ipfs = newShell

	// Reinitialize health manager with new shell
	if rm.core.ipfsHealth != nil {
		rm.core.ipfsHealth.Stop()
	}
	rm.core.ipfsHealth = NewIPFSHealthManager(newShell, rm.cfg, rm.core.log)
	
	// Reinitialize IPFS operations wrapper
	rm.core.ipfsOps = NewIPFSOperations(rm.core)
	
	// Reinitialize scalability manager
	if rm.core.ipfsScalability != nil {
		rm.core.ipfsScalability.Stop()
	}
	rm.core.ipfsScalability = NewIPFSScalabilityManager(rm.core)
	
	// Reinitialize connection recovery manager
	rm.core.connRecovery = NewConnectionRecovery(rm.core.log)
	
	// Reinitialize P2P reconnect manager
	rm.core.p2pReconnect = NewP2PReconnectManager(rm.core)

	// Update wallet IPFS reference if wallet exists
	if rm.core.w != nil {
		rm.core.w.SetupWallet(newShell)
		// Also update with health-managed operations
		if rm.core.ipfsOps != nil {
			rm.core.w.SetIPFSOperations(NewWalletIPFSAdapter(rm.core.ipfsOps))
		}
	}

	rm.log.Info("IPFS shell and operations reinitialized successfully")
	return nil
}

// restoreBootstrapPeers restores bootstrap peers after recovery
func (rm *IPFSRecoveryManager) restoreBootstrapPeers() error {
	rm.log.Info("Restoring bootstrap peers")

	bootstrapPeers := rm.cfg.CfgData.BootStrap
	if rm.core.testNet {
		bootstrapPeers = rm.cfg.CfgData.TestBootStrap
	}

	if len(bootstrapPeers) > 0 {
		for _, peer := range bootstrapPeers {
			_, err := rm.core.ipfs.Request("bootstrap/add", peer).Send(context.Background())
			if err != nil {
				rm.log.Warn("Failed to add bootstrap peer", "peer", peer, "error", err)
			}
		}
		rm.log.Info("Bootstrap peers restored", "count", len(bootstrapPeers))
	}

	return nil
}

// GetRecoveryStats returns recovery statistics
func (rm *IPFSRecoveryManager) GetRecoveryStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"is_recovering":   rm.isRecovering,
		"recovery_count":  rm.recoveryCount,
		"max_recoveries":  rm.maxRecoveries,
		"last_crash_time": rm.lastCrashTime,
		"restart_delay":   rm.restartDelay,
		"health_timeout":  rm.healthTimeout,
	}
}

// SetMaxRecoveries sets the maximum number of recovery attempts
func (rm *IPFSRecoveryManager) SetMaxRecoveries(max int) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.maxRecoveries = max
	rm.log.Info("Updated max recovery attempts", "max", max)
}

// SetRestartDelay sets the delay between restart attempts
func (rm *IPFSRecoveryManager) SetRestartDelay(delay time.Duration) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.restartDelay = delay
	rm.log.Info("Updated restart delay", "delay", delay)
}

// Stop stops the recovery manager
func (rm *IPFSRecoveryManager) Stop() {
	rm.recoveryCancel()
	rm.recoveryWg.Wait()
	// Don't stop IPFS process here - let the main shutdown handle it
	rm.log.Info("IPFS recovery manager stopped")
}