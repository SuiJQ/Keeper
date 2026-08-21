// Package metrics 提供指标导出能力
package metrics

import (
	"fmt"
	"strings"
	"sync"
)

// Counter 计数器指标
type Counter struct {
	name       string
	help       string
	value      int64
	labels     map[string]string
	labelNames []string
	mu         sync.RWMutex
}

// NewCounter 创建计数器
func NewCounter(name, help string, labelNames []string) *Counter {
	return &Counter{
		name:       name,
		help:       help,
		labels:     make(map[string]string),
		labelNames: labelNames,
	}
}

// Inc 增加计数器
func (c *Counter) Inc(labels ...string) {
	c.Add(1, labels...)
}

// Add 增加计数器
func (c *Counter) Add(delta int64, labels ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
	c.updateLabels(labels)
}

// Get 获取计数器值
func (c *Counter) Get() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// updateLabels 更新标签
func (c *Counter) updateLabels(values []string) {
	if len(values) != len(c.labelNames) {
		return
	}
	for i, name := range c.labelNames {
		c.labels[name] = values[i]
	}
}

// Gauge 仪表盘指标
type Gauge struct {
	name       string
	help       string
	value      float64
	labels     map[string]string
	labelNames []string
	mu         sync.RWMutex
}

// NewGauge 创建仪表盘
func NewGauge(name, help string, labelNames []string) *Gauge {
	return &Gauge{
		name:       name,
		help:       help,
		labels:     make(map[string]string),
		labelNames: labelNames,
	}
}

// Set 设置仪表盘值
func (g *Gauge) Set(value float64, labels ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value = value
	g.updateLabels(labels)
}

// Inc 增加仪表盘值
func (g *Gauge) Inc(labels ...string) {
	g.Add(1, labels...)
}

// Dec 减少仪表盘值
func (g *Gauge) Dec(labels ...string) {
	g.Add(-1, labels...)
}

// Add 增加仪表盘值
func (g *Gauge) Add(delta float64, labels ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.value += delta
	g.updateLabels(labels)
}

// Get 获取仪表盘值
func (g *Gauge) Get() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.value
}

// updateLabels 更新标签
func (g *Gauge) updateLabels(values []string) {
	if len(values) != len(g.labelNames) {
		return
	}
	for i, name := range g.labelNames {
		g.labels[name] = values[i]
	}
}

// Histogram 直方图指标
type Histogram struct {
	name       string
	help       string
	buckets    []float64
	count      int64
	sum        float64
	values     []float64
	labels     map[string]string
	labelNames []string
	mu         sync.RWMutex
}

// NewHistogram 创建直方图
func NewHistogram(name, help string, buckets []float64, labelNames []string) *Histogram {
	if buckets == nil {
		buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	}
	return &Histogram{
		name:       name,
		help:       help,
		buckets:    buckets,
		labels:     make(map[string]string),
		labelNames: labelNames,
		values:     make([]float64, 0),
	}
}

// Observe 观察一个值
func (h *Histogram) Observe(value float64, labels ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count++
	h.sum += value
	h.values = append(h.values, value)
	h.updateLabels(labels)
}

// updateLabels 更新标签
func (h *Histogram) updateLabels(values []string) {
	if len(values) != len(h.labelNames) {
		return
	}
	for i, name := range h.labelNames {
		h.labels[name] = values[i]
	}
}

// Summary 摘要指标
type Summary struct {
	name       string
	help       string
	objectives map[float64]float64
	count      int64
	sum        float64
	values     []float64
	labels     map[string]string
	labelNames []string
	mu         sync.RWMutex
}

// NewSummary 创建摘要
func NewSummary(name, help string, objectives map[float64]float64, labelNames []string) *Summary {
	if objectives == nil {
		objectives = map[float64]float64{
			0.5:  0.05,
			0.9:  0.01,
			0.99: 0.001,
		}
	}
	return &Summary{
		name:       name,
		help:       help,
		objectives: objectives,
		labels:     make(map[string]string),
		labelNames: labelNames,
		values:     make([]float64, 0),
	}
}

// Observe 观察一个值
func (s *Summary) Observe(value float64, labels ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	s.sum += value
	s.values = append(s.values, value)
	s.updateLabels(labels)
}

// updateLabels 更新标签
func (s *Summary) updateLabels(values []string) {
	if len(values) != len(s.labelNames) {
		return
	}
	for i, name := range s.labelNames {
		s.labels[name] = values[i]
	}
}

// Registry 指标注册表
type Registry struct {
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
	summaries  map[string]*Summary
	mu         sync.RWMutex
}

// NewRegistry 创建注册表
func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
		summaries:  make(map[string]*Summary),
	}
}

// RegisterCounter 注册计数器
func (r *Registry) RegisterCounter(name, help string, labelNames []string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.counters[name]; !exists {
		r.counters[name] = NewCounter(name, help, labelNames)
	}
	return r.counters[name]
}

// RegisterGauge 注册仪表盘
func (r *Registry) RegisterGauge(name, help string, labelNames []string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.gauges[name]; !exists {
		r.gauges[name] = NewGauge(name, help, labelNames)
	}
	return r.gauges[name]
}

// RegisterHistogram 注册直方图
func (r *Registry) RegisterHistogram(name, help string, buckets []float64, labelNames []string) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.histograms[name]; !exists {
		r.histograms[name] = NewHistogram(name, help, buckets, labelNames)
	}
	return r.histograms[name]
}

// RegisterSummary 注册摘要
func (r *Registry) RegisterSummary(name, help string, objectives map[float64]float64, labelNames []string) *Summary {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.summaries[name]; !exists {
		r.summaries[name] = NewSummary(name, help, objectives, labelNames)
	}
	return r.summaries[name]
}

// GetCounter 获取计数器
func (r *Registry) GetCounter(name string) (*Counter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, exists := r.counters[name]
	return c, exists
}

// GetGauge 获取仪表盘
func (r *Registry) GetGauge(name string) (*Gauge, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	g, exists := r.gauges[name]
	return g, exists
}

// GetHistogram 获取直方图
func (r *Registry) GetHistogram(name string) (*Histogram, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, exists := r.histograms[name]
	return h, exists
}

// GetSummary 获取摘要
func (r *Registry) GetSummary(name string) (*Summary, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, exists := r.summaries[name]
	return s, exists
}

// PrometheusFormat 导出为 Prometheus 格式
func (r *Registry) PrometheusFormat() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var output string

	// 导出计数器
	for name, counter := range r.counters {
		output += fmt.Sprintf("# HELP %s %s\n", name, counter.help)
		output += fmt.Sprintf("# TYPE %s counter\n", name)
		labels := formatLabels(counter.labels, counter.labelNames)
		output += fmt.Sprintf("%s%s %d\n", name, labels, counter.Get())
		output += "\n"
	}

	// 导出仪表盘
	for name, gauge := range r.gauges {
		output += fmt.Sprintf("# HELP %s %s\n", name, gauge.help)
		output += fmt.Sprintf("# TYPE %s gauge\n", name)
		labels := formatLabels(gauge.labels, gauge.labelNames)
		output += fmt.Sprintf("%s%s %f\n", name, labels, gauge.Get())
		output += "\n"
	}

	// 导出直方图
	for name, histogram := range r.histograms {
		output += fmt.Sprintf("# HELP %s %s\n", name, histogram.help)
		output += fmt.Sprintf("# TYPE %s histogram\n", name)
		for _, bucket := range histogram.buckets {
			labels := formatLabels(histogram.labels, histogram.labelNames)
			labels = fmt.Sprintf("%s,le=\"%g\"", labels, bucket)
			count := histogram.count
			for _, v := range histogram.values {
				if v <= bucket {
					count++
				}
			}
			output += fmt.Sprintf("%s_bucket%s %d\n", name, labels, count)
		}
		labels := formatLabels(histogram.labels, histogram.labelNames)
		output += fmt.Sprintf("%s_sum%s %f\n", name, labels, histogram.sum)
		output += fmt.Sprintf("%s_count%s %d\n", name, labels, histogram.count)
		output += "\n"
	}

	// 导出摘要
	for name, summary := range r.summaries {
		output += fmt.Sprintf("# HELP %s %s\n", name, summary.help)
		output += fmt.Sprintf("# TYPE %s summary\n", name)
		for quantile := range summary.objectives {
			labels := formatLabels(summary.labels, summary.labelNames)
			labels = fmt.Sprintf("%s,quantile=\"%g\"", labels, quantile)
			value := calculateQuantile(summary.values, quantile)
			output += fmt.Sprintf("%s%s %f\n", name, labels, value)
		}
		labels := formatLabels(summary.labels, summary.labelNames)
		output += fmt.Sprintf("%s_sum%s %f\n", name, labels, summary.sum)
		output += fmt.Sprintf("%s_count%s %d\n", name, labels, summary.count)
		output += "\n"
	}

	return output
}

// formatLabels 格式化标签
func formatLabels(labels map[string]string, labelNames []string) string {
	if len(labels) == 0 {
		return ""
	}

	var pairs []string
	for _, name := range labelNames {
		if value, exists := labels[name]; exists {
			pairs = append(pairs, fmt.Sprintf("%s=\"%s\"", name, value))
		}
	}

	if len(pairs) == 0 {
		return ""
	}

	return "{" + strings.Join(pairs, ",") + "}"
}

// calculateQuantile 计算分位数
func calculateQuantile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// 简单排序
	sorted := make([]float64, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	index := quantile * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[lower]
	}

	fraction := index - float64(lower)
	return sorted[lower] + fraction*(sorted[upper]-sorted[lower])
}

// 全局注册表
var defaultRegistry = NewRegistry()

// RegisterCounter 注册计数器（全局）
func RegisterCounter(name, help string, labelNames []string) *Counter {
	return defaultRegistry.RegisterCounter(name, help, labelNames)
}

// RegisterGauge 注册仪表盘（全局）
func RegisterGauge(name, help string, labelNames []string) *Gauge {
	return defaultRegistry.RegisterGauge(name, help, labelNames)
}

// RegisterHistogram 注册直方图（全局）
func RegisterHistogram(name, help string, buckets []float64, labelNames []string) *Histogram {
	return defaultRegistry.RegisterHistogram(name, help, buckets, labelNames)
}

// RegisterSummary 注册摘要（全局）
func RegisterSummary(name, help string, objectives map[float64]float64, labelNames []string) *Summary {
	return defaultRegistry.RegisterSummary(name, help, objectives, labelNames)
}

// PrometheusFormat 导出为 Prometheus 格式（全局）
func PrometheusFormat() string {
	return defaultRegistry.PrometheusFormat()
}
