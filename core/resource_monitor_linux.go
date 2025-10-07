// +build linux

package core

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// getLinuxMemoryInfo reads memory info from /proc/meminfo
func getLinuxMemoryInfo() (totalMB, availableMB uint64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var memTotal, memAvailable uint64

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch fields[0] {
		case "MemTotal:":
			if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				memTotal = val / 1024 // Convert KB to MB
			}
		case "MemAvailable:":
			if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				memAvailable = val / 1024 // Convert KB to MB
			}
		}

		// If we have both values, we can return
		if memTotal > 0 && memAvailable > 0 {
			return memTotal, memAvailable
		}
	}

	// If MemAvailable is not found (older kernels), estimate it
	if memTotal > 0 && memAvailable == 0 {
		// Rough estimate: 50% of total memory is available
		memAvailable = memTotal / 2
	}

	return memTotal, memAvailable
}