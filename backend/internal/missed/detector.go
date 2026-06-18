package missed

import (
	"cron-scheduler/internal/cronparser"
	"cron-scheduler/internal/models"
	"cron-scheduler/internal/repository"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SchedulerInterface interface {
	TriggerTask(taskID uuid.UUID, taskName string, triggerType string) (*models.ExecutionHistory, error)
}

type MissedDetector struct {
	repo             *repository.Repository
	maxLookbackDays  int
	mu               sync.Mutex
}

func NewMissedDetector(repo *repository.Repository) *MissedDetector {
	return &MissedDetector{
		repo:            repo,
		maxLookbackDays: 7,
	}
}

func (md *MissedDetector) DetectAllMissed() ([]models.MissedExecution, error) {
	md.mu.Lock()
	defer md.mu.Unlock()

	enabled := true
	tasks, err := md.repo.ListTasks(&enabled, "")
	if err != nil {
		return nil, fmt.Errorf("获取启用任务列表失败: %w", err)
	}

	var allMissed []models.MissedExecution
	for i := range tasks {
		missed, err := md.DetectMissedForTask(&tasks[i])
		if err != nil {
			continue
		}
		allMissed = append(allMissed, missed...)
	}

	return allMissed, nil
}

func (md *MissedDetector) DetectMissedForTask(task *models.Task) ([]models.MissedExecution, error) {
	cronExpr, err := cronparser.Parse(task.CronExpr)
	if err != nil {
		return nil, fmt.Errorf("解析Cron表达式失败: %w", err)
	}

	now := time.Now()
	var from time.Time
	if task.LastRunAt != nil {
		from = *task.LastRunAt
	} else {
		from = now.AddDate(0, 0, -md.maxLookbackDays)
	}

	scheduledTimes := cronExpr.NextN(from, 1000)

	var newMissed []models.MissedExecution
	for _, scheduledTime := range scheduledTimes {
		if !scheduledTime.After(from) || !scheduledTime.Before(now) {
			continue
		}

		exists, err := md.repo.ExecutionExistsNear(task.ID, scheduledTime, 1)
		if err != nil {
			continue
		}
		if exists {
			continue
		}

		alreadyMissed, err := md.repo.MissedExecutionExists(task.ID, scheduledTime, 1)
		if err != nil {
			continue
		}
		if alreadyMissed {
			continue
		}

		missed := models.MissedExecution{
			TaskID:        task.ID,
			TaskName:      task.Name,
			ScheduledTime: scheduledTime,
			DetectedAt:    now,
			Compensation:  task.Compensation,
			Compensated:   false,
		}

		err = md.repo.CreateMissed(&missed)
		if err != nil {
			continue
		}
		newMissed = append(newMissed, missed)
	}

	return newMissed, nil
}

func (md *MissedDetector) ProcessCompensation(missed *models.MissedExecution, scheduler SchedulerInterface) error {
	md.mu.Lock()
	defer md.mu.Unlock()

	now := time.Now()

	switch missed.Compensation {
	case models.CompSkip:
		missed.Compensated = true
		missed.CompensatedAt = &now
	case models.CompExecuteOnce:
		_, err := scheduler.TriggerTask(missed.TaskID, missed.TaskName, models.TriggerCompensation)
		if err != nil {
			return fmt.Errorf("触发补偿执行失败: %w", err)
		}
		missed.Compensated = true
		missed.CompensatedAt = &now
	case models.CompQueue:
		_, err := scheduler.TriggerTask(missed.TaskID, missed.TaskName, models.TriggerCompensation)
		if err != nil {
			return fmt.Errorf("放入补偿队列失败: %w", err)
		}
		missed.Compensated = true
		missed.CompensatedAt = &now
	default:
		missed.Compensated = true
		missed.CompensatedAt = &now
	}

	err := md.repo.MarkMissedCompensated(missed.ID)
	if err != nil {
		return fmt.Errorf("更新补偿记录失败: %w", err)
	}

	return nil
}
