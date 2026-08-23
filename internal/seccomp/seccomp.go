// Package seccomp 提供 Seccomp BPF 程序生成能力
package seccomp

import (
	"encoding/binary"
	"fmt"
)

// BPFInstruction Seccomp BPF 指令（8 字节）
type BPFInstruction struct {
	Opcode uint16
	Jt     uint8
	Jf     uint8
	K      uint32
}

// BPFProgram Seccomp BPF 程序
type BPFProgram struct {
	Instructions []BPFInstruction
}

// Filter Seccomp 过滤器配置
type Filter struct {
	// 允许的系统调用（如果为空，则默认允许所有，除非在 DenyList 中）
	AllowList []string
	// 拒绝的系统调用（优先级高于 AllowList）
	DenyList []string
	// 默认动作：kill、errno、allow 等
	DefaultAction RetAction
}

// RetAction 返回动作
type RetAction uint32

const (
	// RetKill 终止进程
	RetKill RetAction = 0x00000000
	// RetTrap 发送 SIGSYS
	RetTrap RetAction = 0x00000001
	// RetErrno 返回 errno
	RetErrno RetAction = 0x00010000
	// RetTrace 通知 tracer
	RetTrace RetAction = 0x00700000
	// RetAllow 允许
	RetAllow RetAction = 0x7fff0000
	// RetKillThread 终止线程
	RetKillThread RetAction = 0x00000002
)

// BPF 操作码常量
const (
	// BPF Class
	BPF_LD  = 0x00
	BPF_JMP = 0x05
	BPF_ALU = 0x04
	BPF_RET = 0x06

	// BPF Size
	BPF_W = 0x00

	// BPF Mode
	BPF_ABS = 0x20
	BPF_IND = 0x40
	BPF_IMM = 0x00

	// BPF Jump
	BPF_JA  = 0x00
	BPF_JEQ = 0x10
	BPF_JGT = 0x20
	BPF_JGE = 0x30

	// BPF ALU
	BPF_ADD = 0x00
	BPF_SUB = 0x10
	BPF_MUL = 0x20
	BPF_DIV = 0x30
	BPF_OR  = 0x40
	BPF_AND = 0x50
	BPF_LSH = 0x60
	BPF_RSH = 0x70
	BPF_NEG = 0x80
	BPF_MOD = 0x90
	BPF_XOR = 0xa0

	// BPF Mem
	BPF_MEM = 0x10

	// BPF Return
	BPF_K = 0x00
)

// 系统调用号映射（x86_64）
var syscallNumbers = map[string]uint32{
	"read":                   0,
	"write":                  1,
	"open":                   2,
	"openat":                 257,
	"close":                  3,
	"creat":                  85,
	"unlink":                 87,
	"unlinkat":               263,
	"mkdir":                  83,
	"mkdirat":                258,
	"rmdir":                  84,
	"chdir":                  80,
	"fchdir":                 81,
	"getcwd":                 79,
	"stat":                   4,
	"statx":                  332,
	"fstat":                  5,
	"lstat":                  6,
	"access":                 21,
	"faccessat":              269,
	"readlink":               89,
	"readlinkat":             298,
	"chmod":                  90,
	"fchmod":                 91,
	"chown":                  92,
	"fchown":                 93,
	"lchown":                 94,
	"poll":                   7,
	"lseek":                  8,
	"mmap":                   9,
	"mprotect":               10,
	"munmap":                 11,
	"brk":                    12,
	"rt_sigaction":           13,
	"rt_sigprocmask":         14,
	"rt_sigreturn":           15,
	"ioctl":                  16,
	"pread64":                17,
	"pwrite64":               18,
	"readv":                  19,
	"writev":                 20,
	"pipe":                   22,
	"select":                 23,
	"pselect6":               270,
	"sched_yield":            24,
	"mremap":                 25,
	"msync":                  26,
	"mincore":                27,
	"madvise":                28,
	"shmget":                 29,
	"shmat":                  30,
	"shmctl":                 31,
	"dup":                    32,
	"dup2":                   33,
	"pause":                  34,
	"nanosleep":              35,
	"getitimer":              36,
	"alarm":                  37,
	"setitimer":              38,
	"getpid":                 39,
	"sendfile":               40,
	"socket":                 41,
	"connect":                42,
	"accept":                 43,
	"sendto":                 44,
	"recvfrom":               45,
	"sendmsg":                46,
	"recvmsg":                47,
	"shutdown":               48,
	"bind":                   49,
	"listen":                 50,
	"getsockname":            51,
	"getpeername":            52,
	"socketpair":             53,
	"setsockopt":             54,
	"getsockopt":             55,
	"clone":                  56,
	"fork":                   57,
	"vfork":                  58,
	"execve":                 59,
	"exit":                   60,
	"wait4":                  61,
	"kill":                   62,
	"uname":                  63,
	"semget":                 64,
	"semop":                  65,
	"semctl":                 66,
	"shmdt":                  67,
	"msgget":                 68,
	"msgsnd":                 69,
	"msgrcv":                 70,
	"msgctl":                 71,
	"fcntl":                  72,
	"flock":                  73,
	"fsync":                  74,
	"fdatasync":              75,
	"truncate":               76,
	"ftruncate":              77,
	"getdents":               78,
	"rename":                 82,
	"link":                   86,
	"symlink":                88,
	"umask":                  95,
	"gettimeofday":           96,
	"getrlimit":              97,
	"getrusage":              98,
	"sysinfo":                99,
	"times":                  100,
	"ptrace":                 101,
	"getuid":                 102,
	"syslog":                 103,
	"getgid":                 104,
	"setuid":                 105,
	"setgid":                 106,
	"geteuid":                107,
	"getegid":                108,
	"setpgid":                109,
	"getppid":                110,
	"getpgrp":                111,
	"setsid":                 112,
	"setreuid":               113,
	"setregid":               114,
	"getgroups":              115,
	"setgroups":              116,
	"setresuid":              117,
	"getresuid":              118,
	"setresgid":              119,
	"getresgid":              120,
	"getpgid":                121,
	"setfsuid":               122,
	"setfsgid":               123,
	"getsid":                 124,
	"capget":                 125,
	"capset":                 126,
	"rt_sigpending":          127,
	"rt_sigtimedwait":        128,
	"rt_sigqueueinfo":        129,
	"rt_sigsuspend":          130,
	"sigaltstack":            131,
	"utime":                  132,
	"mknod":                  133,
	"personality":            134,
	"ustat":                  135,
	"statfs":                 136,
	"fstatfs":                137,
	"sysfs":                  138,
	"getpriority":            139,
	"setpriority":            140,
	"sched_setparam":         141,
	"sched_getparam":         142,
	"sched_setscheduler":     143,
	"sched_getscheduler":     144,
	"sched_get_priority_max": 145,
	"sched_get_priority_min": 146,
	"sched_rr_get_interval":  147,
	"setns":                  308,
	"unshare":                335,
	"mlock":                  148,
	"munlock":                149,
	"mlockall":               150,
	"munlockall":             151,
	"vhangup":                152,
	"modify_ldt":             153,
	"pivot_root":             154,
	"_sysctl":                155,
	"prctl":                  156,
	"arch_prctl":             158,
	"adjtimex":               159,
	"setrlimit":              160,
	"chroot":                 161,
	"sync":                   162,
	"acct":                   163,
	"settimeofday":           164,
	"mount":                  165,
	"umount2":                166,
	"umount":                 166,
	"swapon":                 167,
	"swapoff":                168,
	"reboot":                 169,
	"setdomainname":          170,
	"iopl":                   171,
	"ioperm":                 172,
	"create_module":          174,
	"delete_module":          175,
	"get_kernel_syms":        177,
	"query_module":           178,
	"init_module":            176,
	"finit_module":           350,
	"kexec_load":             203,
	"quotactl":               179,
	"nfsservctl":             180,
	"getpmsg":                181,
	"putpmsg":                182,
	"afs_syscall":            183,
	"tuxcall":                184,
	"security":               185,
	"gettid":                 186,
	"readahead":              187,
	"setxattr":               188,
	"lsetxattr":              189,
	"fsetxattr":              190,
	"getxattr":               191,
	"lgetxattr":              192,
	"fgetxattr":              193,
	"listxattr":              194,
	"llistxattr":             195,
	"flistxattr":             196,
	"removexattr":            197,
	"lremovexattr":           198,
	"fremovexattr":           199,
	"tkill":                  200,
	"time":                   201,
	"clock_gettime":          228,
	"clock_getres":           229,
	"clock_nanosleep":        230,
	"futex":                  202,
	"sched_setaffinity":      203,
	"sched_getaffinity":      204,
	"set_thread_area":        205,
	"io_setup":               206,
	"io_destroy":             207,
	"io_getevents":           208,
	"io_submit":              209,
	"io_cancel":              210,
	"get_thread_area":        211,
	"lookup_dcookie":         212,
	"epoll_create":           213,
	"epoll_ctl_old":          214,
	"epoll_wait_old":         215,
	"epoll_create1":          233,
	"epoll_ctl":              233,
	"epoll_wait":             232,
	"epoll_pwait":            281,
	"dup3":                   292,
	"pipe2":                  293,
	"inotify_init":           294,
	"inotify_add_watch":      296,
	"inotify_rm_watch":       297,
	"eventfd":                291,
	"eventfd2":               290,
	"signalfd":               289,
	"signalfd4":              289,
	"memfd_create":           319,
	"userfaultfd":            323,
	"fallocate":              324,
	"preadv":                 327,
	"pwritev":                328,
	"getcpu":                 309,
	"renameat":               302,
	"newfstatat":             307,
	"statfs64":               268,
	"fstatfs64":              269,
	"prlimit64":              302,
	"seccomp":                317,
	"getrandom":              318,
	"bpf":                    321,
	"execveat":               322,
	"process_vm_readv":       348,
	"process_vm_writev":      347,
	"kcmp":                   349,
	"pidfd_send_signal":      351,
}

// GenerateBPF 生成 Seccomp BPF 程序
func GenerateBPF(filter *Filter) ([]byte, error) {
	if filter == nil {
		filter = &Filter{
			DefaultAction: RetAllow,
		}
	}

	if filter.DefaultAction == 0 {
		filter.DefaultAction = RetAllow
	}

	// 构建系统调用映射
	allowed := make(map[uint32]bool)
	for _, name := range filter.AllowList {
		num, ok := syscallNumbers[name]
		if !ok {
			// 未知系统调用，记录警告但继续
			continue
		}
		allowed[num] = true
	}

	denied := make(map[uint32]bool)
	for _, name := range filter.DenyList {
		num, ok := syscallNumbers[name]
		if !ok {
			// 未知系统调用，记录警告但继续
			continue
		}
		denied[num] = true
	}

	prog := &BPFProgram{}

	// 如果只有 DenyList，生成基于黑名单的 BPF
	if len(filter.AllowList) == 0 && len(filter.DenyList) > 0 {
		return generateBlacklistBPF(prog, denied, filter.DefaultAction)
	}

	// 如果有 AllowList，生成基于白名单的 BPF
	if len(filter.AllowList) > 0 {
		return generateWhitelistBPF(prog, allowed, denied, filter.DefaultAction)
	}

	// 默认：允许所有
	return generateAllowAllBPF(prog, filter.DefaultAction), nil
}

// generateWhitelistBPF 生成白名单 BPF 程序
func generateWhitelistBPF(prog *BPFProgram, allowed map[uint32]bool, denied map[uint32]bool, defaultAction RetAction) ([]byte, error) {
	// BPF 程序结构：
	// 1. 加载系统调用号 (A = syscall_number)
	// 2. 对每个允许的系统调用，检查是否匹配
	// 3. 如果匹配，跳转到允许
	// 4. 默认执行 defaultAction

	// 允许列表（排除拒绝列表中的）
	effectiveAllowed := make(map[uint32]bool)
	for num := range allowed {
		if !denied[num] {
			effectiveAllowed[num] = true
		}
	}

	// 按系统调用号排序
	var syscallNums []uint32
	for num := range effectiveAllowed {
		syscallNums = append(syscallNums, num)
	}
	// 简单排序
	for i := 0; i < len(syscallNums); i++ {
		for j := i + 1; j < len(syscallNums); j++ {
			if syscallNums[i] > syscallNums[j] {
				syscallNums[i], syscallNums[j] = syscallNums[j], syscallNums[i]
			}
		}
	}

	// 构建跳转表
	// 使用二分查找优化（简化实现：线性比较）

	// 加载系统调用号到 A 寄存器
	prog.Instructions = append(prog.Instructions, BPFInstruction{
		Opcode: BPF_LD | BPF_W | BPF_ABS,
		K:      0, // offset 0: syscall number
	})

	// 为每个允许的系统调用添加比较和跳转
	for i, num := range syscallNums {
		// 如果 A == num，跳转到 ALLOW 标签
		// 需要计算跳转偏移
		// 当前指令索引 = 1 + i
		// 目标指令索引 = 1 + len(syscallNums) + 1 (ALLOW 标签)
		jumpOffset := len(syscallNums) - i + 1
		if jumpOffset > 255 {
			return nil, fmt.Errorf("too many syscalls for BPF jump offset: %d > 255", jumpOffset)
		}
		prog.Instructions = append(prog.Instructions, BPFInstruction{
			Opcode: BPF_JMP | BPF_JEQ | BPF_K,
			Jt:     uint8(jumpOffset), // #nosec G115
			Jf:     1,
			K:      num,
		})
	}

	// 默认动作
	prog.Instructions = append(prog.Instructions, BPFInstruction{
		Opcode: BPF_RET | BPF_K,
		K:      uint32(defaultAction),
	})

	// ALLOW 标签
	prog.Instructions = append(prog.Instructions, BPFInstruction{
		Opcode: BPF_RET | BPF_K,
		K:      uint32(RetAllow),
	})

	return prog.Bytes(), nil
}

// generateBlacklistBPF 生成黑名单 BPF 程序
func generateBlacklistBPF(prog *BPFProgram, denied map[uint32]bool, defaultAction RetAction) ([]byte, error) {
	// BPF 程序结构：
	// 1. 加载系统调用号
	// 2. 检查是否在拒绝列表中
	// 3. 如果在，执行拒绝动作
	// 4. 否则执行默认动作

	// 加载系统调用号
	prog.Instructions = append(prog.Instructions, BPFInstruction{
		Opcode: BPF_LD | BPF_W | BPF_ABS,
		K:      0,
	})

	// 为每个拒绝的系统调用添加检查
	var syscallNums []uint32
	for num := range denied {
		syscallNums = append(syscallNums, num)
	}
	// 排序
	for i := 0; i < len(syscallNums); i++ {
		for j := i + 1; j < len(syscallNums); j++ {
			if syscallNums[i] > syscallNums[j] {
				syscallNums[i], syscallNums[j] = syscallNums[j], syscallNums[i]
			}
		}
	}

	for _, num := range syscallNums {
		// A == num -> 跳转到 DENY
		// 跳转偏移需要计算
		denyOffset := len(syscallNums)
		if denyOffset > 255 {
			return nil, fmt.Errorf("too many syscalls for BPF jump offset: %d > 255", denyOffset)
		}
		prog.Instructions = append(prog.Instructions, BPFInstruction{
			Opcode: BPF_JMP | BPF_JEQ | BPF_K,
			Jt:     uint8(denyOffset),
			Jf:     1,
			K:      num,
		})
	}

	// 默认动作
	prog.Instructions = append(prog.Instructions, BPFInstruction{
		Opcode: BPF_RET | BPF_K,
		K:      uint32(defaultAction),
	})

	// DENY 标签
	for range syscallNums {
		prog.Instructions = append(prog.Instructions, BPFInstruction{
			Opcode: BPF_RET | BPF_K,
			K:      uint32(RetKill),
		})
	}

	return prog.Bytes(), nil
}

// generateAllowAllBPF 生成允许所有的 BPF 程序
func generateAllowAllBPF(prog *BPFProgram, defaultAction RetAction) []byte {
	// 简单的 BPF 程序：直接返回允许
	prog.Instructions = append(prog.Instructions, BPFInstruction{
		Opcode: BPF_RET | BPF_K,
		K:      uint32(RetAllow),
	})
	return prog.Bytes()
}

// Bytes 将 BPF 程序转换为字节流
func (p *BPFProgram) Bytes() []byte {
	data := make([]byte, len(p.Instructions)*8)
	for i, inst := range p.Instructions {
		binary.LittleEndian.PutUint16(data[i*8:], inst.Opcode)
		data[i*8+2] = inst.Jt
		data[i*8+3] = inst.Jf
		binary.LittleEndian.PutUint32(data[i*8+4:], inst.K)
	}
	return data
}

// NewDefaultFilter 创建默认过滤器
func NewDefaultFilter() *Filter {
	return &Filter{
		AllowList: []string{
			// 基本 I/O
			"read", "write", "close", "lseek", "mmap", "munmap", "brk",
			// 进程管理
			"getpid", "getppid", "getuid", "getgid", "geteuid", "getegid",
			"fork", "vfork", "execve", "exit", "wait4", "kill",
			// 文件系统
			"open", "openat", "creat", "unlink", "unlinkat", "mkdir", "mkdirat",
			"rmdir", "chdir", "fchdir", "getcwd", "stat", "statx", "fstat",
			"lstat", "access", "faccessat", "readlink", "readlinkat",
			// 时间
			"time", "gettimeofday", "clock_gettime", "nanosleep",
			// 信号
			"rt_sigaction", "rt_sigprocmask", "rt_sigreturn", "rt_sigpending",
			"rt_sigtimedwait", "rt_sigqueueinfo", "rt_sigsuspend", "sigaltstack",
			// 网络
			"socket", "bind", "listen", "accept", "connect", "sendto", "recvfrom",
			"sendmsg", "recvmsg", "shutdown", "getsockname", "getpeername",
			"setsockopt", "getsockopt", "socketpair",
			// 内存
			"mprotect", "madvise", "msync", "mincore", "mlock", "munlock",
			"mlockall", "munlockall",
			// 其他
			"ioctl", "poll", "select", "pselect6", "epoll_create", "epoll_create1",
			"epoll_ctl", "epoll_wait", "epoll_pwait", "eventfd", "eventfd2",
			"signalfd", "signalfd4", "pipe", "pipe2", "dup", "dup2", "dup3",
			"fcntl", "flock", "fsync", "fdatasync", "sync",
			"getrandom", "getcpu", "gettid",
			// 调度
			"sched_yield", "sched_getaffinity", "sched_setaffinity",
			"sched_getparam", "sched_setparam", "sched_getscheduler",
			"sched_setscheduler", "sched_get_priority_max", "sched_get_priority_min",
			"sched_rr_get_interval",
			// 命名空间
			"setns", "unshare",
			// 其他安全相关
			"prctl", "capget", "capset",
		},
		DenyList: []string{
			// 禁止这些危险系统调用
			"reboot", "mount", "umount", "swapon", "swapoff",
			"pivot_root", "chroot", "create_module", "delete_module",
			"init_module", "finit_module", "kexec_load", "syslog",
			"iopl", "ioperm", "modify_ldt", "ptrace", "process_vm_readv",
			"process_vm_writev", "kcmp", "pidfd_send_signal",
		},
		DefaultAction: RetErrno,
	}
}

// NewWhitelistFilter 创建白名单过滤器（只允许指定系统调用）
func NewWhitelistFilter() *Filter {
	return &Filter{
		AllowList: []string{
			"read", "write", "close", "mmap", "munmap", "brk",
			"getpid", "getppid", "getuid", "getgid", "geteuid", "getegid",
			"exit", "wait4",
			"open", "openat", "creat", "unlink", "unlinkat", "stat", "fstat",
			"faccessat", "readlink", "readlinkat",
			"time", "gettimeofday", "clock_gettime", "nanosleep",
			"rt_sigaction", "rt_sigprocmask", "rt_sigreturn",
			"socket", "bind", "listen", "accept", "connect", "sendto", "recvfrom",
			"shutdown", "getsockname", "getpeername",
			"mprotect", "madvise",
			"ioctl", "poll", "select", "pselect6",
			"epoll_create", "epoll_create1", "epoll_ctl", "epoll_wait",
			"pipe", "pipe2", "dup", "dup2", "dup3",
			"fcntl", "flock", "fsync", "fdatasync",
			"getrandom", "gettid",
			"setns", "unshare",
			"prctl", "capget", "capset",
		},
		DefaultAction: RetErrno,
	}
}

// NewBlacklistFilter 创建黑名单过滤器（拒绝指定系统调用）
func NewBlacklistFilter() *Filter {
	return &Filter{
		DenyList: []string{
			"reboot", "mount", "umount", "swapon", "swapoff",
			"pivot_root", "chroot", "create_module", "delete_module",
			"init_module", "finit_module", "kexec_load", "syslog",
			"iopl", "ioperm", "modify_ldt", "ptrace", "process_vm_readv",
			"process_vm_writev", "kcmp", "pidfd_send_signal",
			"kexec_file_load", "bpf", "userfaultfd", "landlock_create_ruleset",
			"landlock_add_rule", "landlock_restrict_self",
		},
		DefaultAction: RetAllow,
	}
}

// GetSyscallNumber 获取系统调用号
func GetSyscallNumber(name string) (uint32, bool) {
	num, ok := syscallNumbers[name]
	return num, ok
}
