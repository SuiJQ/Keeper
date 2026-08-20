package container

import (
	"os"
	"testing"

	"keeper/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBPFGenerator(t *testing.T) {
	gen := NewBPFGenerator(nil)
	require.NotNil(t, gen)
	
	assert.Equal(t, "default", gen.Name())
	
	bpf, err := gen.GenerateBPF()
	require.NoError(t, err)
	assert.NotEmpty(t, bpf)
	// BPF should be valid byte sequence
	assert.True(t, len(bpf) > 0, "BPF should not be empty")
}

func TestBPFGeneratorWithLogger(t *testing.T) {
	logger := log.New(os.Stderr)
	gen := NewBPFGenerator(logger)
	require.NotNil(t, gen)
	assert.Equal(t, "default", gen.Name())
}

func TestOverlayBuilder(t *testing.T) {
	builder := NewOverlayBuilder(nil)
	require.NotNil(t, builder)
	
	assert.Equal(t, "default", builder.Name())
	
	args := builder.BuildArgs("/lower", "/upper", "/work", "/mnt")
	require.Len(t, args, 11)
	assert.Contains(t, args, "--overlay")
	assert.Contains(t, args, "/mnt")
	assert.Contains(t, args, "--ro-bind")
	assert.Contains(t, args, "/lower")
	assert.Contains(t, args, "/mnt")
	assert.Contains(t, args, "--bind")
	assert.Contains(t, args, "/upper")
	assert.Contains(t, args, "/work")
	assert.Contains(t, args, "/work")
}

func TestOverlayBuilderWithLogger(t *testing.T) {
	logger := log.New(os.Stderr)
	builder := NewOverlayBuilder(logger)
	require.NotNil(t, builder)
}

func TestNewSeccompStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{"default", "default", "default", false},
		{"empty", "", "default", false},
		{"whitelist", "whitelist", "whitelist", false},
		{"blacklist", "blacklist", "blacklist", false},
		{"allow_all", "allow_all", "allow_all", false},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewSeccompStrategy(tt.arg, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Name())
			}
		})
	}
}

func TestNewSeccompStrategyWithLogger(t *testing.T) {
	logger := log.New(os.Stderr)
	got, err := NewSeccompStrategy("default", logger)
	require.NoError(t, err)
	assert.Equal(t, "default", got.Name())
}

func TestWhitelistStrategy(t *testing.T) {
	strat := &WhitelistStrategy{logger: log.New(os.Stderr)}
	
	assert.Equal(t, "whitelist", strat.Name())
	
	bpf, err := strat.GenerateBPF()
	require.NoError(t, err)
	assert.NotEmpty(t, bpf)
}

func TestBlacklistStrategy(t *testing.T) {
	strat := &BlacklistStrategy{logger: log.New(os.Stderr)}
	
	assert.Equal(t, "blacklist", strat.Name())
	
	bpf, err := strat.GenerateBPF()
	require.NoError(t, err)
	assert.NotEmpty(t, bpf)
}

func TestAllowAllStrategy(t *testing.T) {
	strat := &AllowAllStrategy{logger: log.New(os.Stderr)}
	
	assert.Equal(t, "allow_all", strat.Name())
	
	bpf, err := strat.GenerateBPF()
	require.NoError(t, err)
	assert.Empty(t, bpf, "allow_all should return empty BPF")
}

func TestNewOverlayStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{"default", "default", "default", false},
		{"empty", "", "default", false},
		{"overlayfs", "overlayfs", "overlayfs", false},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewOverlayStrategy(tt.arg, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Name())
			}
		})
	}
}

func TestNewOverlayStrategyWithLogger(t *testing.T) {
	logger := log.New(os.Stderr)
	got, err := NewOverlayStrategy("default", logger)
	require.NoError(t, err)
	assert.Equal(t, "default", got.Name())
}

func TestOverlayFSStrategy(t *testing.T) {
	strat := &OverlayFSStrategy{logger: log.New(os.Stderr)}
	
	assert.Equal(t, "overlayfs", strat.Name())
	
	args := strat.BuildArgs("/lower", "/upper", "/work", "/mnt")
	require.Len(t, args, 14)
	assert.Contains(t, args, "--overlay")
	assert.Contains(t, args, "/mnt")
	assert.Contains(t, args, "--setenv")
	assert.Contains(t, args, "XDG_RUNTIME_DIR")
	assert.Contains(t, args, "/tmp")
}

func TestBwrapContainerSetStrategies(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)
	
	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	
	// 设置 seccomp 策略
	seccompStrat := NewBPFGenerator(nil)
	bwrap.SetSeccompStrategy(seccompStrat)
	assert.NotNil(t, bwrap.seccompStrat)
	
	// 设置 overlay 策略
	overlayStrat := NewOverlayBuilder(nil)
	bwrap.SetOverlayStrategy(overlayStrat)
	assert.NotNil(t, bwrap.overlayStrat)
}

func TestStrategyInterfaces(t *testing.T) {
	// 验证所有策略都实现了接口
	var _ SeccompStrategy = (*BPFGenerator)(nil)
	var _ SeccompStrategy = (*WhitelistStrategy)(nil)
	var _ SeccompStrategy = (*BlacklistStrategy)(nil)
	var _ SeccompStrategy = (*AllowAllStrategy)(nil)
	
	var _ OverlayStrategy = (*OverlayBuilder)(nil)
	var _ OverlayStrategy = (*OverlayFSStrategy)(nil)
	
	// 验证工厂函数返回正确类型
	seccomp, err := NewSeccompStrategy("default", nil)
	require.NoError(t, err)
	assert.NotNil(t, seccomp)
	
	overlay, err := NewOverlayStrategy("default", nil)
	require.NoError(t, err)
	assert.NotNil(t, overlay)
}

func TestDefaultNetworkStrategy(t *testing.T) {
	strat := NewDefaultNetworkStrategy(nil)
	assert.Equal(t, "default", strat.Name())
	
	args, err := strat.Configure(ContainerSpec{})
	require.NoError(t, err)
	// 默认情况下可能返回空切片
	assert.NotNil(t, args)
}

func TestDefaultNetworkStrategyWithEnv(t *testing.T) {
	strat := NewDefaultNetworkStrategy(nil)
	
	args, err := strat.Configure(ContainerSpec{
		Envvars: []string{"FOO=bar", "BAZ=qux"},
	})
	require.NoError(t, err)
	assert.Contains(t, args, "--setenv=NAMESERVER=8.8.8.8")
}

func TestNewNetworkStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{"default", "default", "default", false},
		{"empty", "", "default", false},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewNetworkStrategy(tt.arg, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Name())
			}
		})
	}
}

func TestBwrapContainerSetNetworkStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)
	
	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	
	// 设置网络策略
	networkStrat := NewDefaultNetworkStrategy(nil)
	bwrap.SetNetworkStrategy(networkStrat)
	assert.NotNil(t, bwrap.networkStrat)
}

func TestDefaultResourceStrategy(t *testing.T) {
	strat := NewDefaultResourceStrategy(nil)
	assert.Equal(t, "default", strat.Name())
	
	args, err := strat.Configure(ContainerSpec{})
	require.NoError(t, err)
	assert.Empty(t, args)
	
	args, err = strat.Configure(ContainerSpec{ShmSize: 64})
	require.NoError(t, err)
	assert.Contains(t, args, "--shm-size=64m")
}

func TestNewResourceStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{"default", "default", "default", false},
		{"empty", "", "default", false},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewResourceStrategy(tt.arg, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Name())
			}
		})
	}
}

func TestBwrapContainerSetResourceStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)
	
	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	
	// 设置资源限制策略
	resourceStrat := NewDefaultResourceStrategy(nil)
	bwrap.SetResourceStrategy(resourceStrat)
	assert.NotNil(t, bwrap.resourceStrat)
}

func TestDefaultLogStrategy(t *testing.T) {
	strat := NewDefaultLogStrategy(nil)
	assert.Equal(t, "default", strat.Name())
	
	args, err := strat.Configure(ContainerSpec{})
	require.NoError(t, err)
	assert.Contains(t, args, "--log-level=info")
}

func TestNewLogStrategy(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		want    string
		wantErr bool
	}{
		{"default", "default", "default", false},
		{"empty", "", "default", false},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLogStrategy(tt.arg, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got.Name())
			}
		})
	}
}

func TestBwrapContainerSetLogStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)
	
	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	
	// 设置日志策略
	logStrat := NewDefaultLogStrategy(nil)
	bwrap.SetLogStrategy(logStrat)
	assert.NotNil(t, bwrap.logStrat)
}
