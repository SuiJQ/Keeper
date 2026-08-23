package network

import (
	"testing"
)

func BenchmarkNetworkMetrics(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RecordPortForward("success")
		RecordPortForwardDuration(0.1)
		RecordProxyConnection("success")
		RecordProxyConnectionDuration(0.1)
		RecordDataTransfer("to_container", 1024)
		SetForwarderActiveConnections("8080", 2)
	}
}
