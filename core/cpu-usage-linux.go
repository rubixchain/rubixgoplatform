package core

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// CPU usage calculation using /proc/stat (Linux-specific)
func (c *Core) getCPUUsageLinux(lastStats map[string]uint64) (float64, map[string]uint64) {
	if runtime.GOOS != "linux" {
		// For non-Linux systems, return moderate CPU usage estimate
		return 50.0, nil
	}

	file, err := os.Open("/proc/stat")
	if err != nil {
		c.log.Error("Failed to read /proc/stat", "error", err)
		return 50.0, lastStats // Return default if can't read
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				break
			}

			// Parse CPU stats: user, nice, system, idle, iowait, irq, softirq
			currentStats := make(map[string]uint64)
			statNames := []string{"user", "nice", "system", "idle", "iowait", "irq", "softirq"}

			var totalCurrent uint64
			for i, name := range statNames {
				if val, err := strconv.ParseUint(fields[i+1], 10, 64); err == nil {
					currentStats[name] = val
					totalCurrent += val
				}
			}
			currentStats["total"] = totalCurrent

			// Calculate CPU usage if we have previous stats
			if lastStats != nil && lastStats["total"] > 0 {
				totalDiff := currentStats["total"] - lastStats["total"]
				idleDiff := currentStats["idle"] - lastStats["idle"]

				if totalDiff > 0 {
					cpuUsage := float64(totalDiff-idleDiff) / float64(totalDiff) * 100.0
					return cpuUsage, currentStats
				}
			}

			return 0.0, currentStats // First measurement, return 0%
		}
	}

	return 50.0, lastStats // Fallback
}
