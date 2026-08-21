package container

// Strategy 接口定义汇总
//
// 本包采用插件化策略模式，将容器运行时的各个关注点拆分为独立策略：
//   - SeccompStrategy：Seccomp BPF 生成
//   - OverlayStrategy：OverlayFS 挂载
//   - NetworkStrategy：网络配置
//   - ResourceStrategy：资源限制
//   - LogStrategy：日志配置
//
// 每个策略都有对应的工厂函数（如 NewSeccompStrategy），
// 便于后续扩展 KubeVirt、runC 等后端。
