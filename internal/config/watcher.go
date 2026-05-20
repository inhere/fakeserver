package config

import (
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher 是 fsnotify 包装：监听一组配置文件 + 防抖窗口 + 触发回调。
// design §5.2 热加载入口；Task 7 填充 Watch / Stop。
//
// 当前 stub 仅占位，让 import 链路就绪。
type Watcher struct {
	debounce time.Duration
	inner    *fsnotify.Watcher
}
