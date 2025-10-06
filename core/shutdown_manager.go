package core

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
	
	"github.com/rubixchain/rubixgoplatform/wrapper/logger"
)

// ShutdownManager handles graceful shutdown of all components
type ShutdownManager struct {
	log            logger.Logger
	core           *Core
	mu             sync.Mutex
	shutdownSteps  []ShutdownStep
	isShuttingDown bool
}

// ShutdownStep represents a step in the shutdown process
type ShutdownStep struct {
	Name     string
	Function func() error
	Timeout  time.Duration
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(core *Core) *ShutdownManager {
	sm := &ShutdownManager{
		log:  core.log.Named("ShutdownManager"),
		core: core,
	}
	
	// Define shutdown steps in order
	sm.shutdownSteps = []ShutdownStep{
		{
			Name: "Stop IPFS Scalability Manager",
			Function: func() error {
				if sm.core.ipfsScalability != nil {
					sm.core.ipfsScalability.Stop()
				}
				return nil
			},
			Timeout: 5 * time.Second,
		},
		{
			Name: "Stop IPFS Health Manager",
			Function: func() error {
				if sm.core.ipfsHealth != nil {
					sm.core.ipfsHealth.Stop()
				}
				return nil
			},
			Timeout: 5 * time.Second,
		},
		{
			Name: "Stop IPFS Recovery Manager",
			Function: func() error {
				if sm.core.ipfsRecovery != nil {
					sm.core.ipfsRecovery.Stop()
				}
				return nil
			},
			Timeout: 5 * time.Second,
		},
		{
			Name: "Stop IPFS Daemon",
			Function: func() error {
				return sm.stopIPFSDaemon()
			},
			Timeout: 15 * time.Second,
		},
		{
			Name: "Stop HTTP Listener",
			Function: func() error {
				if sm.core.l != nil {
					sm.core.l.Shutdown()
				}
				return nil
			},
			Timeout: 5 * time.Second,
		},
		{
			Name: "Close Database Connections",
			Function: func() error {
				if sm.core.s != nil {
					sm.core.s.Close()
				}
				// Wallet doesn't have a Close method, just log
				if sm.core.w != nil {
					sm.log.Debug("Wallet cleanup completed")
				}
				return nil
			},
			Timeout: 5 * time.Second,
		},
		{
			Name: "Close Performance Tracker",
			Function: func() error {
				if sm.core.perfTracker != nil {
					return sm.core.perfTracker.Close()
				}
				return nil
			},
			Timeout: 2 * time.Second,
		},
	}
	
	return sm
}

// Shutdown performs graceful shutdown
func (sm *ShutdownManager) Shutdown() error {
	sm.mu.Lock()
	if sm.isShuttingDown {
		sm.mu.Unlock()
		return fmt.Errorf("shutdown already in progress")
	}
	sm.isShuttingDown = true
	sm.mu.Unlock()
	
	sm.log.Info("Starting graceful shutdown")
	
	var lastErr error
	for _, step := range sm.shutdownSteps {
		sm.log.Info("Executing shutdown step", "step", step.Name)
		
		ctx, cancel := context.WithTimeout(context.Background(), step.Timeout)
		defer cancel()
		
		done := make(chan error, 1)
		go func() {
			done <- step.Function()
		}()
		
		select {
		case err := <-done:
			if err != nil {
				sm.log.Error("Shutdown step failed", "step", step.Name, "error", err)
				lastErr = err
			} else {
				sm.log.Info("Shutdown step completed", "step", step.Name)
			}
		case <-ctx.Done():
			sm.log.Error("Shutdown step timed out", "step", step.Name, "timeout", step.Timeout)
			lastErr = fmt.Errorf("step %s timed out", step.Name)
		}
	}
	
	sm.log.Info("Graceful shutdown completed", "hadErrors", lastErr != nil)
	return lastErr
}

// stopIPFSDaemon stops the IPFS daemon with proper process management
func (sm *ShutdownManager) stopIPFSDaemon() error {
	if !sm.core.GetIPFSState() {
		sm.log.Debug("IPFS daemon not running, skip stop")
		return nil
	}
	
	sm.log.Info("Stopping IPFS daemon")
	
	// First try graceful shutdown via channel
	select {
	case sm.core.ipfsChan <- true:
		sm.log.Debug("Sent shutdown signal to IPFS")
	default:
		sm.log.Warn("IPFS channel blocked, trying alternative methods")
	}
	
	// Wait for graceful shutdown
	gracefulTimeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-gracefulTimeout:
			// Graceful shutdown failed, try forceful methods
			sm.log.Warn("Graceful IPFS shutdown timed out, attempting forceful shutdown")
			return sm.forceStopIPFS()
		case <-ticker.C:
			if !sm.core.GetIPFSState() {
				sm.log.Info("IPFS daemon stopped gracefully")
				return nil
			}
		}
	}
}

// forceStopIPFS forcefully stops IPFS using OS-specific methods
func (sm *ShutdownManager) forceStopIPFS() error {
	sm.log.Warn("Attempting to force stop IPFS daemon")
	
	// First try to use the stored command/PID if available
	if sm.core.ipfsCmd != nil && sm.core.ipfsCmd.Process != nil {
		sm.log.Info("Using stored process reference to kill IPFS", "pid", sm.core.ipfsPID)
		if err := sm.core.ipfsCmd.Process.Kill(); err != nil {
			sm.log.Error("Failed to kill IPFS using process reference", "err", err, "pid", sm.core.ipfsPID)
		} else {
			sm.log.Info("Successfully killed IPFS using process reference", "pid", sm.core.ipfsPID)
			sm.core.SetIPFSState(false)
			return nil
		}
	}
	
	// If we have a PID, try to kill that specific process
	if sm.core.ipfsPID > 0 {
		sm.log.Info("Attempting to kill IPFS by PID", "pid", sm.core.ipfsPID)
		if err := sm.killProcessByPID(sm.core.ipfsPID); err != nil {
			sm.log.Error("Failed to kill IPFS by PID", "err", err, "pid", sm.core.ipfsPID)
		} else {
			sm.log.Info("Successfully killed IPFS by PID", "pid", sm.core.ipfsPID)
			sm.core.SetIPFSState(false)
			return nil
		}
	}
	
	// Last resort: find and kill by process name (but be careful)
	sm.log.Warn("No PID available, attempting to find IPFS process")
	switch runtime.GOOS {
	case "windows":
		return sm.killIPFSWindows()
	case "linux", "darwin":
		return sm.killIPFSUnix()
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// killProcessByPID kills a specific process by PID
func (sm *ShutdownManager) killProcessByPID(pid int) error {
	if runtime.GOOS == "windows" {
		// Windows: taskkill /F /PID <pid>
		cmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("taskkill failed: %v, output: %s", err, string(output))
		}
	} else {
		// Unix: kill -9 <pid>
		cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("kill failed: %v", err)
		}
	}
	return nil
}

// killIPFSWindows kills IPFS on Windows (last resort, be careful)
func (sm *ShutdownManager) killIPFSWindows() error {
	sm.log.Warn("Using generic IPFS kill method - this may affect other IPFS instances!")
	
	// Note: On Windows, we can't easily filter by repo path
	// This may affect other IPFS instances - use stored PID when possible
	
	// First try taskkill with /T flag to kill process tree
	cmd := exec.Command("taskkill", "/F", "/T", "/IM", "ipfs.exe")
	output, err := cmd.CombinedOutput()
	if err != nil {
		sm.log.Error("Failed to kill IPFS with taskkill", "error", err, "output", string(output))
		
		// Try PowerShell as fallback
		psCmd := `Get-Process ipfs -ErrorAction SilentlyContinue | Stop-Process -Force`
		cmd = exec.Command("powershell", "-Command", psCmd)
		output, err = cmd.CombinedOutput()
		if err != nil {
			sm.log.Error("Failed to kill IPFS with PowerShell", "error", err, "output", string(output))
			return err
		}
	}
	
	sm.log.Info("IPFS process killed on Windows")
	sm.core.SetIPFSState(false)
	return nil
}

// killIPFSUnix kills IPFS on Unix-like systems (last resort)
func (sm *ShutdownManager) killIPFSUnix() error {
	sm.log.Warn("Using generic IPFS kill method - this may affect other IPFS instances!")
	
	// Try to be more specific by including the repo path in the search
	ipfsRepo := sm.core.cfg.DirPath + ".ipfs"
	
	// First try pkill with more specific pattern
	cmd := exec.Command("pkill", "-f", fmt.Sprintf("ipfs.*--repo=%s.*daemon", ipfsRepo))
	if err := cmd.Run(); err != nil {
		sm.log.Debug("Specific pkill failed, trying general pattern", "error", err)
		
		// Try general pattern
		cmd = exec.Command("pkill", "-f", "ipfs daemon")
		if err := cmd.Run(); err != nil {
			sm.log.Debug("pkill failed, trying killall", "error", err)
		
		// Try killall as fallback
		cmd = exec.Command("killall", "-9", "ipfs")
		if err := cmd.Run(); err != nil {
			sm.log.Debug("killall failed, trying pgrep/kill", "error", err)
			
			// Last resort: find PID and kill directly
			// Try to find with repo path first
			cmd = exec.Command("pgrep", "-f", fmt.Sprintf("ipfs.*--repo=%s.*daemon", ipfsRepo))
			output, err := cmd.Output()
			if err != nil || len(output) == 0 {
				// Fallback to general search
				cmd = exec.Command("pgrep", "-f", "ipfs daemon")
				output, err = cmd.Output()
			}
			if err != nil {
				sm.log.Error("Failed to find IPFS process", "error", err)
				return err
			}
			
			pids := string(output)
			if pids == "" {
				sm.log.Info("No IPFS process found")
				sm.core.SetIPFSState(false)
				return nil
			}
			
			// Kill each PID
			for _, pid := range splitLines(pids) {
				if pid == "" {
					continue
				}
				killCmd := exec.Command("kill", "-9", pid)
				if err := killCmd.Run(); err != nil {
					sm.log.Error("Failed to kill PID", "pid", pid, "error", err)
				} else {
					sm.log.Info("Killed IPFS process", "pid", pid)
				}
			}
		}
	}
	}
	
	sm.log.Info("IPFS process killed on Unix")
	sm.core.SetIPFSState(false)
	return nil
}

// splitLines splits a string by newlines and returns non-empty lines
func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// FindIPFSProcess checks if THIS node's IPFS process is running
func (sm *ShutdownManager) FindIPFSProcess() (bool, int) {
	// First check if we have a stored PID
	if sm.core.ipfsPID > 0 {
		// Verify the process is still running
		if sm.isProcessRunning(sm.core.ipfsPID) {
			return true, sm.core.ipfsPID
		}
		// PID is stale, clear it
		sm.core.ipfsPID = 0
	}
	
	// If no PID stored, try to find by repo path
	ipfsRepo := sm.core.cfg.DirPath + ".ipfs"
	
	switch runtime.GOOS {
	case "windows":
		// On Windows, we can't easily filter by working directory
		// Just check if we have a stored PID
		if sm.core.ipfsPID > 0 {
			cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", sm.core.ipfsPID))
			output, err := cmd.Output()
			if err == nil && strings.Contains(string(output), "ipfs.exe") {
				return true, sm.core.ipfsPID
			}
		}
		return false, 0
		
	case "linux", "darwin":
		// Try to find with specific repo path
		cmd := exec.Command("pgrep", "-f", fmt.Sprintf("ipfs.*--repo=%s.*daemon", ipfsRepo))
		output, err := cmd.Output()
		if err != nil || len(output) == 0 {
			// Try alternative pattern (IPFS_PATH environment variable)
			cmd = exec.Command("pgrep", "-f", fmt.Sprintf("IPFS_PATH=%s.*ipfs daemon", ipfsRepo))
			output, err = cmd.Output()
			if err != nil {
				return false, 0
			}
		}
		
		pidStr := strings.TrimSpace(string(output))
		if pidStr == "" {
			return false, 0
		}
		// Return first PID
		pids := splitLines(pidStr)
		if len(pids) > 0 {
			var pid int
			fmt.Sscanf(pids[0], "%d", &pid)
			return true, pid
		}
		return false, 0
		
	default:
		return false, 0
	}
}

// isProcessRunning checks if a specific PID is still running
func (sm *ShutdownManager) isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
		output, err := cmd.Output()
		return err == nil && strings.Contains(string(output), fmt.Sprintf("%d", pid))
		
	case "linux", "darwin":
		// kill -0 checks if process exists without actually sending a signal
		cmd := exec.Command("kill", "-0", fmt.Sprintf("%d", pid))
		return cmd.Run() == nil
		
	default:
		return false
	}
}