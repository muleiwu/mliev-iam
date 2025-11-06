package reload

// reloadChan 用于触发重启的通道
var reloadChan = make(chan struct{}, 1)

// GetReloadChan 获取重启通道，供API调用
func GetReloadChan() chan struct{} {
	return reloadChan
}

// TriggerReload 触发重载（非阻塞）
func TriggerReload() bool {
	select {
	case reloadChan <- struct{}{}:
		return true
	default:
		// 通道已满，说明已经有重启请求在队列中
		return false
	}
}
