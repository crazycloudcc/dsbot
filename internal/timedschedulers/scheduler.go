package timedschedulers

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskFunc 任务执行函数类型
type TaskFunc func() error

// ScheduleMode 调度模式
type ScheduleMode int

const (
	// ModeInterval 固定间隔模式
	ModeInterval ScheduleMode = iota
	// ModeAlignedWithDelay 对齐时间模式（每时0/15/30/45分+延迟）
	ModeAlignedWithDelay
)

// Scheduler 定时任务调度器
type Scheduler struct {
	mode           ScheduleMode       // 调度模式
	interval       time.Duration      // 间隔时间（ModeInterval模式使用）
	alignMinutes   []int              // 对齐的分钟数（ModeAlignedWithDelay模式使用）
	delay          time.Duration      // 延迟时间（ModeAlignedWithDelay模式使用）
	task           TaskFunc           // 要执行的任务
	runImmediately bool               // 是否立即执行
	ctx            context.Context    // 上下文
	cancel         context.CancelFunc // 取消函数
	wg             sync.WaitGroup     // 等待组
	running        bool               // 是否正在运行
	mu             sync.Mutex         // 互斥锁
	onError        func(error)        // 错误处理函数
	onComplete     func()             // 任务完成回调
}

// SchedulerOption 调度器选项
type SchedulerOption func(*Scheduler)

// WithInterval 设置固定间隔模式
func WithInterval(interval time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		s.mode = ModeInterval
		s.interval = interval
	}
}

// WithAlignedSchedule 设置对齐时间模式（根据间隔自动计算对齐点+延迟）
// interval: 时间间隔（如5分钟、15分钟）
// delay: 在对齐点后的延迟时间
func WithAlignedSchedule(delay time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		s.mode = ModeAlignedWithDelay
		// 根据interval自动计算对齐点
		s.alignMinutes = calculateAlignMinutes(s.interval)
		s.delay = delay
	}
}

// WithCustomAlignedSchedule 设置自定义对齐时间模式
func WithCustomAlignedSchedule(minutes []int, delay time.Duration) SchedulerOption {
	return func(s *Scheduler) {
		s.mode = ModeAlignedWithDelay
		s.alignMinutes = minutes
		s.delay = delay
	}
}

// calculateAlignMinutes 根据间隔时间计算对齐的分钟点
// 例如：5分钟 -> [0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55]
//
//	15分钟 -> [0, 15, 30, 45]
//	30分钟 -> [0, 30]
func calculateAlignMinutes(interval time.Duration) []int {
	intervalMinutes := int(interval.Minutes())

	// 确保间隔在合理范围内（1-60分钟）
	if intervalMinutes < 1 {
		intervalMinutes = 1
	}
	if intervalMinutes > 60 {
		intervalMinutes = 60
	}

	// 如果间隔不能整除60，使用固定间隔模式更合适
	// 但为了兼容，我们仍然生成对齐点
	var alignMinutes []int
	for minute := 0; minute < 60; minute += intervalMinutes {
		alignMinutes = append(alignMinutes, minute)
	}

	return alignMinutes
}

// WithRunImmediately 设置是否立即执行
func WithRunImmediately(immediate bool) SchedulerOption {
	return func(s *Scheduler) {
		s.runImmediately = immediate
	}
}

// WithErrorHandler 设置错误处理函数
func WithErrorHandler(handler func(error)) SchedulerOption {
	return func(s *Scheduler) {
		s.onError = handler
	}
}

// WithCompleteHandler 设置任务完成回调
func WithCompleteHandler(handler func()) SchedulerOption {
	return func(s *Scheduler) {
		s.onComplete = handler
	}
}

// NewScheduler 创建新的调度器
func NewScheduler(task TaskFunc, interval time.Duration, options ...SchedulerOption) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		mode:           ModeInterval,
		interval:       interval,
		task:           task,
		runImmediately: true,
		ctx:            ctx,
		cancel:         cancel,
	}

	// 应用选项
	for _, opt := range options {
		opt(s)
	}

	return s
}

// Start 启动调度器
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("调度器已在运行中")
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run()

	return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// IsRunning 检查调度器是否正在运行
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// run 运行调度器主循环
func (s *Scheduler) run() {
	defer s.wg.Done()

	// 立即执行一次
	if s.runImmediately {
		s.executeTask()
	}

	// 根据模式运行
	switch s.mode {
	case ModeInterval:
		s.runIntervalMode()
	case ModeAlignedWithDelay:
		s.runAlignedMode()
	}
}

// runIntervalMode 固定间隔模式
func (s *Scheduler) runIntervalMode() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.executeTask()
		case <-s.ctx.Done():
			return
		}
	}
}

// runAlignedMode 对齐时间模式
func (s *Scheduler) runAlignedMode() {
	for {
		// 计算下次执行时间
		nextRun := s.calculateNextAlignedTime()

		// 等待到下次执行时间
		waitDuration := time.Until(nextRun)

		select {
		case <-time.After(waitDuration):
			s.executeTask()
		case <-s.ctx.Done():
			return
		}
	}
}

// calculateNextAlignedTime 计算下次对齐的执行时间
func (s *Scheduler) calculateNextAlignedTime() time.Time {
	now := time.Now()

	// 找到下一个对齐的分钟数
	currentMinute := now.Minute()
	var nextMinute int
	var addHour bool

	// 查找大于当前分钟的下一个对齐点
	found := false
	for _, min := range s.alignMinutes {
		if min > currentMinute {
			nextMinute = min
			found = true
			break
		}
	}

	// 如果没找到，使用第一个对齐点，并加1小时
	if !found {
		nextMinute = s.alignMinutes[0]
		addHour = true
	}

	// 构建下次执行时间
	nextTime := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		now.Hour(),
		nextMinute,
		0,
		0,
		now.Location(),
	)

	// 如果需要加1小时
	if addHour {
		nextTime = nextTime.Add(1 * time.Hour)
	}

	// 加上延迟
	nextTime = nextTime.Add(s.delay)

	return nextTime
}

// executeTask 执行任务
func (s *Scheduler) executeTask() {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("任务执行panic: %v", r)
			s.handleError(err)
		}
	}()

	if err := s.task(); err != nil {
		s.handleError(err)
	} else {
		if s.onComplete != nil {
			s.onComplete()
		}
	}
}

// handleError 处理错误
func (s *Scheduler) handleError(err error) {
	if s.onError != nil {
		s.onError(err)
	}
}

// GetNextRunTime 获取下次运行时间
func (s *Scheduler) GetNextRunTime() time.Time {
	switch s.mode {
	case ModeInterval:
		return time.Now().Add(s.interval)
	case ModeAlignedWithDelay:
		return s.calculateNextAlignedTime()
	default:
		return time.Time{}
	}
}

// GetMode 获取调度模式
func (s *Scheduler) GetMode() ScheduleMode {
	return s.mode
}

// GetInterval 获取间隔时间（仅ModeInterval模式有效）
func (s *Scheduler) GetInterval() time.Duration {
	return s.interval
}

// GetAlignMinutes 获取对齐分钟数（仅ModeAlignedWithDelay模式有效）
func (s *Scheduler) GetAlignMinutes() []int {
	return s.alignMinutes
}

// GetDelay 获取延迟时间（仅ModeAlignedWithDelay模式有效）
func (s *Scheduler) GetDelay() time.Duration {
	return s.delay
}
