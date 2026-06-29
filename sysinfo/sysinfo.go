package sysinfo

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type GPUInfo struct {
	Name   string  `json:"name"`
	Pct    float64 `json:"pct"`
	VRAMMB int64   `json:"vram_mb"`
}

type Stats struct {
	RSSMB  int64    `json:"rss_mb"`
	CPUPct float64  `json:"cpu_pct"`
	GPU    *GPUInfo `json:"gpu,omitempty"`
}

var cpuCache struct {
	sync.Mutex
	pct float64
	at  time.Time
}

var gpuCache struct {
	sync.Mutex
	info *GPUInfo
	at   time.Time
}

var tempCache struct {
	sync.Mutex
	temps map[string]float64
	at    time.Time
}

var netSpeedCache struct {
	sync.Mutex
	rxPS int64
	txPS int64
	at   time.Time
}

func Collect() Stats {
	return Stats{
		RSSMB:  ProcessRSSMB(),
		CPUPct: cachedCPUPct(),
		GPU:    cachedGPU(),
	}
}

func ProcessRSSMB() int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseInt(fields[1], 10, 64)
				return kb / 1024
			}
		}
	}
	return 0
}

func CPUThreads() int {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	count := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "processor") {
			count++
		}
	}
	return count
}

func CPUTempCelsius() map[string]float64 {
	tempCache.Lock()
	defer tempCache.Unlock()
	if time.Since(tempCache.at) < 5*time.Second {
		return tempCache.temps
	}
	temps := map[string]float64{}
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, zone := range zones {
		data, err := os.ReadFile(zone)
		if err != nil {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			continue
		}
		name := filepath.Base(filepath.Dir(zone))
		temps[name] = float64(v) / 1000.0
	}
	tempCache.temps = temps
	tempCache.at = time.Now()
	return temps
}

func NetworkSpeed() (rxPS, txPS int64) {
	netSpeedCache.Lock()
	defer netSpeedCache.Unlock()
	if time.Since(netSpeedCache.at) < 5*time.Second {
		return netSpeedCache.rxPS, netSpeedCache.txPS
	}
	rx1, tx1 := readNetBytes()
	time.Sleep(200 * time.Millisecond)
	rx2, tx2 := readNetBytes()
	netSpeedCache.rxPS = (rx2 - rx1) * 5
	netSpeedCache.txPS = (tx2 - tx1) * 5
	netSpeedCache.at = time.Now()
	return netSpeedCache.rxPS, netSpeedCache.txPS
}

func DiskStats(path string) (used, free int64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	avail := int64(stat.Bavail) * int64(stat.Bsize)
	return total - avail, avail
}

func cachedCPUPct() float64 {
	cpuCache.Lock()
	defer cpuCache.Unlock()
	if time.Since(cpuCache.at) < time.Second {
		return cpuCache.pct
	}
	cpuCache.pct = measureCPUPct(150 * time.Millisecond)
	cpuCache.at = time.Now()
	return cpuCache.pct
}

func cachedGPU() *GPUInfo {
	gpuCache.Lock()
	defer gpuCache.Unlock()
	if time.Since(gpuCache.at) < 2*time.Second {
		return gpuCache.info
	}
	gpuCache.info = queryGPU()
	gpuCache.at = time.Now()
	return gpuCache.info
}

func measureCPUPct(d time.Duration) float64 {
	const clkTck = 100.0
	u1, s1 := procCPUTicks()
	t1 := time.Now()
	time.Sleep(d)
	u2, s2 := procCPUTicks()
	elapsed := time.Since(t1).Seconds()
	if elapsed <= 0 {
		return 0
	}
	delta := float64((u2 + s2) - (u1 + s1))
	pct := delta / clkTck / elapsed * 100
	if pct < 0 {
		return 0
	}
	return pct
}

func procCPUTicks() (utime, stime int64) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 {
		return
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 13 {
		return
	}
	utime, _ = strconv.ParseInt(fields[11], 10, 64)
	stime, _ = strconv.ParseInt(fields[12], 10, 64)
	return
}

func readNetBytes() (rx, tx int64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		fields := strings.Fields(strings.SplitN(line, ":", 2)[1])
		if len(fields) >= 9 {
			r, _ := strconv.ParseInt(fields[0], 10, 64)
			t, _ := strconv.ParseInt(fields[8], 10, 64)
			rx += r
			tx += t
		}
	}
	return
}

func queryGPU() *GPUInfo {
	if info := queryNvidia(); info != nil {
		return info
	}
	return queryROCm()
}

func queryNvidia() *GPUInfo {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return nil
	}
	line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
	parts := strings.Split(line, ", ")
	if len(parts) < 3 {
		return nil
	}
	name := strings.TrimSpace(parts[0])
	pct, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	vram, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	return &GPUInfo{Name: name, Pct: pct, VRAMMB: vram}
}

func queryROCm() *GPUInfo {
	out, err := exec.Command("rocm-smi",
		"--showproductname",
		"--showuse",
		"--showmemuse",
		"--csv",
	).Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return nil
	}
	headers := strings.Split(lines[0], ",")
	values := strings.Split(lines[1], ",")
	if len(headers) != len(values) {
		return nil
	}
	idx := func(key string) int {
		for i, h := range headers {
			if strings.Contains(strings.ToLower(h), strings.ToLower(key)) {
				return i
			}
		}
		return -1
	}
	nameIdx := idx("product")
	useIdx := idx("gpu use")
	memIdx := idx("vram use")

	name, pct, vram := "AMD GPU", 0.0, int64(0)
	if nameIdx >= 0 {
		name = strings.Trim(strings.TrimSpace(values[nameIdx]), "\"")
	}
	if useIdx >= 0 {
		v := strings.TrimSuffix(strings.TrimSpace(values[useIdx]), "%")
		pct, _ = strconv.ParseFloat(v, 64)
	}
	if memIdx >= 0 {
		v := strings.Fields(strings.TrimSpace(values[memIdx]))
		if len(v) > 0 {
			mb, _ := strconv.ParseFloat(v[0], 64)
			if len(v) > 1 && strings.ToUpper(v[1]) == "GB" {
				mb *= 1024
			}
			vram = int64(mb)
		}
	}
	return &GPUInfo{Name: name, Pct: pct, VRAMMB: vram}
}
