package sysstats

import "testing"

func TestParseCPUSample(t *testing.T) {
	const procStat = `cpu  100 0 50 800 50 0 0 0 0 0
cpu0 50 0 25 400 25 0 0 0 0 0
intr 12345
`
	s := ParseCPUSample(procStat)
	if !s.Valid() {
		t.Fatalf("expected valid sample, got %+v", s)
	}
	// total = 100+0+50+800+50 = 1000; idle = 800 (idle) + 50 (iowait) = 850
	if s.Total != 1000 {
		t.Errorf("Total = %d, want 1000", s.Total)
	}
	if s.Idle != 850 {
		t.Errorf("Idle = %d, want 850", s.Idle)
	}
}

func TestParseCPUSampleMissing(t *testing.T) {
	s := ParseCPUSample("intr 1\nctxt 2\n")
	if s.Valid() {
		t.Errorf("expected invalid sample, got %+v", s)
	}
}

func TestCPUPercent(t *testing.T) {
	prev := CPUSample{Idle: 850, Total: 1000}
	cur := CPUSample{Idle: 900, Total: 1100} // total delta 100, idle delta 50 -> 50% busy
	got := CPUPercent(prev, cur)
	if got != 50 {
		t.Errorf("CPUPercent = %v, want 50", got)
	}
}

func TestCPUPercentFirstSample(t *testing.T) {
	if got := CPUPercent(CPUSample{}, CPUSample{Idle: 1, Total: 2}); got != 0 {
		t.Errorf("CPUPercent with invalid prev = %v, want 0", got)
	}
}

func TestCPUPercentClamp(t *testing.T) {
	// Counter reset: cur < prev -> negative delta -> 0.
	if got := CPUPercent(CPUSample{Idle: 900, Total: 2000}, CPUSample{Idle: 950, Total: 1000}); got != 0 {
		t.Errorf("CPUPercent on reset = %v, want 0", got)
	}
}

func TestParseMeminfo(t *testing.T) {
	const meminfo = `MemTotal:        8000000 kB
MemFree:          500000 kB
MemAvailable:    4000000 kB
Buffers:          100000 kB
`
	total, avail := ParseMeminfo(meminfo)
	if total != 8000000 {
		t.Errorf("total = %d, want 8000000", total)
	}
	if avail != 4000000 {
		t.Errorf("avail = %d, want 4000000", avail)
	}
}

func TestParseDf(t *testing.T) {
	const df = `Filesystem     1B-blocks       Used  Available Capacity Mounted on
/dev/sda1    100000000000 62000000000 38000000000      62% /
tmpfs          4000000000           0  4000000000       0% /dev/shm
/dev/sdb1    500000000000 10000000000 490000000000       2% /mnt/data drive
`
	mounts := ParseDf(df)
	if len(mounts) != 2 {
		t.Fatalf("got %d mounts, want 2 (tmpfs filtered): %+v", len(mounts), mounts)
	}
	root := mounts[0]
	if root.MountPoint != "/" {
		t.Errorf("mount0 = %q, want /", root.MountPoint)
	}
	if root.Total != 100000000000 || root.Used != 62000000000 {
		t.Errorf("root sizes wrong: %+v", root)
	}
	if root.UsePercent < 61.9 || root.UsePercent > 62.1 {
		t.Errorf("root use%% = %v, want ~62", root.UsePercent)
	}
	if mounts[1].MountPoint != "/mnt/data drive" {
		t.Errorf("mount1 with space = %q, want '/mnt/data drive'", mounts[1].MountPoint)
	}
}

func TestParseDockerPs(t *testing.T) {
	const out = `{"ID":"abc123","Names":"web","Image":"nginx:latest","Status":"Up 2 hours","State":"running","Ports":"0.0.0.0:80->80/tcp"}
{"ID":"def456","Names":"db","Image":"postgres:16","Status":"Exited (0) 5 minutes ago","State":"exited","Ports":""}
garbage line
`
	cs := ParseDockerPs(out)
	if len(cs) != 2 {
		t.Fatalf("got %d containers, want 2: %+v", len(cs), cs)
	}
	if cs[0].Name != "web" || cs[0].Image != "nginx:latest" || cs[0].State != "running" {
		t.Errorf("container0 wrong: %+v", cs[0])
	}
	if cs[1].Name != "db" || cs[1].State != "exited" {
		t.Errorf("container1 wrong: %+v", cs[1])
	}
}

func TestBuildProcessStats(t *testing.T) {
	const stat1 = `cpu  100 0 50 800 50 0 0 0 0 0
cpu0 50 0 25 400 25 0 0 0 0 0
cpu1 50 0 25 400 25 0 0 0 0 0
`
	const stat2 = `cpu  200 0 100 850 50 0 0 0 0 0
cpu0 120 0 55 410 25 0 0 0 0 0
cpu1 80 0 45 440 25 0 0 0 0 0
`
	const meminfo = `MemTotal:        8000000 kB
MemAvailable:    4000000 kB
SwapTotal:       2000000 kB
SwapFree:        1500000 kB
`
	const loadavg = `0.50 0.75 1.00 1/234 5678`
	const uptime = `123456.78 654321.00`
	const ps = `    PID USER                                 %CPU %MEM COMMAND
   1234 root                                 12.5  3.4 nginx
   5678 www-data                              2.0  1.1 php-fpm
`

	st := BuildProcessStats(stat1, stat2, meminfo, loadavg, uptime, ps)

	if len(st.Cores) != 3 {
		t.Fatalf("got %d cores, want 3 (cpu + cpu0 + cpu1): %+v", len(st.Cores), st.Cores)
	}
	if st.Cores[0].Name != "cpu" {
		t.Errorf("first core = %q, want aggregate 'cpu'", st.Cores[0].Name)
	}
	if st.Cores[1].Name != "cpu0" || st.Cores[2].Name != "cpu1" {
		t.Errorf("core ordering wrong: %+v", st.Cores)
	}
	// aggregate: total delta = (1200-1000)=200, idle delta = (900-850)=50 -> 75%
	if st.Cores[0].Percent != 75 {
		t.Errorf("aggregate cpu%% = %v, want 75", st.Cores[0].Percent)
	}
	if st.MemTotal != 8000000*1024 || st.MemUsed != 4000000*1024 {
		t.Errorf("mem wrong: total=%d used=%d", st.MemTotal, st.MemUsed)
	}
	if st.SwapTotal != 2000000*1024 || st.SwapUsed != 500000*1024 {
		t.Errorf("swap wrong: total=%d used=%d", st.SwapTotal, st.SwapUsed)
	}
	if st.Load1 != 0.5 || st.Load5 != 0.75 || st.Load15 != 1.0 {
		t.Errorf("load wrong: %v %v %v", st.Load1, st.Load5, st.Load15)
	}
	if st.UptimeSeconds < 123456 || st.UptimeSeconds > 123457 {
		t.Errorf("uptime = %v, want ~123456.78", st.UptimeSeconds)
	}
	if len(st.Processes) != 2 {
		t.Fatalf("got %d processes, want 2: %+v", len(st.Processes), st.Processes)
	}
	p0 := st.Processes[0]
	if p0.PID != 1234 || p0.User != "root" || p0.CPU != 12.5 || p0.Mem != 3.4 || p0.Command != "nginx" {
		t.Errorf("process0 wrong: %+v", p0)
	}
}

func TestParseCapabilities(t *testing.T) {
	c := ParseCapabilities("docker\nhtop\nLinux\n")
	if !c.Docker || !c.Htop || c.OS != "Linux" {
		t.Errorf("caps = %+v, want all set with OS=Linux", c)
	}

	c2 := ParseCapabilities("Linux\n")
	if c2.Docker || c2.Htop || c2.OS != "Linux" {
		t.Errorf("caps2 = %+v, want only OS=Linux", c2)
	}
}
