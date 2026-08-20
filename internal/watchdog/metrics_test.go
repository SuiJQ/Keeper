package watchdog

import (
	"testing"
)

// TestWatchdogMetrics 测试看门狗指标函数不会 panic
func TestWatchdogMetrics(t *testing.T) {
	RecordWatchdogStart("success")
	RecordWatchdogStop("success")
	RecordAgentRegister("success")
	RecordAgentUnregister("success")
	RecordAgentTimeout("test-agent")
	RecordAgentDState("test-agent")
	RecordAgentCheckDuration("test-agent", 0.5)
	SetActiveAgents(1)
	SetActiveAgents(0)
}

// TestWatchdogStartCounter 验证启动计数器
func TestWatchdogStartCounter(t *testing.T) {
	for i := 0; i < 5; i++ {
		RecordWatchdogStart("test")
	}
}

// TestWatchdogStopCounter 验证停止计数器
func TestWatchdogStopCounter(t *testing.T) {
	for i := 0; i < 5; i++ {
		RecordWatchdogStop("test")
	}
}

// TestAgentRegisterCounter 验证注册计数器
func TestAgentRegisterCounter(t *testing.T) {
	RecordAgentRegister("success")
	RecordAgentRegister("error")
}

// TestAgentUnregisterCounter 验证注销计数器
func TestAgentUnregisterCounter(t *testing.T) {
	RecordAgentUnregister("success")
	RecordAgentUnregister("error")
}

// TestAgentTimeoutCounter 验证超时计数器
func TestAgentTimeoutCounter(t *testing.T) {
	agents := []string{"agent1", "agent2", "agent3"}
	for _, agent := range agents {
		RecordAgentTimeout(agent)
	}
}

// TestAgentDStateCounter 验证 D 态计数器
func TestAgentDStateCounter(t *testing.T) {
	agents := []string{"agent1", "agent2"}
	for _, agent := range agents {
		RecordAgentDState(agent)
	}
}

// TestAgentCheckDuration 验证检查耗时
func TestAgentCheckDuration(t *testing.T) {
	durations := []float64{0.1, 0.5, 1.0, 2.5, 5.0}
	for _, d := range durations {
		RecordAgentCheckDuration("test-agent", d)
	}
}

// TestSetActiveAgents 验证活跃 agent 数量
func TestSetActiveAgents(t *testing.T) {
	testValues := []float64{0, 1, 5, 10, 100}
	for _, v := range testValues {
		SetActiveAgents(v)
	}
}

// TestWatchdogMetricsConcurrent 并发测试指标函数
func TestWatchdogMetricsConcurrent(t *testing.T) {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			agentName := "agent-" + string(rune(id+'0'))
			RecordWatchdogStart("concurrent")
			RecordAgentTimeout(agentName)
			RecordAgentCheckDuration(agentName, 0.1)
			SetActiveAgents(1)
			done <- true
		}(i)
	}
	
	for i := 0; i < 10; i++ {
		<-done
	}
}
