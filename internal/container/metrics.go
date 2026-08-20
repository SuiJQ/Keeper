// Package container 提供容器运行时的指标监控
package container

import "keeper/internal/metrics"

// 容器生命周期指标
var (
	ContainerStartCounter  = metrics.RegisterCounter("keeper_container_start_total", "Total number of container starts", []string{"runtime", "result"})
	ContainerStopCounter   = metrics.RegisterCounter("keeper_container_stop_total", "Total number of container stops", []string{"runtime", "result"})
	ContainerExecCounter   = metrics.RegisterCounter("keeper_container_exec_total", "Total number of container exec calls", []string{"runtime", "result"})
	ContainerStartDuration = metrics.RegisterHistogram("keeper_container_start_duration_seconds", "Container start duration in seconds", nil, []string{"runtime"})
	ContainerStopDuration  = metrics.RegisterHistogram("keeper_container_stop_duration_seconds", "Container stop duration in seconds", nil, []string{"runtime"})
	ContainerExecDuration  = metrics.RegisterHistogram("keeper_container_exec_duration_seconds", "Container exec duration in seconds", nil, []string{"runtime"})
	ContainerActiveGauge   = metrics.RegisterGauge("keeper_container_active", "Number of currently active containers", []string{"runtime"})
)

// RecordContainerStart 记录容器启动
func RecordContainerStart(runtime, result string) {
	ContainerStartCounter.Inc(runtime, result)
}

// RecordContainerStop 记录容器停止
func RecordContainerStop(runtime, result string) {
	ContainerStopCounter.Inc(runtime, result)
}

// RecordContainerExec 记录容器执行
func RecordContainerExec(runtime, result string) {
	ContainerExecCounter.Inc(runtime, result)
}

// RecordContainerStartDuration 记录容器启动耗时
func RecordContainerStartDuration(runtime string, duration float64) {
	ContainerStartDuration.Observe(duration, runtime)
}

// RecordContainerStopDuration 记录容器停止耗时
func RecordContainerStopDuration(runtime string, duration float64) {
	ContainerStopDuration.Observe(duration, runtime)
}

// RecordContainerExecDuration 记录容器执行耗时
func RecordContainerExecDuration(runtime string, duration float64) {
	ContainerExecDuration.Observe(duration, runtime)
}

// SetContainerActive 设置活跃容器数
func SetContainerActive(runtime string, count float64) {
	ContainerActiveGauge.Set(count, runtime)
}
