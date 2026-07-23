package main

import (
        "net/http"
        "os"
        "strconv"
        "strings"
        "time"

        "github.com/prometheus/client_golang/prometheus"
        "github.com/prometheus/client_golang/prometheus/promhttp"
)

func read(path string) string {
        b, err := os.ReadFile(path)
        if err != nil {
                return ""
        }
        return strings.TrimSpace(string(b))
}

func readFloat(path string) float64 {
        v := read(path)
        if v == "" {
                return 0
        }
        f, _ := strconv.ParseFloat(v, 64)
        return f
}

type GPUSample struct {
        TempC     float64
        PowerW    float64
        VRAMUsed  float64
        VRAMTotal float64
        GTTUsed   float64
        GTTTotal  float64
}

func sampleGPU() GPUSample {
        hwmonBase := "/sys/class/drm/card0/device/hwmon"
        hwmon := ""

        ents, err := os.ReadDir(hwmonBase)
        if err == nil && len(ents) > 0 {
                hwmon = hwmonBase + "/" + ents[0].Name()
        }

        s := GPUSample{}

        // Temperature
        if hwmon != "" {
                s.TempC = readFloat(hwmon+"/temp1_input") / 1000.0
                s.PowerW = readFloat(hwmon+"/power1_average") / 1000000.0
        }

        // Memory (APU unified memory)
        s.VRAMUsed = readFloat("/sys/class/drm/card0/device/mem_info_vram_used")
        s.VRAMTotal = readFloat("/sys/class/drm/card0/device/mem_info_vram_total")

        s.GTTUsed = readFloat("/sys/class/drm/card0/device/mem_info_gtt_used")
        s.GTTTotal = readFloat("/sys/class/drm/card0/device/mem_info_gtt_total")

        return s
}

func memPressure() float64 {
        data := read("/proc/meminfo")

        var total, avail float64

        lines := strings.Split(data, "\n")
        for _, l := range lines {
                if strings.HasPrefix(l, "MemTotal:") {
                        fields := strings.Fields(l)
                        total, _ = strconv.ParseFloat(fields[1], 64)
                }
                if strings.HasPrefix(l, "MemAvailable:") {
                        fields := strings.Fields(l)
                        avail, _ = strconv.ParseFloat(fields[1], 64)
                }
        }

        if total == 0 {
                return 0
        }
        return (total - avail) / total
}

var (
        gpuTemp = prometheus.NewGauge(prometheus.GaugeOpts{
                Name: "amd_gpu_temp_celsius",
        })

        gpuPower = prometheus.NewGauge(prometheus.GaugeOpts{
                Name: "amd_gpu_power_watts",
        })

        memUsed = prometheus.NewGauge(prometheus.GaugeOpts{
                Name: "amd_gpu_vram_used_bytes",
        })

        memTotal = prometheus.NewGauge(prometheus.GaugeOpts{
                Name: "amd_gpu_vram_total_bytes",
        })

        sysMemPressure = prometheus.NewGauge(prometheus.GaugeOpts{
                Name: "amd_system_memory_pressure_ratio",
        })
        gpuBusy = prometheus.NewGauge(prometheus.GaugeOpts{
                Name: "amd_gpu_busy_percent",
        })
)

func readBusy() float64 {
        return readFloat("/sys/class/drm/card0/device/gpu_busy_percent")
}

func collect() {
        s := sampleGPU()

        gpuTemp.Set(s.TempC)
        gpuPower.Set(s.PowerW)

        memUsed.Set(s.VRAMUsed)
        memTotal.Set(s.VRAMTotal)

        sysMemPressure.Set(memPressure())

        gpuBusy.Set(readBusy())
}

func main() {
        prometheus.MustRegister(gpuTemp)
        prometheus.MustRegister(gpuPower)
        prometheus.MustRegister(memUsed)
        prometheus.MustRegister(memTotal)
        prometheus.MustRegister(sysMemPressure)
        prometheus.MustRegister(gpuBusy)

        go func() {
                for {
                        collect()
                        time.Sleep(1 * time.Second)
                }
        }()

        http.Handle("/metrics", promhttp.Handler())
        http.ListenAndServe("0.0.0.0:9101", nil)
}
