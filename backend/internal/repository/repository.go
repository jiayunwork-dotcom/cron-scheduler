package repository

import (
	"cron-scheduler/internal/models"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateTask(task *models.Task) error {
	var allTasks []models.Task
	if err := r.db.Find(&allTasks).Error; err != nil {
		return fmt.Errorf("获取所有任务失败: %w", err)
	}
	if err := r.ValidateDAG(task, allTasks); err != nil {
		return err
	}
	if err := r.db.Create(task).Error; err != nil {
		return fmt.Errorf("创建任务失败: %w", err)
	}
	return nil
}

func (r *Repository) UpdateTask(task *models.Task) error {
	var allTasks []models.Task
	if err := r.db.Where("id != ?", task.ID).Find(&allTasks).Error; err != nil {
		return fmt.Errorf("获取所有任务失败: %w", err)
	}
	if err := r.ValidateDAG(task, allTasks); err != nil {
		return err
	}
	if err := r.db.Save(task).Error; err != nil {
		return fmt.Errorf("更新任务失败: %w", err)
	}
	return nil
}

func (r *Repository) GetTaskByName(name string) (*models.Task, error) {
	var task models.Task
	if err := r.db.Where("name = ?", name).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("任务不存在: %s", name)
		}
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}
	return &task, nil
}

func (r *Repository) GetTaskByID(id uuid.UUID) (*models.Task, error) {
	var task models.Task
	if err := r.db.Where("id = ?", id).First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("任务不存在: %s", id)
		}
		return nil, fmt.Errorf("获取任务失败: %w", err)
	}
	return &task, nil
}

func (r *Repository) ListTasks(enabled *bool, tag string) ([]models.Task, error) {
	var tasks []models.Task
	query := r.db
	if enabled != nil {
		query = query.Where("enabled = ?", *enabled)
	}
	if tag != "" {
		query = query.Where("tags @> ?::text[]", fmt.Sprintf("{%s}", tag))
	}
	if err := query.Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("获取任务列表失败: %w", err)
	}
	return tasks, nil
}

func (r *Repository) DeleteTask(name string) error {
	result := r.db.Where("name = ?", name).Delete(&models.Task{})
	if result.Error != nil {
		return fmt.Errorf("删除任务失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("任务不存在: %s", name)
	}
	return nil
}

func (r *Repository) ListAllTaskNames() ([]string, error) {
	var names []string
	if err := r.db.Model(&models.Task{}).Pluck("name", &names).Error; err != nil {
		return nil, fmt.Errorf("获取任务名称列表失败: %w", err)
	}
	return names, nil
}

func (r *Repository) GetTaskLastExecution(taskID uuid.UUID) (*models.ExecutionHistory, error) {
	var exec models.ExecutionHistory
	if err := r.db.Where("task_id = ?", taskID).Order("trigger_time DESC").First(&exec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("获取任务最近执行记录失败: %w", err)
	}
	return &exec, nil
}

func (r *Repository) ValidateDAG(task *models.Task, allTasks []models.Task) error {
	taskMap := make(map[string]models.Task)
	for _, t := range allTasks {
		taskMap[t.Name] = t
	}
	taskMap[task.Name] = *task

	for _, dep := range task.Dependencies {
		if dep == task.Name {
			return fmt.Errorf("任务不能依赖自身: %s", dep)
		}
		if _, exists := taskMap[dep]; !exists {
			return fmt.Errorf("依赖任务不存在: %s", dep)
		}
	}

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(name string) bool
	dfs = func(name string) bool {
		visited[name] = true
		recStack[name] = true

		t, ok := taskMap[name]
		if !ok {
			return false
		}

		for _, dep := range t.Dependencies {
			if !visited[dep] {
				if dfs(dep) {
					return true
				}
			} else if recStack[dep] {
				return true
			}
		}

		recStack[name] = false
		return false
	}

	for name := range taskMap {
		if !visited[name] {
			if dfs(name) {
				return fmt.Errorf("检测到循环依赖")
			}
		}
	}

	return nil
}

func (r *Repository) CreateExecution(exec *models.ExecutionHistory) error {
	if err := r.db.Create(exec).Error; err != nil {
		return fmt.Errorf("创建执行记录失败: %w", err)
	}
	return nil
}

func (r *Repository) UpdateExecution(exec *models.ExecutionHistory) error {
	if err := r.db.Save(exec).Error; err != nil {
		return fmt.Errorf("更新执行记录失败: %w", err)
	}
	return nil
}

func (r *Repository) GetExecution(id uuid.UUID) (*models.ExecutionHistory, error) {
	var exec models.ExecutionHistory
	if err := r.db.Where("id = ?", id).First(&exec).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("执行记录不存在: %s", id)
		}
		return nil, fmt.Errorf("获取执行记录失败: %w", err)
	}
	return &exec, nil
}

func (r *Repository) ListExecutions(taskName string, startTime, endTime time.Time, limit, offset int) ([]models.ExecutionHistory, int64, error) {
	var execs []models.ExecutionHistory
	var total int64

	query := r.db.Model(&models.ExecutionHistory{})
	if taskName != "" {
		query = query.Where("task_name = ?", taskName)
	}
	if !startTime.IsZero() {
		query = query.Where("trigger_time >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("trigger_time <= ?", endTime)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取执行记录总数失败: %w", err)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Order("trigger_time DESC").Find(&execs).Error; err != nil {
		return nil, 0, fmt.Errorf("获取执行记录列表失败: %w", err)
	}

	return execs, total, nil
}

func (r *Repository) GetRunningExecutions() ([]models.ExecutionHistory, error) {
	var execs []models.ExecutionHistory
	if err := r.db.Where("status = ?", models.StatusRunning).Find(&execs).Error; err != nil {
		return nil, fmt.Errorf("获取运行中执行记录失败: %w", err)
	}
	return execs, nil
}

func (r *Repository) CreateAlert(alert *models.Alert) error {
	if err := r.db.Create(alert).Error; err != nil {
		return fmt.Errorf("创建告警失败: %w", err)
	}
	return nil
}

func (r *Repository) ListAlerts(limit, offset int) ([]models.Alert, int64, error) {
	var alerts []models.Alert
	var total int64

	query := r.db.Model(&models.Alert{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取告警总数失败: %w", err)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	if err := query.Order("created_at DESC").Find(&alerts).Error; err != nil {
		return nil, 0, fmt.Errorf("获取告警列表失败: %w", err)
	}

	return alerts, total, nil
}

func (r *Repository) MarkAlertSent(id uuid.UUID, errMsg string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"sent":          true,
		"sent_at":       &now,
		"error_message": errMsg,
	}
	result := r.db.Model(&models.Alert{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("标记告警已发送失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("告警不存在: %s", id)
	}
	return nil
}

func (r *Repository) GetAllSettings() (map[string]string, error) {
	var settings []models.SystemSetting
	if err := r.db.Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("获取系统设置失败: %w", err)
	}
	result := make(map[string]string)
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

func (r *Repository) UpdateSettings(settings map[string]string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		for key, value := range settings {
			setting := models.SystemSetting{
				Key:       key,
				Value:     value,
				UpdatedAt: time.Now(),
			}
			if err := tx.Save(&setting).Error; err != nil {
				return fmt.Errorf("更新系统设置失败: %w", err)
			}
		}
		return nil
	})
}

func (r *Repository) GetSetting(key, defaultValue string) string {
	var setting models.SystemSetting
	if err := r.db.Where("key = ?", key).First(&setting).Error; err != nil {
		return defaultValue
	}
	return setting.Value
}

func (r *Repository) CreateMissed(m *models.MissedExecution) error {
	if err := r.db.Create(m).Error; err != nil {
		return fmt.Errorf("创建错过执行记录失败: %w", err)
	}
	return nil
}

func (r *Repository) ListMissedByTask(taskID uuid.UUID) ([]models.MissedExecution, error) {
	var missed []models.MissedExecution
	if err := r.db.Where("task_id = ?", taskID).Order("scheduled_time DESC").Find(&missed).Error; err != nil {
		return nil, fmt.Errorf("获取任务错过执行记录失败: %w", err)
	}
	return missed, nil
}

func (r *Repository) MarkMissedCompensated(id uuid.UUID) error {
	now := time.Now()
	updates := map[string]interface{}{
		"compensated":    true,
		"compensated_at": &now,
	}
	result := r.db.Model(&models.MissedExecution{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("标记错过执行已补偿失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("错过执行记录不存在: %s", id)
	}
	return nil
}

func (r *Repository) ListUncompensatedMissed() ([]models.MissedExecution, error) {
	var missed []models.MissedExecution
	if err := r.db.Where("compensated = ?", false).Order("scheduled_time ASC").Find(&missed).Error; err != nil {
		return nil, fmt.Errorf("获取未补偿错过执行记录失败: %w", err)
	}
	return missed, nil
}

func (r *Repository) CleanOldHistory(days int) (int64, error) {
	cutoffTime := time.Now().AddDate(0, 0, -days)
	result := r.db.Where("created_at < ?", cutoffTime).Delete(&models.ExecutionHistory{})
	if result.Error != nil {
		return 0, fmt.Errorf("清理旧执行记录失败: %w", result.Error)
	}
	return result.RowsAffected, nil
}

func (r *Repository) ExecutionExistsNear(taskID uuid.UUID, scheduledTime time.Time, toleranceSec int) (bool, error) {
	start := scheduledTime.Add(-time.Duration(toleranceSec) * time.Second)
	end := scheduledTime.Add(time.Duration(toleranceSec) * time.Second)
	var count int64
	err := r.db.Model(&models.ExecutionHistory{}).
		Where("task_id = ? AND trigger_time >= ? AND trigger_time <= ?", taskID, start, end).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查执行记录是否存在失败: %w", err)
	}
	return count > 0, nil
}

func (r *Repository) MissedExecutionExists(taskID uuid.UUID, scheduledTime time.Time, toleranceSec int) (bool, error) {
	start := scheduledTime.Add(-time.Duration(toleranceSec) * time.Second)
	end := scheduledTime.Add(time.Duration(toleranceSec) * time.Second)
	var count int64
	err := r.db.Model(&models.MissedExecution{}).
		Where("task_id = ? AND scheduled_time >= ? AND scheduled_time <= ?", taskID, start, end).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("检查错过执行记录是否存在失败: %w", err)
	}
	return count > 0, nil
}

func (r *Repository) GetRecentCompletedExecutions(limit int) ([]models.ExecutionHistory, error) {
	var execs []models.ExecutionHistory
	if limit <= 0 {
		limit = 50
	}
	if err := r.db.Where("status != ?", models.StatusRunning).
		Order("end_time DESC NULLS LAST, trigger_time DESC").
		Limit(limit).
		Find(&execs).Error; err != nil {
		return nil, fmt.Errorf("获取最近完成执行记录失败: %w", err)
	}
	return execs, nil
}

func (r *Repository) ListExecutionsForCalendar(startDate, endDate time.Time) ([]models.ExecutionHistory, error) {
	var execs []models.ExecutionHistory
	query := r.db.Where("status != ?", models.StatusPending)
	if !startDate.IsZero() {
		query = query.Where("start_time >= ? OR (start_time IS NULL AND trigger_time >= ?)", startDate, startDate)
	}
	if !endDate.IsZero() {
		query = query.Where("start_time < ? OR (start_time IS NULL AND trigger_time < ?)", endDate.AddDate(0, 0, 1), endDate.AddDate(0, 0, 1))
	}
	if err := query.Order("start_time ASC NULLS LAST, trigger_time ASC").Find(&execs).Error; err != nil {
		return nil, fmt.Errorf("获取日历执行记录失败: %w", err)
	}
	return execs, nil
}
