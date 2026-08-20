// Package watchdog 提供看门狗指标监控
package watchdog

import "keeper/internal/metrics"

// 看门狗指标
var (
	WatchdogStartCounter   = metrics.RegisterCounter("keeper_watchdog_start_total", "Total number of watchdog starts", []string{"result"})
	WatchdogStopCounter    = metrics.RegisterCounter("keeper_watchdog_stop_total", "Total number of watchdog stops", []string{"result"})
	AgentRegisterCounter   = metrics.RegisterCounter("keeper_watchdog_agent_register_total", "Total number of agent registrations", []string{"result"})
	AgentUnregisterCounter = metrics.RegisterCounter("keeper_watchdog_agent_unregister_total", "Total number of agent unregistrations", []string{"result"})
	AgentTimeoutCounter    = metrics.RegisterCounter("keeper_watchdog_agent_timeout_total", "Total number of agent timeouts", []string{"agent"})
	AgentDStateCounter     = metrics.RegisterCounter("keeper_watchdog_agent_dstate_total", "Total number of agent D state detections", []string{"agent"})
	AgentCheckDuration     = metrics.RegisterHistogram("keeper_watchdog_agent_check_duration_seconds", "Agent check duration in seconds", nil, []string{"agent"})
	WatchdogActiveGauge    = metrics.RegisterGauge("keeper_watchdog_active_agents", "Number of currently monitored agents", nil)
)

// RecordWatchdogStart 记录看门狗启动
func RecordWatchdogStart(result string) {
	WatchdogStartCounter.Inc(result)
}

// RecordWatchdogStop 记录看门狗停止
func RecordWatchdogStop(result string) {
	WatchdogStopCounter.Inc(result)
}

// RecordAgentRegister 记录 agent 注册
func RecordAgentRegister(result string) {
	AgentRegisterCounter.Inc(result)
}

// RecordAgentUnregister 记录 agent 注销
func RecordAgentUnregister(result string) {
	AgentUnregisterCounter.Inc(result)
}

// RecordAgentTimeout 记录 agent 超时
func RecordAgentTimeout(agentName string) {
	AgentTimeoutCounter.Inc(agentName)
}

// RecordAgentDState 记录 agent D 态检测
func RecordAgentDState(agentName string) {
	AgentDStateCounter.Inc(agentName)
}

// RecordAgentCheckDuration 记录 agent 检查耗时
func RecordAgentCheckDuration(agentName string, duration float64) {
	AgentCheckDuration.Observe(duration, agentName)
}

// SetActiveAgents 设置活跃 agent 数量
func SetActiveAgents(count float64) {
	WatchdogActiveGauge.Set(count)
}
