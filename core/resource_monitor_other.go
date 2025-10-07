// +build !linux

package core

// getLinuxMemoryInfo is a stub for non-Linux platforms
func getLinuxMemoryInfo() (totalMB, availableMB uint64) {
	// Return 0, 0 to indicate this platform doesn't support /proc/meminfo
	return 0, 0
}