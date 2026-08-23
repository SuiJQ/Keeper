package container

import "sync"

// registry 全局容器运行时注册表，用于跨函数共享活跃容器实例。
// 仅用于进程内生命周期关联，不会持久化到磁盘。
var registry = struct {
	mu     sync.Mutex
	byName map[string]Container
}{
	byName: make(map[string]Container),
}

// Register 注册一个容器实例。若已存在同名实例，则覆盖。
func Register(name string, c Container) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.byName[name] = c
}

// Unregister 注销容器实例。
func Unregister(name string) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	delete(registry.byName, name)
}

// Get 获取已注册的容器实例。
func Get(name string) (Container, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	c, ok := registry.byName[name]
	return c, ok
}
