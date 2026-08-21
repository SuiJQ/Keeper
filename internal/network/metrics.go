// Package network 提供网络模块的指标监控
package network

import "keeper/internal/metrics"

// 网络操作指标
var (
	PortForwardCounter      = metrics.RegisterCounter("keeper_network_port_forward_total", "Total number of port forward operations", []string{"result"})
	PortForwardDuration     = metrics.RegisterHistogram("keeper_network_port_forward_duration_seconds", "Port forward duration in seconds", nil, nil)
	ProxyConnectionCounter  = metrics.RegisterCounter("keeper_network_proxy_connection_total", "Total number of proxy connections", []string{"result"})
	ProxyConnectionDuration = metrics.RegisterHistogram("keeper_network_proxy_connection_duration_seconds", "Proxy connection duration in seconds", nil, nil)
	DataTransferBytes       = metrics.RegisterCounter("keeper_network_data_transfer_bytes_total", "Total data transfer bytes", []string{"direction"})
)

// RecordPortForward 记录端口转发
func RecordPortForward(result string) {
	PortForwardCounter.Inc(result)
}

// RecordPortForwardDuration 记录端口转发耗时
func RecordPortForwardDuration(duration float64) {
	PortForwardDuration.Observe(duration)
}

// RecordProxyConnection 记录代理连接
func RecordProxyConnection(result string) {
	ProxyConnectionCounter.Inc(result)
}

// RecordProxyConnectionDuration 记录代理连接耗时
func RecordProxyConnectionDuration(duration float64) {
	ProxyConnectionDuration.Observe(duration)
}

// RecordDataTransfer 记录数据传输
func RecordDataTransfer(direction string, bytes int64) {
	DataTransferBytes.Add(bytes, direction)
}
