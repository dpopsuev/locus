package resource

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func selfRSSMB() float64 {
	return readProcRSSMB("/proc/self/status")
}

func childRSSMB(pids []int) []ChildProcess {
	procs := make([]ChildProcess, 0, len(pids))
	for _, pid := range pids {
		rss := readProcRSSMB(fmt.Sprintf("/proc/%d/status", pid))
		if rss <= 0 {
			continue
		}
		cmd := readProcComm(pid)
		procs = append(procs, ChildProcess{PID: pid, Command: cmd, RSSMB: rss})
	}
	return procs
}

func readProcRSSMB(path string) float64 {
	f, err := os.Open(path) //nolint:gosec // path is from /proc, not user input
	if err != nil {
		return 0
	}
	defer f.Close() //nolint:errcheck // read-only file

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			return parseKBLine(line)
		}
	}
	_ = sc.Err()
	return 0
}

func parseKBLine(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	kb, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0
	}
	return kb / 1024
}

func readProcComm(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}
