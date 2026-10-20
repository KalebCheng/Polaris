package asyncd

import (
	"fmt"
	"strings"
)

// errJobNotFound 任务定义不存在（入队即拒绝：定义是"谁执行、怎么执行"的唯一来源）
func errJobNotFound(name string) error {
	return fmt.Errorf("task: 未注册的任务定义 %q（请先用 Service.RegisterJob 声明）", name)
}

// errUnknownTaskType 定义指向了未注册的任务类型（启动期声明与处理器注册不匹配）
func errUnknownTaskType(taskType string) error {
	return fmt.Errorf("task: 未注册的任务类型 %q（请先用 Registry.Register 登记处理器）", taskType)
}

// errNoHandler 领取到执行实例但找不到处理器（通常是定义被回滚的旧任务）→ 入死信等待人工处理
func errNoHandler(taskType string, registered []string) error {
	return fmt.Errorf("task: 任务类型 %q 没有处理器；已注册: %s", taskType, strings.Join(registered, ","))
}
