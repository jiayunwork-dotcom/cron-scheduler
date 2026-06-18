package scheduler

import (
	"bytes"
	"context"
	"cron-scheduler/internal/alerter"
	"cron-scheduler/internal/cronparser"
	"cron-scheduler/internal/models"
	"cron-scheduler/internal/redis"
	"cron-scheduler/internal/repository"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Scheduler struct {
	repo           *repository.Repository
	redisClient    *redis.RedisClient
	alerter        *alerter.Alerter
	maxConcurrent  int
	running        bool
	stopCh         chan struct{}
	wg             sync.WaitGroup
	executingTasks sync.Map
	lastTrigger    map[string]time.Time
	lastTriggerMu  sync.Mutex
}

func NewScheduler(repo *repository.Repository, rc *redis.RedisClient, al *alerter.Alerter, maxConcurrent int) *Scheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}
	return &Scheduler{
		repo:          repo,
		redisClient:   rc,
		alerter:       al,
		maxConcurrent: maxConcurrent,
		stopCh:        make(chan struct{}),
		lastTrigger:   make(map[string]time.Time),
	}
}

func (s *Scheduler) Start() error {
	s.running = true

	if err := s.markRunningTasksInterrupted(); err != nil {
		return fmt.Errorf("标记中断任务失败: %w", err)
	}

	s.wg.Add(1)
	go s.scanLoop()

	for i := 0; i < s.maxConcurrent; i++ {
		s.wg.Add(1)
		go s.workerLoop()
	}

	s.wg.Add(1)
	go s.cleanupLoop()

	return nil
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.running = false
}

func (s *Scheduler) scanLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			now = now.Truncate(time.Second)
			s.scanTasks(now)
		}
	}
}

func (s *Scheduler) scanTasks(now time.Time) {
	enabled := true
	tasks, err := s.repo.ListTasks(&enabled, "")
	if err != nil {
		return
	}

	for i := range tasks {
		task := &tasks[i]
		cronExpr, err := cronparser.Parse(task.CronExpr)
		if err != nil {
			continue
		}

		if !matchesCurrentSecond(cronExpr, now) {
			continue
		}

		s.lastTriggerMu.Lock()
		lastTrig, ok := s.lastTrigger[task.ID.String()]
		if ok && lastTrig.Equal(now) {
			s.lastTriggerMu.Unlock()
			continue
		}
		s.lastTrigger[task.ID.String()] = now
		s.lastTriggerMu.Unlock()

		_ = s.checkDependenciesAndEnqueue(task, now, models.TriggerCron)
	}
}

func (s *Scheduler) checkDependenciesAndEnqueue(task *models.Task, triggerTime time.Time, triggerType string) error {
	taskID := task.ID.String()

	if _, loaded := s.executingTasks.LoadOrStore(taskID, true); loaded {
		exec := &models.ExecutionHistory{
			ID:           uuid.New(),
			TaskID:       task.ID,
			TaskName:     task.Name,
			TriggerType:  triggerType,
			TriggerTime:  triggerTime,
			Status:       models.StatusSkipped,
			ErrorMessage: "并发冲突",
			CreatedAt:    time.Now(),
		}
		s.executingTasks.Delete(taskID)
		return s.repo.CreateExecution(exec)
	}
	s.executingTasks.Delete(taskID)

	exists, err := s.redisClient.RunningTaskExists(taskID)
	if err == nil && exists {
		exec := &models.ExecutionHistory{
			ID:           uuid.New(),
			TaskID:       task.ID,
			TaskName:     task.Name,
			TriggerType:  triggerType,
			TriggerTime:  triggerTime,
			Status:       models.StatusSkipped,
			ErrorMessage: "并发冲突",
			CreatedAt:    time.Now(),
		}
		return s.repo.CreateExecution(exec)
	}

	depMet := s.checkDependencies(task)

	if !depMet {
		exec := &models.ExecutionHistory{
			ID:           uuid.New(),
			TaskID:       task.ID,
			TaskName:     task.Name,
			TriggerType:  triggerType,
			TriggerTime:  triggerTime,
			Status:       models.StatusSkipped,
			ErrorMessage: "依赖条件不满足",
			CreatedAt:    time.Now(),
		}
		return s.repo.CreateExecution(exec)
	}

	exec := &models.ExecutionHistory{
		ID:          uuid.New(),
		TaskID:      task.ID,
		TaskName:    task.Name,
		TriggerType: triggerType,
		TriggerTime: triggerTime,
		Status:      models.StatusPending,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.CreateExecution(exec); err != nil {
		return err
	}

	return s.redisClient.ReadyQueueAdd(taskID, task.Priority)
}

func (s *Scheduler) checkDependencies(task *models.Task) bool {
	if len(task.Dependencies) == 0 {
		return true
	}

	allLastExecs := make([]*models.ExecutionHistory, 0, len(task.Dependencies))
	for _, depName := range task.Dependencies {
		depTask, err := s.repo.GetTaskByName(depName)
		if err != nil {
			return false
		}
		lastExec, err := s.repo.GetTaskLastExecution(depTask.ID)
		if err != nil {
			return false
		}
		allLastExecs = append(allLastExecs, lastExec)
	}

	switch task.TriggerCondition {
	case models.CondAllSuccess:
		for _, exec := range allLastExecs {
			if exec == nil || exec.Status != models.StatusSuccess {
				return false
			}
		}
		return true
	case models.CondAnySuccess:
		for _, exec := range allLastExecs {
			if exec != nil && exec.Status == models.StatusSuccess {
				return true
			}
		}
		return false
	case models.CondAnyComplete:
		for _, exec := range allLastExecs {
			if exec == nil {
				return false
			}
			if exec.Status == models.StatusPending || exec.Status == models.StatusRunning {
				return false
			}
		}
		return true
	default:
		for _, exec := range allLastExecs {
			if exec == nil || exec.Status != models.StatusSuccess {
				return false
			}
		}
		return true
	}
}

func (s *Scheduler) workerLoop() {
	defer s.wg.Done()

	batchSize := s.maxConcurrent / 2
	if batchSize < 1 {
		batchSize = 1
	}

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		taskIDs, err := s.redisClient.ReadyQueuePop(batchSize)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		if len(taskIDs) == 0 {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for _, taskID := range taskIDs {
			s.executeTask(taskID)
		}
	}
}

func (s *Scheduler) executeTask(taskID string) {
	taskUUID, err := uuid.Parse(taskID)
	if err != nil {
		return
	}

	task, err := s.repo.GetTaskByID(taskUUID)
	if err != nil {
		return
	}

	exec, err := s.getEarliestPendingExecution(taskUUID)
	if err != nil {
		return
	}
	if exec == nil {
		return
	}

	exists, err := s.redisClient.RunningTaskExists(taskID)
	if err == nil && exists {
		return
	}

	s.executingTasks.Store(taskID, true)
	defer s.executingTasks.Delete(taskID)

	ttl := time.Duration(task.TimeoutSec+300) * time.Second
	if err := s.redisClient.RunningTaskAdd(taskID, ttl); err != nil {
		return
	}
	defer s.redisClient.RunningTaskRemove(taskID)

	now := time.Now()
	exec.Status = models.StatusRunning
	exec.StartTime = &now
	if err := s.repo.UpdateExecution(exec); err != nil {
		return
	}

	task.LastRunAt = &now
	_ = s.repo.UpdateTask(task)

	exitCode, stdout, stderr, timedOut, runErr := s.runCommand(task, exec)

	endTime := time.Now()
	durationMs := endTime.Sub(now).Milliseconds()
	exec.EndTime = &endTime
	exec.DurationMs = &durationMs
	exec.Stdout = stdout
	exec.Stderr = stderr

	if runErr != nil && exec.ErrorMessage == "" {
		exec.ErrorMessage = runErr.Error()
	}

	if timedOut {
		exec.Status = models.StatusTimeout
		exec.ExitCode = nil
		if exec.ErrorMessage == "" {
			exec.ErrorMessage = "执行超时"
		}
	} else if exitCode == 0 {
		exec.Status = models.StatusSuccess
		code := 0
		exec.ExitCode = &code
	} else {
		exec.Status = models.StatusFailed
		code := exitCode
		exec.ExitCode = &code
	}

	if exec.Status == models.StatusSuccess {
		s.alerter.OnTaskSuccess(task.Name)
	}

	_ = s.repo.UpdateExecution(exec)

	if (exec.Status == models.StatusFailed || exec.Status == models.StatusTimeout) && exec.RetryCount < task.MaxRetries {
		exec.RetryCount++
		retryInterval := s.calculateRetryInterval(task, exec.RetryCount)
		retryExec := &models.ExecutionHistory{
			ID:           uuid.New(),
			TaskID:       task.ID,
			TaskName:     task.Name,
			TriggerType:  exec.TriggerType,
			TriggerTime:  time.Now().Add(retryInterval),
			Status:       models.StatusPending,
			RetryCount:   exec.RetryCount,
			ErrorMessage: exec.ErrorMessage,
			CreatedAt:    time.Now(),
		}
		_ = s.repo.CreateExecution(retryExec)

		time.AfterFunc(retryInterval, func() {
			_ = s.redisClient.ReadyQueueAdd(taskID, task.Priority)
		})
	} else if exec.Status == models.StatusFailed || exec.Status == models.StatusTimeout {
		_ = s.alerter.CheckAndAlert(task, exec)
	}

	cronExpr, err := cronparser.Parse(task.CronExpr)
	if err == nil {
		nextTimes := cronExpr.NextN(time.Now(), 1)
		if len(nextTimes) > 0 {
			task.NextRunAt = &nextTimes[0]
			_ = s.repo.UpdateTask(task)
		}
	}
}

func (s *Scheduler) getEarliestPendingExecution(taskID uuid.UUID) (*models.ExecutionHistory, error) {
	task, err := s.repo.GetTaskByID(taskID)
	if err != nil {
		return nil, err
	}

	execs, _, err := s.repo.ListExecutions(task.Name, time.Time{}, time.Time{}, 100, 0)
	if err != nil {
		return nil, err
	}

	var earliest *models.ExecutionHistory
	for i := range execs {
		e := &execs[i]
		if e.Status != models.StatusPending {
			continue
		}
		if earliest == nil || e.TriggerTime.Before(earliest.TriggerTime) {
			earliest = e
		}
	}
	return earliest, nil
}

func (s *Scheduler) runCommand(task *models.Task, execution *models.ExecutionHistory) (exitCode int, stdoutStr string, stderrStr string, timedOut bool, err error) {
	var shellName, shellArg string
	if runtime.GOOS == "windows" {
		shellName = "cmd"
		shellArg = "/c"
	} else {
		shellName = "sh"
		shellArg = "-c"
	}

	timeout := time.Duration(task.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shellName, shellArg, task.Command)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	stdoutStr = stdoutBuf.String()
	stderrStr = stderrBuf.String()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	if ctx.Err() == context.DeadlineExceeded {
		timedOut = true
		err = ctx.Err()
	}

	_ = execution
	return exitCode, stdoutStr, stderrStr, timedOut, err
}

func (s *Scheduler) TriggerTask(taskID uuid.UUID, taskName string, triggerType string) (*models.ExecutionHistory, error) {
	task, err := s.repo.GetTaskByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}

	now := time.Now()
	if err := s.checkDependenciesAndEnqueue(task, now, triggerType); err != nil {
		return nil, err
	}

	exec, err := s.getEarliestPendingExecution(taskID)
	if err != nil {
		return nil, err
	}
	return exec, nil
}

func (s *Scheduler) calculateRetryInterval(task *models.Task, retryCount int) time.Duration {
	baseSec := task.RetryIntervalSec
	if baseSec <= 0 {
		baseSec = 60
	}

	switch task.RetryStrategy {
	case models.RetryExponential:
		secs := baseSec * (1 << retryCount)
		return time.Duration(secs) * time.Second
	case models.RetryFixed:
		fallthrough
	default:
		return time.Duration(baseSec) * time.Second
	}
}

func (s *Scheduler) markRunningTasksInterrupted() error {
	execs, err := s.repo.GetRunningExecutions()
	if err != nil {
		return err
	}

	now := time.Now()
	for i := range execs {
		exec := &execs[i]
		exec.Status = models.StatusInterrupted
		exec.EndTime = &now
		if exec.ErrorMessage == "" {
			exec.ErrorMessage = "服务重启,任务被中断"
		}
		_ = s.repo.UpdateExecution(exec)
	}
	return nil
}

func (s *Scheduler) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			_, _ = s.repo.CleanOldHistory(90)
		}
	}
}

func matchesCurrentSecond(cronExpr *cronparser.CronExpression, t time.Time) bool {
	t = t.Truncate(time.Second)
	next := cronExpr.NextN(t.Add(-time.Second), 1)
	if len(next) > 0 && next[0].Equal(t) {
		return true
	}
	return false
}

var _ = strconv.Itoa
