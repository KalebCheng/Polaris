package asyncd

import (
	"fmt"
	"strings"
)

// errUnknownTaskType 未注册的任务类型（入队即拒绝，避免静默堆积）
func errUnknownTaskType(taskType string) error {
	return fmt.Errorf("task: 未注册的任务类型 %q（请先用 Registry.Register 登记处理器）", taskType)
}

// errNoHandler 领取到任务但找不到处理器（通常是被回滚的旧类型）→ 入死信等待人工处理
func errNoHandler(taskType string, registered []string) error {
	return fmt.Errorf("task: 任务类型 %q 没有处理器；已注册: %s", taskType, strings.Join(registered, ","))
}
