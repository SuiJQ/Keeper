package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// newTestRegistry 创建新的测试注册表（避免全局污染）
func newTestRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		summaries:  make(map[string]*Summary),
	}
}

// TestCounterInc 测试计数器增加
func TestCounterInc(t *testing.T) {
	registry := newTestRegistry()
	counter := registry.RegisterCounter("test_counter", "A test counter", []string{"label"})

	counter.Inc("value1")
	assert.Equal(t, int64(1), counter.Get())

	counter.Inc("value1")
	assert.Equal(t, int64(2), counter.Get())

	counter.Inc("value2")
	// 注意：当前实现将所有标签的值累加到同一个value字段
	assert.Equal(t, int64(3), counter.Get())
}

// TestCounterAdd 测试计数器加法
func TestCounterAdd(t *testing.T) {
	registry := newTestRegistry()
	counter := registry.RegisterCounter("test_add", "A test counter", []string{})

	counter.Add(5)
	assert.Equal(t, int64(5), counter.Get())

	counter.Add(10)
	assert.Equal(t, int64(15), counter.Get())
}

// TestCounterConcurrent 测试计数器并发安全
func TestCounterConcurrent(t *testing.T) {
	registry := newTestRegistry()
	counter := registry.RegisterCounter("test_concurrent", "A test counter", []string{})

	done := make(chan struct{}, 100)
	for i := 0; i < 100; i++ {
		go func() {
			counter.Inc()
			done <- struct{}{}
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	assert.Equal(t, int64(100), counter.Get())
}

// TestGaugeSet 测试仪表盘设置
func TestGaugeSet(t *testing.T) {
	registry := newTestRegistry()
	gauge := registry.RegisterGauge("test_gauge", "A test gauge", []string{"label"})

	gauge.Set(42.5, "value1")
	assert.Equal(t, 42.5, gauge.Get())

	gauge.Set(0)
	assert.Equal(t, 0.0, gauge.Get())
}

// TestGaugeIncDec 测试仪表盘增减
func TestGaugeIncDec(t *testing.T) {
	registry := newTestRegistry()
	gauge := registry.RegisterGauge("test_gauge_incdec", "A test gauge", []string{})

	gauge.Set(10)
	gauge.Inc()
	assert.Equal(t, 11.0, gauge.Get())

	gauge.Dec()
	assert.Equal(t, 10.0, gauge.Get())

	gauge.Add(5.5)
	assert.Equal(t, 15.5, gauge.Get())
}

// TestHistogramObserve 测试直方图观察
func TestHistogramObserve(t *testing.T) {
	registry := newTestRegistry()
	histogram := registry.RegisterHistogram("test_histogram", "A test histogram", []float64{0.1, 1, 10}, []string{})

	histogram.Observe(0.05)
	histogram.Observe(0.5)
	histogram.Observe(5)
	histogram.Observe(50)

	assert.Equal(t, int64(4), histogram.count)
	assert.Equal(t, 55.55, histogram.sum)
}

// TestPrometheusFormat 测试 Prometheus 格式导出
func TestPrometheusFormat(t *testing.T) {
	registry := newTestRegistry()
	counter := registry.RegisterCounter("test_prometheus", "A test counter for prometheus", []string{"label"})
	counter.Inc("value1")
	counter.Inc("value1")

	gauge := registry.RegisterGauge("test_gauge_prometheus", "A test gauge", []string{})
	gauge.Set(42.0)

	output := registry.PrometheusFormat()
	assert.Contains(t, output, "# HELP test_prometheus A test counter for prometheus")
	assert.Contains(t, output, "# TYPE test_prometheus counter")
	assert.Contains(t, output, `test_prometheus{label="value1"} 2`)

	assert.Contains(t, output, "# HELP test_gauge_prometheus A test gauge")
	assert.Contains(t, output, "# TYPE test_gauge_prometheus gauge")
	assert.Contains(t, output, "test_gauge_prometheus 42")
}

// TestRegistryIsolation 测试注册表隔离
func TestRegistryIsolation(t *testing.T) {
	registry1 := newTestRegistry()
	registry2 := newTestRegistry()

	c1 := registry1.RegisterCounter("isolated_counter", "A counter", []string{})
	c2 := registry2.RegisterCounter("isolated_counter", "A counter", []string{})

	c1.Inc()
	c2.Inc()
	c2.Inc()

	assert.Equal(t, int64(1), c1.Get())
	assert.Equal(t, int64(2), c2.Get())
}

// TestCounterLabels 测试计数器标签
func TestCounterLabels(t *testing.T) {
	registry := newTestRegistry()
	counter := registry.RegisterCounter("label_test", "A test counter", []string{"method", "status"})

	counter.Inc("GET", "200")
	counter.Inc("POST", "201")
	counter.Inc("GET", "200")

	// 由于updateLabels会覆盖labels，Get()返回的是最新标签的value
	assert.Equal(t, int64(3), counter.Get())
}

// TestGaugeLabels 测试仪表盘标签
func TestGaugeLabels(t *testing.T) {
	registry := newTestRegistry()
	gauge := registry.RegisterGauge("gauge_label_test", "A test gauge", []string{"host"})

	gauge.Set(10, "host1")
	gauge.Set(20, "host2")

	// 由于updateLabels会覆盖labels，Get()返回的是最新标签的value
	assert.Equal(t, 20.0, gauge.Get())
}

// TestHistogramBuckets 测试直方图分桶
func TestHistogramBuckets(t *testing.T) {
	registry := newTestRegistry()
	histogram := registry.RegisterHistogram("bucket_test", "A test histogram", []float64{1, 2, 5, 10}, []string{})

	histogram.Observe(0.5)
	histogram.Observe(1.5)
	histogram.Observe(3)
	histogram.Observe(7)
	histogram.Observe(15)

	// 验证直方图内部状态
	assert.Equal(t, int64(5), histogram.count)
	assert.Equal(t, 27.0, histogram.sum)
	assert.Len(t, histogram.values, 5)
}

// TestSummaryQuantiles 测试摘要分位数
func TestSummaryQuantiles(t *testing.T) {
	registry := newTestRegistry()
	summary := registry.RegisterSummary("summary_test", "A test summary", map[float64]float64{0.5: 0.05, 0.9: 0.01}, []string{})

	for i := 0; i < 100; i++ {
		summary.Observe(float64(i))
	}

	output := registry.PrometheusFormat()
	assert.Contains(t, output, "# TYPE summary_test summary")
	assert.Contains(t, output, "summary_test_sum")
	assert.Contains(t, output, "summary_test_count")
}

// TestPrometheusFormatNoMetrics 测试无指标时导出
func TestPrometheusFormatNoMetrics(t *testing.T) {
	registry := newTestRegistry()
	output := registry.PrometheusFormat()
	assert.Empty(t, strings.TrimSpace(output))
}

// TestCounterNegativeAdd 测试计数器负值加法（技术上允许，但语义上不合理）
func TestCounterNegativeAdd(t *testing.T) {
	registry := newTestRegistry()
	counter := registry.RegisterCounter("negative_test", "A test counter", []string{})

	counter.Add(10)
	counter.Add(-3)
	assert.Equal(t, int64(7), counter.Get())
}

// BenchmarkRegistryPrometheusFormat 基准测试 Prometheus 导出
func BenchmarkRegistryPrometheusFormat(b *testing.B) {
	registry := newTestRegistry()
	registry.RegisterCounter("bench_counter", "help", []string{"l"}).Inc("v")
	registry.RegisterGauge("bench_gauge", "help", []string{"l"}).Set(1)
	registry.RegisterHistogram("bench_histogram", "help", []float64{1}, []string{"l"}).Observe(1)
	registry.RegisterSummary("bench_summary", "help", map[float64]float64{0.5: 0.01}, []string{"l"}).Observe(1)

	for i := 0; i < b.N; i++ {
		_ = registry.PrometheusFormat()
	}
}

// TestRegistryGetters 测试 Registry 的 Getter 方法
func TestRegistryGetters(t *testing.T) {
	registry := newTestRegistry()

	// 注册一些指标
	counter := registry.RegisterCounter("getter_counter", "A test counter", []string{"label"})
	gauge := registry.RegisterGauge("getter_gauge", "A test gauge", []string{"label"})
	histogram := registry.RegisterHistogram("getter_histogram", "A test histogram", []float64{1, 2, 3}, []string{"label"})
	summary := registry.RegisterSummary("getter_summary", "A test summary", map[float64]float64{0.5: 0.05}, []string{"label"})

	// 测试 GetCounter
	c, exists := registry.GetCounter("getter_counter")
	assert.True(t, exists)
	assert.Equal(t, counter, c)

	// 测试 GetGauge
	g, exists := registry.GetGauge("getter_gauge")
	assert.True(t, exists)
	assert.Equal(t, gauge, g)

	// 测试 GetHistogram
	h, exists := registry.GetHistogram("getter_histogram")
	assert.True(t, exists)
	assert.Equal(t, histogram, h)

	// 测试 GetSummary
	s, exists := registry.GetSummary("getter_summary")
	assert.True(t, exists)
	assert.Equal(t, summary, s)

	// 测试获取不存在的指标
	_, exists = registry.GetCounter("nonexistent")
	assert.False(t, exists)

	_, exists = registry.GetGauge("nonexistent")
	assert.False(t, exists)

	_, exists = registry.GetHistogram("nonexistent")
	assert.False(t, exists)

	_, exists = registry.GetSummary("nonexistent")
	assert.False(t, exists)
}

// TestRegistryRegisterDuplicate 测试重复注册
func TestRegistryRegisterDuplicate(t *testing.T) {
	registry := newTestRegistry()

	// 第一次注册
	c1 := registry.RegisterCounter("dup_counter", "First", []string{})
	// 第二次注册应该返回同一个实例
	c2 := registry.RegisterCounter("dup_counter", "Second", []string{})
	assert.Equal(t, c1, c2)

	h1 := registry.RegisterHistogram("dup_histogram", "First", []float64{1}, []string{})
	h2 := registry.RegisterHistogram("dup_histogram", "Second", []float64{2}, []string{})
	assert.Equal(t, h1, h2)

	s1 := registry.RegisterSummary("dup_summary", "First", map[float64]float64{0.5: 0.05}, []string{})
	s2 := registry.RegisterSummary("dup_summary", "Second", map[float64]float64{0.9: 0.01}, []string{})
	assert.Equal(t, s1, s2)
}

// TestHTTPServerSetChecks 测试设置健康检查与就绪检查
func TestHTTPServerSetChecks(t *testing.T) {
	server := NewHTTPServer(":0")

	healthCalled := false
	readyCalled := false

	server.SetHealthCheck(func() error {
		healthCalled = true
		return nil
	})

	server.SetReadyCheck(func() error {
		readyCalled = true
		return nil
	})

	// 验证检查函数已设置（通过内部状态）
	server.mu.Lock()
	assert.NotNil(t, server.healthCheck)
	assert.NotNil(t, server.readyCheck)
	server.mu.Unlock()

	// 调用检查函数
	err := server.healthCheck()
	assert.NoError(t, err)
	assert.True(t, healthCalled)

	err = server.readyCheck()
	assert.NoError(t, err)
	assert.True(t, readyCalled)
}
