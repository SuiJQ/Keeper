// Package metrics 提供指标导出能力
package metrics

import (
	"fmt"
	"strconv"
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

// 环形缓冲区指标
var (
	// RingBufferDropped 环形缓冲区丢弃条目计数
	RingBufferDropped = RegisterCounter("keeper_ringbuffer_dropped_total", "Total number of log entries dropped by ring buffer", nil)
	// RingBufferSize 环形缓冲区当前大小
	RingBufferSize = RegisterGauge("keeper_ringbuffer_size", "Current number of entries in ring buffer", nil)
)

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
	for name, counter := range r.counters {
		output += counterFormat(name, counter)
	}
	for name, gauge := range r.gauges {
		output += gaugeFormat(name, gauge)
	}
	for name, histogram := range r.histograms {
		output += histogramFormat(name, histogram)
	}
	for name, summary := range r.summaries {
		output += summaryFormat(name, summary)
	}
	return output
}

func counterFormat(name string, c *Counter) string {
	labels := formatLabels(c.labels, c.labelNames)
	return "# HELP " + name + " " + c.help + "\n# TYPE " + name + " counter\n" + name + labels + " " + strconv.FormatInt(c.Get(), 10) + "\n\n"
}

func gaugeFormat(name string, g *Gauge) string {
	labels := formatLabels(g.labels, g.labelNames)
	return "# HELP " + name + " " + g.help + "\n# TYPE " + name + " gauge\n" + name + labels + " " + strconv.FormatFloat(g.Get(), 'f', -1, 64) + "\n\n"
}

func histogramFormat(name string, h *Histogram) string {
	labels := formatLabels(h.labels, h.labelNames)
	var buckets string
	for _, bucket := range h.buckets {
		bl := labels + ",le=\"" + strconv.FormatFloat(bucket, 'f', -1, 64) + "\""
		count := h.count
		for _, v := range h.values {
			if v <= bucket {
				count++
			}
		}
		buckets += name + "_bucket" + bl + " " + strconv.FormatInt(count, 10) + "\n"
	}
	return "# HELP " + name + " " + h.help + "\n# TYPE " + name + " histogram\n" + buckets + name + "_sum" + labels + " " + strconv.FormatFloat(h.sum, 'f', -1, 64) + "\n" + name + "_count" + labels + " " + strconv.FormatInt(h.count, 10) + "\n\n"
}

func summaryFormat(name string, s *Summary) string {
	labels := formatLabels(s.labels, s.labelNames)
	var quantiles string
	for quantile := range s.objectives {
		ql := labels + ",quantile=\"" + strconv.FormatFloat(quantile, 'f', -1, 64) + "\""
		value := calculateQuantile(s.values, quantile)
		quantiles += name + ql + " " + strconv.FormatFloat(value, 'f', -1, 64) + "\n"
	}
	return "# HELP " + name + " " + s.help + "\n# TYPE " + name + " summary\n" + quantiles + name + "_sum" + labels + " " + strconv.FormatFloat(s.sum, 'f', -1, 64) + "\n" + name + "_count" + labels + " " + strconv.FormatInt(s.count, 10) + "\n\n"
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
