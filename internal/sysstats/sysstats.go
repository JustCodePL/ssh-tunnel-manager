// Package sysstats parses remote-host resource data (CPU, memory, disk,
// docker, host capabilities) gathered by running standard commands over SSH.
// All functions here are pure string parsers so they can be unit-tested
// without a live connection.
package sysstats

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// CPUSample is a single reading of cumulative CPU time from /proc/stat.
// Idle and Total are jiffies; CPU usage is derived from the delta between two
// samples.
type CPUSample struct {
	Idle  uint64 `json:"idle"`
	Total uint64 `json:"total"`
}

// Valid reports whether the sample holds usable data.
func (s CPUSample) Valid() bool { return s.Total > 0 }

// ServerStats is the at-a-glance resource snapshot shown in the inline widget.
type ServerStats struct {
	CPUPercent float64 `json:"cpuPercent"`
	MemTotal   uint64  `json:"memTotal"` // bytes
	MemUsed    uint64  `json:"memUsed"`  // bytes
	// HasCPU is false when a previous CPU sample was unavailable, so the
	// frontend can show "--" instead of a misleading 0%.
	HasCPU bool `json:"hasCPU"`
}

// DiskMount is one filesystem row from df.
type DiskMount struct {
	Filesystem string  `json:"filesystem"`
	Total      uint64  `json:"total"` // bytes
	Used       uint64  `json:"used"`  // bytes
	Avail      uint64  `json:"avail"` // bytes
	UsePercent float64 `json:"usePercent"`
	MountPoint string  `json:"mountPoint"`
}

// DockerContainer is one row from `docker ps`.
type DockerContainer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
	Ports  string `json:"ports"`
}

// Capabilities reports which optional tools are available on the remote host.
type Capabilities struct {
	Docker bool   `json:"docker"`
	Htop   bool   `json:"htop"`
	OS     string `json:"os"` // uname -s, e.g. "Linux", "Darwin"
}

// ParseCPUSample extracts the aggregate CPU sample from /proc/stat. It reads
// the first line beginning with "cpu " (the all-cores aggregate). The returned
// sample is zero-valued (and !Valid) if no such line is found.
func ParseCPUSample(procStat string) CPUSample {
	for _, line := range strings.Split(procStat, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var total, idle uint64
		for i, f := range fields[1:] {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				continue
			}
			total += v
			// Field index 3 is idle, 4 is iowait (both counted as "not busy").
			if i == 3 || i == 4 {
				idle += v
			}
		}
		return CPUSample{Idle: idle, Total: total}
	}
	return CPUSample{}
}

// CPUPercent computes busy CPU percentage from two samples. It is independent
// of the sampling interval. Returns 0 if the samples don't form a positive
// delta (e.g. first poll, or counters reset).
func CPUPercent(prev, cur CPUSample) float64 {
	if !prev.Valid() || !cur.Valid() {
		return 0
	}
	totalDelta := float64(cur.Total) - float64(prev.Total)
	idleDelta := float64(cur.Idle) - float64(prev.Idle)
	if totalDelta <= 0 {
		return 0
	}
	pct := (totalDelta - idleDelta) / totalDelta * 100
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// ParseMeminfo extracts MemTotal and MemAvailable (in kB) from /proc/meminfo.
func ParseMeminfo(s string) (totalKB, availKB uint64) {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	return totalKB, availKB
}

// ParseDf parses the output of `df -P -B1` (POSIX format, sizes in bytes).
// The header line and pseudo filesystems (tmpfs, devtmpfs, overlay, etc.) are
// skipped so the list reflects real storage.
func ParseDf(s string) []DiskMount {
	var mounts []DiskMount
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header: Filesystem 1024-blocks Used Available Capacity Mounted on
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		fs := fields[0]
		if isPseudoFS(fs) {
			continue
		}
		total, _ := strconv.ParseUint(fields[1], 10, 64)
		used, _ := strconv.ParseUint(fields[2], 10, 64)
		avail, _ := strconv.ParseUint(fields[3], 10, 64)
		// fields[4] is "NN%"; recompute from bytes for accuracy when possible.
		pct := 0.0
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		} else {
			pct = parsePercent(fields[4])
		}
		// The mount point may contain spaces; rejoin the tail.
		mountPoint := strings.Join(fields[5:], " ")
		mounts = append(mounts, DiskMount{
			Filesystem: fs,
			Total:      total,
			Used:       used,
			Avail:      avail,
			UsePercent: pct,
			MountPoint: mountPoint,
		})
	}
	return mounts
}

func isPseudoFS(fs string) bool {
	switch fs {
	case "tmpfs", "devtmpfs", "overlay", "shm", "udev", "none", "cgroup", "cgroup2", "squashfs":
		return true
	}
	// Kernel/virtual filesystems exposed by df on some systems.
	if strings.HasPrefix(fs, "/dev/loop") {
		return true
	}
	return false
}

func parsePercent(s string) float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// dockerPSLine mirrors the JSON emitted by `docker ps --format '{{json .}}'`.
type dockerPSLine struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	Status string `json:"Status"`
	State  string `json:"State"`
	Ports  string `json:"Ports"`
}

// ParseDockerPs parses the newline-delimited JSON output of
// `docker ps -a --format '{{json .}}'`. Lines that don't parse are skipped.
func ParseDockerPs(s string) []DockerContainer {
	var out []DockerContainer
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var d dockerPSLine
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			continue
		}
		out = append(out, DockerContainer{
			ID:     d.ID,
			Name:   d.Names,
			Image:  d.Image,
			Status: d.Status,
			State:  d.State,
			Ports:  d.Ports,
		})
	}
	return out
}

// CPUCore is one CPU's busy percentage. Name is "cpu" for the aggregate or
// "cpu0", "cpu1", ... for individual cores.
type CPUCore struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

// ProcessInfo is one row from `ps`, used by the graphical monitor.
type ProcessInfo struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	Command string  `json:"command"`
}

// ProcessStats is the htop-like snapshot: per-core CPU, memory/swap, load,
// uptime and the top processes. Memory/swap fields are in bytes.
type ProcessStats struct {
	Cores         []CPUCore     `json:"cores"`
	MemTotal      uint64        `json:"memTotal"`
	MemUsed       uint64        `json:"memUsed"`
	SwapTotal     uint64        `json:"swapTotal"`
	SwapUsed      uint64        `json:"swapUsed"`
	Load1         float64       `json:"load1"`
	Load5         float64       `json:"load5"`
	Load15        float64       `json:"load15"`
	UptimeSeconds float64       `json:"uptimeSeconds"`
	Processes     []ProcessInfo `json:"processes"`
}

// ParseAllCPUSamples parses every "cpu"/"cpuN" line of /proc/stat into samples
// keyed by name.
func ParseAllCPUSamples(procStat string) map[string]CPUSample {
	out := map[string]CPUSample{}
	for _, line := range strings.Split(procStat, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		var total, idle uint64
		for i, f := range fields[1:] {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				continue
			}
			total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		out[fields[0]] = CPUSample{Idle: idle, Total: total}
	}
	return out
}

// ParseMeminfoFull extracts MemTotal, MemAvailable, SwapTotal and SwapFree
// (all in kB) from /proc/meminfo.
func ParseMeminfoFull(s string) (memTotalKB, memAvailKB, swapTotalKB, swapFreeKB uint64) {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotalKB = v
		case "MemAvailable:":
			memAvailKB = v
		case "SwapTotal:":
			swapTotalKB = v
		case "SwapFree:":
			swapFreeKB = v
		}
	}
	return memTotalKB, memAvailKB, swapTotalKB, swapFreeKB
}

// ParseLoadAvg reads the three load averages from /proc/loadavg.
func ParseLoadAvg(s string) (l1, l5, l15 float64) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return 0, 0, 0
	}
	l1, _ = strconv.ParseFloat(f[0], 64)
	l5, _ = strconv.ParseFloat(f[1], 64)
	l15, _ = strconv.ParseFloat(f[2], 64)
	return l1, l5, l15
}

// ParseUptime reads the uptime (seconds) from /proc/uptime.
func ParseUptime(s string) float64 {
	f := strings.Fields(s)
	if len(f) < 1 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

// ParsePs parses `ps -eo pid,user,pcpu,pmem,comm` output (header + rows).
func ParsePs(s string) []ProcessInfo {
	var out []ProcessInfo
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // header / blanks
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		cpu, _ := strconv.ParseFloat(f[2], 64)
		mem, _ := strconv.ParseFloat(f[3], 64)
		out = append(out, ProcessInfo{
			PID:     pid,
			User:    f[1],
			CPU:     cpu,
			Mem:     mem,
			Command: strings.Join(f[4:], " "),
		})
	}
	return out
}

// coreIndex returns the numeric suffix of a "cpuN" name (e.g. "cpu3" -> 3) for
// stable ordering. The aggregate "cpu" sorts first via -1.
func coreIndex(name string) int {
	n := strings.TrimPrefix(name, "cpu")
	if n == "" {
		return -1
	}
	v, err := strconv.Atoi(n)
	if err != nil {
		return 1 << 30
	}
	return v
}

// BuildProcessStats assembles a ProcessStats from the raw command segments:
// two /proc/stat reads (for per-core CPU deltas), /proc/meminfo, /proc/loadavg,
// /proc/uptime and ps output.
func BuildProcessStats(stat1, stat2, meminfo, loadavg, uptime, ps string) ProcessStats {
	s1 := ParseAllCPUSamples(stat1)
	s2 := ParseAllCPUSamples(stat2)

	var cores []CPUCore
	if a2, ok := s2["cpu"]; ok {
		cores = append(cores, CPUCore{Name: "cpu", Percent: CPUPercent(s1["cpu"], a2)})
	}
	var names []string
	for name := range s2 {
		if name != "cpu" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool { return coreIndex(names[i]) < coreIndex(names[j]) })
	for _, name := range names {
		cores = append(cores, CPUCore{Name: name, Percent: CPUPercent(s1[name], s2[name])})
	}

	memT, memA, swT, swF := ParseMeminfoFull(meminfo)
	l1, l5, l15 := ParseLoadAvg(loadavg)

	procs := ParsePs(ps)
	if procs == nil {
		procs = []ProcessInfo{}
	}
	return ProcessStats{
		Cores:         cores,
		MemTotal:      memT * 1024,
		MemUsed:       (memT - memA) * 1024,
		SwapTotal:     swT * 1024,
		SwapUsed:      (swT - swF) * 1024,
		Load1:         l1,
		Load5:         l5,
		Load15:        l15,
		UptimeSeconds: ParseUptime(uptime),
		Processes:     procs,
	}
}

// ParseCapabilities reads the output of the detection command, which echoes
// "docker" and/or "htop" when present and the `uname -s` value on its own line.
func ParseCapabilities(s string) Capabilities {
	var c Capabilities
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "":
			continue
		case "docker":
			c.Docker = true
		case "htop":
			c.Htop = true
		default:
			// Last non-marker line is the uname output.
			c.OS = line
		}
	}
	return c
}
