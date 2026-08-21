package seccomp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateBPFDefault(t *testing.T) {
	filter := NewDefaultFilter()
	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	require.NotNil(t, bpf)
	require.Greater(t, len(bpf), 0)
	// 应该是 8 字节的倍数
	assert.Equal(t, 0, len(bpf)%8)
}

func TestGenerateBPFAllowAll(t *testing.T) {
	filter := &SeccompFilter{
		AllowList:     []string{},
		DenyList:      []string{},
		DefaultAction: RetAllow,
	}
	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	require.NotNil(t, bpf)
	// 应该只有一条 RET 指令
	assert.Equal(t, 8, len(bpf))
}

func TestGenerateBPFWhitelist(t *testing.T) {
	filter := &SeccompFilter{
		AllowList:     []string{"read", "write", "close"},
		DenyList:      []string{},
		DefaultAction: RetKill,
	}
	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	require.NotNil(t, bpf)
	require.Greater(t, len(bpf), 8)
}

func TestGenerateBPFBlacklist(t *testing.T) {
	filter := &SeccompFilter{
		AllowList:     []string{},
		DenyList:      []string{"reboot", "mount"},
		DefaultAction: RetAllow,
	}
	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	require.NotNil(t, bpf)
	require.Greater(t, len(bpf), 8)
}

func TestGenerateBPFUnknownSyscall(t *testing.T) {
	filter := &SeccompFilter{
		AllowList:     []string{"unknown_syscall_12345"},
		DenyList:      []string{},
		DefaultAction: RetAllow,
	}
	// 未知系统调用现在被跳过而不是返回错误
	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	assert.NotNil(t, bpf)
}

func TestBPFInstructionEncoding(t *testing.T) {
	// 测试 BPF 指令编码
	inst := BPFInstruction{
		Opcode: BPF_LD | BPF_W | BPF_ABS,
		Jt:     0,
		Jf:     0,
		K:      0,
	}

	// 使用 BPFProgram 来编码
	prog := &BPFProgram{Instructions: []BPFInstruction{inst}}
	data := prog.Bytes()
	require.Equal(t, 8, len(data))

	// 解码验证
	decoded := BPFInstruction{}
	decoded.Opcode = uint16(data[0]) | uint16(data[1])<<8
	decoded.Jt = data[2]
	decoded.Jf = data[3]
	decoded.K = uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16 | uint32(data[7])<<24

	assert.Equal(t, inst.Opcode, decoded.Opcode)
	assert.Equal(t, inst.Jt, decoded.Jt)
	assert.Equal(t, inst.Jf, decoded.Jf)
	assert.Equal(t, inst.K, decoded.K)
}

func TestGetSyscallNumber(t *testing.T) {
	// 测试已知系统调用
	num, ok := GetSyscallNumber("read")
	assert.True(t, ok)
	assert.Equal(t, uint32(0), num)

	num, ok = GetSyscallNumber("write")
	assert.True(t, ok)
	assert.Equal(t, uint32(1), num)

	// 测试未知系统调用
	num, ok = GetSyscallNumber("nonexistent_syscall")
	assert.False(t, ok)
	assert.Equal(t, uint32(0), num)
}

func TestSeccompFilterDefaults(t *testing.T) {
	filter := NewDefaultFilter()
	assert.NotNil(t, filter)
	assert.NotNil(t, filter.AllowList)
	assert.NotNil(t, filter.DenyList)
	assert.Greater(t, len(filter.AllowList), 0)
	assert.Greater(t, len(filter.DenyList), 0)
	assert.Equal(t, RetErrno, filter.DefaultAction)
}

func TestBPFProgramBytesAlignment(t *testing.T) {
	// 测试不同大小的 BPF 程序
	tests := []struct {
		name     string
		filter   *SeccompFilter
		minSize  int
		multiple int
	}{
		{
			name:     "allow all",
			filter:   &SeccompFilter{AllowList: []string{}, DenyList: []string{}, DefaultAction: RetAllow},
			minSize:  8,
			multiple: 8,
		},
		{
			name:     "single allow",
			filter:   &SeccompFilter{AllowList: []string{"read"}, DenyList: []string{}, DefaultAction: RetKill},
			minSize:  16,
			multiple: 8,
		},
		{
			name:     "multiple deny",
			filter:   &SeccompFilter{AllowList: []string{}, DenyList: []string{"reboot", "mount"}, DefaultAction: RetAllow},
			minSize:  24,
			multiple: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bpf, err := GenerateBPF(tt.filter)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(bpf), tt.minSize)
			assert.Equal(t, 0, len(bpf)%tt.multiple)
		})
	}
}

// 辅助函数：解析 BPF 指令
func parseBPFInstructions(data []byte) []BPFInstruction {
	var instructions []BPFInstruction
	for i := 0; i < len(data); i += 8 {
		if i+8 > len(data) {
			break
		}
		inst := BPFInstruction{
			Opcode: uint16(data[i]) | uint16(data[i+1])<<8,
			Jt:     data[i+2],
			Jf:     data[i+3],
			K:      uint32(data[i+4]) | uint32(data[i+5])<<8 | uint32(data[i+6])<<16 | uint32(data[i+7])<<24,
		}
		instructions = append(instructions, inst)
	}
	return instructions
}

func TestParseBPFInstructions(t *testing.T) {
	// 创建测试 BPF 程序
	prog := &BPFProgram{
		Instructions: []BPFInstruction{
			{Opcode: BPF_LD | BPF_W | BPF_ABS, K: 0},
			{Opcode: BPF_JMP | BPF_JEQ | BPF_K, Jt: 1, Jf: 0, K: 0},
			{Opcode: BPF_RET | BPF_K, K: uint32(RetKill)},
			{Opcode: BPF_RET | BPF_K, K: uint32(RetAllow)},
		},
	}

	data := prog.Bytes()
	instructions := parseBPFInstructions(data)

	require.Len(t, instructions, 4)
	assert.Equal(t, uint16(BPF_LD|BPF_W|BPF_ABS), instructions[0].Opcode)
	assert.Equal(t, uint16(BPF_JMP|BPF_JEQ|BPF_K), instructions[1].Opcode)
	assert.Equal(t, uint16(BPF_RET|BPF_K), instructions[2].Opcode)
	assert.Equal(t, uint16(BPF_RET|BPF_K), instructions[3].Opcode)
}

func TestBPFBytesConsistency(t *testing.T) {
	// 测试 Bytes() 和解析的一致性
	original := &BPFProgram{
		Instructions: []BPFInstruction{
			{Opcode: 0x00, Jt: 0, Jf: 0, K: 100},
			{Opcode: 0x15, Jt: 1, Jf: 2, K: 200},
			{Opcode: 0x06, Jt: 0, Jf: 0, K: 0x7fff0000},
		},
	}

	data := original.Bytes()
	parsed := parseBPFInstructions(data)

	require.Len(t, parsed, len(original.Instructions))
	for i, inst := range original.Instructions {
		assert.Equal(t, inst.Opcode, parsed[i].Opcode)
		assert.Equal(t, inst.Jt, parsed[i].Jt)
		assert.Equal(t, inst.Jf, parsed[i].Jf)
		assert.Equal(t, inst.K, parsed[i].K)
	}
}

func TestBPFEmptyProgram(t *testing.T) {
	prog := &BPFProgram{}
	data := prog.Bytes()
	assert.Equal(t, 0, len(data))
}

func TestSeccompFilterAllowDenyConflict(t *testing.T) {
	// 测试同时有 Allow 和 Deny 的情况
	// DenyList 应该优先
	filter := &SeccompFilter{
		AllowList:     []string{"read", "write", "reboot"}, // reboot 也在 AllowList 中
		DenyList:      []string{"reboot"},                  // 但被 DenyList 拒绝
		DefaultAction: RetKill,
	}

	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	require.NotNil(t, bpf)

	// 解析 BPF 程序，检查是否有 3 条允许指令（read, write）+ 默认动作
	instructions := parseBPFInstructions(bpf)

	// 应该有：1 条 LD 指令 + 2 条比较跳转 + 1 条默认动作 + 1 条 ALLOW = 5 条
	assert.GreaterOrEqual(t, len(instructions), 5)
}

func TestRetActionConstants(t *testing.T) {
	// 测试返回动作常量
	assert.Equal(t, uint32(0x00000000), uint32(RetKill))
	assert.Equal(t, uint32(0x00000001), uint32(RetTrap))
	assert.Equal(t, uint32(0x00010000), uint32(RetErrno))
	assert.Equal(t, uint32(0x00700000), uint32(RetTrace))
	assert.Equal(t, uint32(0x7fff0000), uint32(RetAllow))
	assert.Equal(t, uint32(0x00000002), uint32(RetKillThread))
}

func TestSeccompFilterNil(t *testing.T) {
	// 测试 nil 过滤器
	bpf, err := GenerateBPF(nil)
	require.NoError(t, err)
	require.NotNil(t, bpf)
	assert.Equal(t, 8, len(bpf)) // 应该只有一条 RET ALLOW
}

func TestSeccompFilterEmptyDefaultAction(t *testing.T) {
	// 测试空 DefaultAction（应该被替换为 RetAllow）
	filter := &SeccompFilter{
		AllowList:     []string{},
		DenyList:      []string{},
		DefaultAction: 0,
	}

	bpf, err := GenerateBPF(filter)
	require.NoError(t, err)
	require.NotNil(t, bpf)
	// 应该返回 RetAllow
	instructions := parseBPFInstructions(bpf)
	assert.Equal(t, uint32(RetAllow), instructions[0].K)
}

// Benchmark 测试
func BenchmarkGenerateBPFDefault(b *testing.B) {
	filter := NewDefaultFilter()
	for i := 0; i < b.N; i++ {
		_, err := GenerateBPF(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateBPFWhitelist(b *testing.B) {
	filter := &SeccompFilter{
		AllowList:     []string{"read", "write", "open", "close", "stat", "fstat", "mmap", "munmap", "brk", "ioctl"},
		DenyList:      []string{},
		DefaultAction: RetKill,
	}
	for i := 0; i < b.N; i++ {
		_, err := GenerateBPF(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateBPFBlacklist(b *testing.B) {
	filter := &SeccompFilter{
		AllowList:     []string{},
		DenyList:      []string{"reboot", "mount", "umount", "swapon", "swapoff", "pivot_root", "chroot"},
		DefaultAction: RetAllow,
	}
	for i := 0; i < b.N; i++ {
		_, err := GenerateBPF(filter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestNewWhitelistFilter 测试创建白名单过滤器
func TestNewWhitelistFilter(t *testing.T) {
	filter := NewWhitelistFilter()
	require.NotNil(t, filter)
	assert.NotEmpty(t, filter.AllowList)
	assert.Equal(t, RetErrno, filter.DefaultAction) // 白名单默认返回 Errno

	// 验证包含常用系统调用
	assert.Contains(t, filter.AllowList, "read")
	assert.Contains(t, filter.AllowList, "write")
	assert.Contains(t, filter.AllowList, "open")
	assert.Contains(t, filter.AllowList, "close")
}

// TestNewBlacklistFilter 测试创建黑名单过滤器
func TestNewBlacklistFilter(t *testing.T) {
	filter := NewBlacklistFilter()
	require.NotNil(t, filter)
	assert.NotEmpty(t, filter.DenyList)
	assert.Equal(t, RetAllow, filter.DefaultAction)

	// 验证包含危险系统调用
	assert.Contains(t, filter.DenyList, "reboot")
	assert.Contains(t, filter.DenyList, "mount")
	assert.Contains(t, filter.DenyList, "umount")
	assert.Contains(t, filter.DenyList, "kexec_load")
}
