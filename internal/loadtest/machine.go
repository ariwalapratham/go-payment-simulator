package loadtest

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

const bytesPerKiB = 1024

// Machine is the host the load generator ran on.
type Machine struct {
	OS         string
	Arch       string
	CPUs       int
	MemTotalKB int64
}

// ReadMachine records OS, CPU count, and MemTotal from /proc/meminfo when present.
func ReadMachine() Machine {
	m := Machine{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(),
	}
	m.MemTotalKB = memTotalKB()
	return m
}

func (m Machine) String() string {
	s := fmt.Sprintf("%s/%s cpus=%d", m.OS, m.Arch, m.CPUs)
	if m.MemTotalKB > 0 {
		s += fmt.Sprintf(" mem=%.1fGiB", float64(m.MemTotalKB)/float64(bytesPerKiB*bytesPerKiB))
	}
	return s
}

func memTotalKB() int64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 { //nolint:mnd // MemTotal line is "MemTotal: <kb> kB"
			return 0
		}
		n, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			return 0
		}
		return n
	}
	return 0
}
