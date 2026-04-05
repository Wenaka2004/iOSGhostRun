//go:build !headless

package services

import "github.com/wailsapp/wails/v3/pkg/application"

// WailsEventDispatcher 包装 Wails 事件系统
type WailsEventDispatcher struct{}

func (w *WailsEventDispatcher) Emit(event string, data interface{}) {
	application.Get().Event.Emit(event, data)
}

// InitWailsEvents 初始化 Wails 事件分发器
func InitWailsEvents() {
	GlobalEvents = &WailsEventDispatcher{}
}
