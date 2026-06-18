package api

import (
	"cron-scheduler/internal/alerter"
	"cron-scheduler/internal/cronparser"
	"cron-scheduler/internal/missed"
	"cron-scheduler/internal/models"
	"cron-scheduler/internal/repository"
	"cron-scheduler/internal/scheduler"
	"cron-scheduler/internal/ws"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	repo           *repository.Repository
	scheduler      *scheduler.Scheduler
	missedDetector *missed.MissedDetector
	alerter        *alerter.Alerter
	wsHub          *ws.Hub
}

func NewHandler(repo *repository.Repository, sched *scheduler.Scheduler, md *missed.MissedDetector, al *alerter.Alerter, wsHub *ws.Hub) *Handler {
	return &Handler{
		repo:           repo,
		scheduler:      sched,
		missedDetector: md,
		alerter:        al,
		wsHub:          wsHub,
	}
}

func SetupRouter(handler *Handler) *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	api := r.Group("/api")
	{
		api.GET("/tasks", handler.ListTasksHandler)
		api.GET("/tasks/:name", handler.GetTaskHandler)
		api.POST("/tasks", handler.CreateTaskHandler)
		api.PUT("/tasks/:name", handler.UpdateTaskHandler)
		api.DELETE("/tasks/:name", handler.DeleteTaskHandler)
		api.POST("/tasks/:name/trigger", handler.TriggerTaskHandler)
		api.POST("/tasks/:name/enable", handler.EnableTaskHandler)
		api.POST("/tasks/:name/disable", handler.DisableTaskHandler)
		api.POST("/tasks/batch/enable", handler.BatchEnableTasksHandler)
		api.POST("/tasks/batch/disable", handler.BatchDisableTasksHandler)
		api.POST("/tasks/batch/delete", handler.BatchDeleteTasksHandler)

		api.POST("/cron/preview", handler.CronPreviewHandler)

		api.GET("/dag", handler.GetDAGHandler)

		api.GET("/executions", handler.ListExecutionsHandler)
		api.GET("/executions/history", handler.GetExecutionHistoryHandler)
		api.GET("/executions/:id", handler.GetExecutionHandler)
		api.GET("/executions/:id/detail", handler.GetExecutionDetailHandler)

		api.GET("/alerts", handler.ListAlertsHandler)

		api.GET("/settings", handler.GetSettingsHandler)
		api.POST("/settings", handler.UpdateSettingsHandler)
		api.POST("/settings/webhook/test", handler.TestWebhookHandler)

		api.GET("/missed", handler.ListMissedHandler)
		api.POST("/missed/detect", handler.DetectMissedHandler)

		api.GET("/health", handler.HealthHandler)
		api.GET("/executions/running", handler.ListRunningExecutionsHandler)
	}

	r.GET("/ws/executions", ws.ServeWS(handler.wsHub))

	return r
}

func taskToResponse(task *models.Task, repo *repository.Repository) (models.TaskResponse, error) {
	resp := models.TaskResponse{
		ID:               task.ID,
		Name:             task.Name,
		CronExpr:         task.CronExpr,
		Command:          task.Command,
		TimeoutSec:       task.TimeoutSec,
		TimeoutStrategy:  task.TimeoutStrategy,
		MaxRetries:       task.MaxRetries,
		RetryStrategy:    task.RetryStrategy,
		RetryIntervalSec: task.RetryIntervalSec,
		Priority:         task.Priority,
		Dependencies:     task.Dependencies,
		TriggerCondition: task.TriggerCondition,
		Enabled:          task.Enabled,
		Tags:             task.Tags,
		Compensation:     task.Compensation,
		AlertEnabled:     task.AlertEnabled,
		LastRunAt:        task.LastRunAt,
		NextRunAt:        task.NextRunAt,
		CreatedAt:        task.CreatedAt,
		UpdatedAt:        task.UpdatedAt,
		LastResult:       "",
		NextTriggerTimes: []time.Time{},
	}

	lastExec, err := repo.GetTaskLastExecution(task.ID)
	if err == nil && lastExec != nil {
		resp.LastResult = lastExec.Status
	}

	cronExpr, err := cronparser.Parse(task.CronExpr)
	if err == nil {
		resp.NextTriggerTimes = cronExpr.NextN(time.Now(), 5)
	}

	return resp, nil
}

func (h *Handler) ListTasksHandler(c *gin.Context) {
	var enabled *bool
	if enabledStr := c.Query("enabled"); enabledStr != "" {
		if b, err := strconv.ParseBool(enabledStr); err == nil {
			enabled = &b
		}
	}
	tag := c.Query("tag")

	tasks, err := h.repo.ListTasks(enabled, tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	responses := make([]models.TaskResponse, 0, len(tasks))
	for i := range tasks {
		resp, err := taskToResponse(&tasks[i], h.repo)
		if err != nil {
			continue
		}
		responses = append(responses, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responses,
		"message": "ok",
	})
}

func (h *Handler) GetTaskHandler(c *gin.Context) {
	name := c.Param("name")

	task, err := h.repo.GetTaskByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp, err := taskToResponse(task, h.repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) CreateTaskHandler(c *gin.Context) {
	var req models.CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.CronExpr = strings.TrimSpace(req.CronExpr)

	if err := cronparser.Validate(req.CronExpr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("Cron表达式错误: %v", err),
		})
		return
	}

	timeoutStrategy := req.TimeoutStrategy
	if timeoutStrategy == "" {
		timeoutStrategy = models.TimeoutKillAndFail
	}

	task := &models.Task{
		ID:               uuid.New(),
		Name:             req.Name,
		CronExpr:         req.CronExpr,
		Command:          req.Command,
		TimeoutSec:       req.TimeoutSec,
		TimeoutStrategy:  timeoutStrategy,
		MaxRetries:       req.MaxRetries,
		RetryStrategy:    req.RetryStrategy,
		RetryIntervalSec: req.RetryIntervalSec,
		Priority:         req.Priority,
		Dependencies:     req.Dependencies,
		TriggerCondition: req.TriggerCondition,
		Enabled:          req.Enabled,
		Tags:             req.Tags,
		Compensation:     req.Compensation,
		AlertEnabled:     req.AlertEnabled,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := h.repo.CreateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp, err := taskToResponse(task, h.repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) UpdateTaskHandler(c *gin.Context) {
	name := c.Param("name")

	var req models.UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.CronExpr = strings.TrimSpace(req.CronExpr)

	task, err := h.repo.GetTaskByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	if req.CronExpr != "" {
		if err := cronparser.Validate(req.CronExpr); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"data":    nil,
				"message": fmt.Sprintf("Cron表达式错误: %v", err),
			})
			return
		}
		task.CronExpr = req.CronExpr
	}

	if req.Name != "" {
		task.Name = req.Name
	}
	if req.Command != "" {
		task.Command = req.Command
	}
	if req.TimeoutSec > 0 {
		task.TimeoutSec = req.TimeoutSec
	}
	if req.TimeoutStrategy != "" {
		task.TimeoutStrategy = req.TimeoutStrategy
	}
	if req.MaxRetries >= 0 {
		task.MaxRetries = req.MaxRetries
	}
	if req.RetryStrategy != "" {
		task.RetryStrategy = req.RetryStrategy
	}
	if req.RetryIntervalSec > 0 {
		task.RetryIntervalSec = req.RetryIntervalSec
	}
	if req.Priority > 0 {
		task.Priority = req.Priority
	}
	if len(req.Dependencies) > 0 {
		task.Dependencies = req.Dependencies
	}
	if req.TriggerCondition != "" {
		task.TriggerCondition = req.TriggerCondition
	}
	task.Enabled = req.Enabled
	if len(req.Tags) > 0 {
		task.Tags = req.Tags
	}
	if req.Compensation != "" {
		task.Compensation = req.Compensation
	}
	task.AlertEnabled = req.AlertEnabled
	task.UpdatedAt = time.Now()

	if err := h.repo.UpdateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp, err := taskToResponse(task, h.repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) DeleteTaskHandler(c *gin.Context) {
	name := c.Param("name")

	if err := h.repo.DeleteTask(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nil,
		"message": "ok",
	})
}

func (h *Handler) TriggerTaskHandler(c *gin.Context) {
	name := c.Param("name")

	task, err := h.repo.GetTaskByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	exec, err := h.scheduler.TriggerTask(task.ID, task.Name, models.TriggerManual)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    exec,
		"message": "ok",
	})
}

func (h *Handler) EnableTaskHandler(c *gin.Context) {
	name := c.Param("name")

	task, err := h.repo.GetTaskByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	task.Enabled = true
	task.UpdatedAt = time.Now()

	if err := h.repo.UpdateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp, err := taskToResponse(task, h.repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) DisableTaskHandler(c *gin.Context) {
	name := c.Param("name")

	task, err := h.repo.GetTaskByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	task.Enabled = false
	task.UpdatedAt = time.Now()

	if err := h.repo.UpdateTask(task); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp, err := taskToResponse(task, h.repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) CronPreviewHandler(c *gin.Context) {
	var req models.CronPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	req.Expr = strings.TrimSpace(req.Expr)

	count := req.Count
	if count <= 0 {
		count = 5
	}

	resp := models.CronPreviewResponse{
		Valid: false,
		Error: "",
		Times: []time.Time{},
	}

	times, err := cronparser.PreviewNext(req.Expr, count)
	if err != nil {
		resp.Error = err.Error()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    resp,
			"message": "ok",
		})
		return
	}

	resp.Valid = true
	resp.Times = times

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func getStatusColor(status string) string {
	switch status {
	case models.StatusSuccess:
		return "#10b981"
	case models.StatusFailed, models.StatusTimeout:
		return "#ef4444"
	case models.StatusRunning:
		return "#3b82f6"
	case models.StatusPending:
		return "#f59e0b"
	default:
		return "#9ca3af"
	}
}

func (h *Handler) GetDAGHandler(c *gin.Context) {
	tasks, err := h.repo.ListTasks(nil, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	nodes := make([]models.DAGNode, 0, len(tasks))
	edges := make([]models.DAGEdge, 0)

	for i := range tasks {
		task := &tasks[i]
		status := ""
		lastExec, err := h.repo.GetTaskLastExecution(task.ID)
		if err == nil && lastExec != nil {
			status = lastExec.Status
		}

		nodes = append(nodes, models.DAGNode{
			ID:     task.Name,
			Name:   task.Name,
			Status: status,
			Color:  getStatusColor(status),
		})

		for _, dep := range task.Dependencies {
			edges = append(edges, models.DAGEdge{
				Source: dep,
				Target: task.Name,
			})
		}
	}

	dagResp := models.DAGResponse{
		Nodes: nodes,
		Edges: edges,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    dagResp,
		"message": "ok",
	})
}

func (h *Handler) ListExecutionsHandler(c *gin.Context) {
	taskName := c.Query("task_name")
	startTimeStr := c.Query("start_time")
	endTimeStr := c.Query("end_time")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 20
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	var startTime, endTime time.Time
	if startTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, startTimeStr); err == nil {
			startTime = t
		}
	}
	if endTimeStr != "" {
		if t, err := time.Parse(time.RFC3339, endTimeStr); err == nil {
			endTime = t
		}
	}

	offset := (page - 1) * pageSize

	execs, total, err := h.repo.ListExecutions(taskName, startTime, endTime, pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	items := make([]models.ExecutionResponse, 0, len(execs))
	for i := range execs {
		items = append(items, models.ExecutionResponse{
			ID:           execs[i].ID,
			TaskID:       execs[i].TaskID,
			TaskName:     execs[i].TaskName,
			TriggerType:  execs[i].TriggerType,
			TriggerTime:  execs[i].TriggerTime,
			StartTime:    execs[i].StartTime,
			EndTime:      execs[i].EndTime,
			DurationMs:   execs[i].DurationMs,
			Status:       execs[i].Status,
			ExitCode:     execs[i].ExitCode,
			Stdout:       execs[i].Stdout,
			Stderr:       execs[i].Stderr,
			RetryCount:   execs[i].RetryCount,
			ErrorMessage: execs[i].ErrorMessage,
			CreatedAt:    execs[i].CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"message": "ok",
	})
}

func (h *Handler) GetExecutionHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "无效的执行记录ID",
		})
		return
	}

	exec, err := h.repo.GetExecution(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp := models.ExecutionResponse{
		ID:           exec.ID,
		TaskID:       exec.TaskID,
		TaskName:     exec.TaskName,
		TriggerType:  exec.TriggerType,
		TriggerTime:  exec.TriggerTime,
		StartTime:    exec.StartTime,
		EndTime:      exec.EndTime,
		DurationMs:   exec.DurationMs,
		Status:       exec.Status,
		ExitCode:     exec.ExitCode,
		Stdout:       exec.Stdout,
		Stderr:       exec.Stderr,
		RetryCount:   exec.RetryCount,
		ErrorMessage: exec.ErrorMessage,
		CreatedAt:    exec.CreatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) ListAlertsHandler(c *gin.Context) {
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")

	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 20
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	offset := (page - 1) * pageSize

	alerts, total, err := h.repo.ListAlerts(pageSize, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"items":     alerts,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
		"message": "ok",
	})
}

func (h *Handler) GetSettingsHandler(c *gin.Context) {
	settings, err := h.repo.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    settings,
		"message": "ok",
	})
}

func (h *Handler) UpdateSettingsHandler(c *gin.Context) {
	var req models.SystemSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	cleaned := make(models.SystemSettingsRequest)
	for k, v := range req {
		cleaned[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

	if err := h.repo.UpdateSettings(cleaned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	webhookURL := h.repo.GetSetting("alert_webhook_url", "")
	consecutiveFailuresStr := h.repo.GetSetting("consecutive_failures_for_alert", "1")
	silentMinutesStr := h.repo.GetSetting("alert_silent_minutes", "5")

	consecutiveFailures := 1
	if v, err := strconv.Atoi(consecutiveFailuresStr); err == nil && v > 0 {
		consecutiveFailures = v
	}

	silentMinutes := 5
	if v, err := strconv.Atoi(silentMinutesStr); err == nil && v > 0 {
		silentMinutes = v
	}

	h.alerter.UpdateConfig(webhookURL, consecutiveFailures, silentMinutes)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    nil,
		"message": "ok",
	})
}

func (h *Handler) ListMissedHandler(c *gin.Context) {
	taskName := c.Query("task_name")

	type MissedWithTask struct {
		models.MissedExecution
		Task *models.Task `json:"task,omitempty"`
	}

	var missedList []models.MissedExecution
	var err error
	var task *models.Task

	if taskName != "" {
		task, err = h.repo.GetTaskByName(taskName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"data":    nil,
				"message": err.Error(),
			})
			return
		}
		missedList, err = h.repo.ListMissedByTask(task.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"data":    nil,
				"message": err.Error(),
			})
			return
		}
	} else {
		tasks, err := h.repo.ListTasks(nil, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"data":    nil,
				"message": err.Error(),
			})
			return
		}
		for i := range tasks {
			taskMissed, err := h.repo.ListMissedByTask(tasks[i].ID)
			if err != nil {
				continue
			}
			missedList = append(missedList, taskMissed...)
		}
	}

	taskMap := make(map[string]*models.Task)
	if taskName != "" {
		task, _ := h.repo.GetTaskByName(taskName)
		if task != nil {
			taskMap[taskName] = task
		}
	} else {
		tasks, _ := h.repo.ListTasks(nil, "")
		for i := range tasks {
			taskMap[tasks[i].Name] = &tasks[i]
		}
	}

	result := make([]MissedWithTask, 0, len(missedList))
	for i := range missedList {
		m := missedList[i]
		item := MissedWithTask{
			MissedExecution: m,
			Task:            taskMap[m.TaskName],
		}
		item.TaskName = strings.TrimSpace(item.TaskName)
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "ok",
	})
}

func (h *Handler) DetectMissedHandler(c *gin.Context) {
	newMissed, err := h.missedDetector.DetectAllMissed()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	uncompensated, err := h.repo.ListUncompensatedMissed()
	if err == nil {
		for i := range uncompensated {
			_ = h.missedDetector.ProcessCompensation(&uncompensated[i], h.scheduler)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    newMissed,
		"message": "ok",
	})
}

func (h *Handler) HealthHandler(c *gin.Context) {
	status := gin.H{
		"status":  "ok",
		"db":      "ok",
		"redis":   "ok",
		"service": "running",
	}

	if _, err := h.repo.ListTasks(nil, ""); err != nil {
		status["db"] = "error"
		status["status"] = "degraded"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
		"message": "ok",
	})
}

func (h *Handler) BatchEnableTasksHandler(c *gin.Context) {
	var req models.BatchTaskNamesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	result := models.BatchOperationResult{
		SuccessCount: 0,
		FailedCount:  0,
		FailedTasks:  []string{},
	}

	now := time.Now()
	for _, name := range req.TaskNames {
		task, err := h.repo.GetTaskByName(name)
		if err != nil {
			result.FailedCount++
			result.FailedTasks = append(result.FailedTasks, name)
			continue
		}
		task.Enabled = true
		task.UpdatedAt = now
		if err := h.repo.UpdateTask(task); err != nil {
			result.FailedCount++
			result.FailedTasks = append(result.FailedTasks, name)
			continue
		}
		result.SuccessCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "ok",
	})
}

func (h *Handler) BatchDisableTasksHandler(c *gin.Context) {
	var req models.BatchTaskNamesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	result := models.BatchOperationResult{
		SuccessCount: 0,
		FailedCount:  0,
		FailedTasks:  []string{},
	}

	now := time.Now()
	for _, name := range req.TaskNames {
		task, err := h.repo.GetTaskByName(name)
		if err != nil {
			result.FailedCount++
			result.FailedTasks = append(result.FailedTasks, name)
			continue
		}
		task.Enabled = false
		task.UpdatedAt = now
		if err := h.repo.UpdateTask(task); err != nil {
			result.FailedCount++
			result.FailedTasks = append(result.FailedTasks, name)
			continue
		}
		result.SuccessCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "ok",
	})
}

func (h *Handler) BatchDeleteTasksHandler(c *gin.Context) {
	var req models.BatchTaskNamesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	result := models.BatchOperationResult{
		SuccessCount: 0,
		FailedCount:  0,
		FailedTasks:  []string{},
	}

	for _, name := range req.TaskNames {
		if err := h.repo.DeleteTask(name); err != nil {
			result.FailedCount++
			result.FailedTasks = append(result.FailedTasks, name)
			continue
		}
		result.SuccessCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "ok",
	})
}

func (h *Handler) TestWebhookHandler(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": fmt.Sprintf("请求参数错误: %v", err),
		})
		return
	}

	webhookURL := req["webhook_url"]
	if webhookURL == "" {
		webhookURL = h.repo.GetSetting("alert_webhook_url", "")
	}

	testResult := h.alerter.SendTestWebhook(webhookURL)

	resp := models.WebhookTestResponse{
		Success:    testResult.Success,
		StatusCode: testResult.StatusCode,
		DurationMs: testResult.DurationMs,
		Error:      testResult.Error,
	}
	if testResult.Success {
		resp.Message = "Webhook测试成功"
	} else {
		resp.Message = "Webhook测试失败"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}

func (h *Handler) ListRunningExecutionsHandler(c *gin.Context) {
	execs, err := h.repo.GetRunningExecutions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	items := make([]models.ExecutionResponse, 0, len(execs))
	for i := range execs {
		items = append(items, models.ExecutionResponse{
			ID:           execs[i].ID,
			TaskID:       execs[i].TaskID,
			TaskName:     execs[i].TaskName,
			TriggerType:  execs[i].TriggerType,
			TriggerTime:  execs[i].TriggerTime,
			StartTime:    execs[i].StartTime,
			EndTime:      execs[i].EndTime,
			DurationMs:   execs[i].DurationMs,
			Status:       execs[i].Status,
			ExitCode:     execs[i].ExitCode,
			Stdout:       execs[i].Stdout,
			Stderr:       execs[i].Stderr,
			RetryCount:   execs[i].RetryCount,
			ErrorMessage: execs[i].ErrorMessage,
			CreatedAt:    execs[i].CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"message": "ok",
	})
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func (h *Handler) GetExecutionHistoryHandler(c *gin.Context) {
	execs, err := h.repo.GetRecentCompletedExecutions(50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	items := make([]models.ExecutionResponse, 0, len(execs))
	for i := range execs {
		items = append(items, models.ExecutionResponse{
			ID:           execs[i].ID,
			TaskID:       execs[i].TaskID,
			TaskName:     execs[i].TaskName,
			TriggerType:  execs[i].TriggerType,
			TriggerTime:  execs[i].TriggerTime,
			StartTime:    execs[i].StartTime,
			EndTime:      execs[i].EndTime,
			DurationMs:   execs[i].DurationMs,
			Status:       execs[i].Status,
			ExitCode:     execs[i].ExitCode,
			Stdout:       truncateString(execs[i].Stdout, 500),
			Stderr:       truncateString(execs[i].Stderr, 500),
			RetryCount:   execs[i].RetryCount,
			ErrorMessage: execs[i].ErrorMessage,
			CreatedAt:    execs[i].CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"message": "ok",
	})
}

func (h *Handler) GetExecutionDetailHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"data":    nil,
			"message": "无效的执行记录ID",
		})
		return
	}

	exec, err := h.repo.GetExecution(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	resp := models.ExecutionResponse{
		ID:           exec.ID,
		TaskID:       exec.TaskID,
		TaskName:     exec.TaskName,
		TriggerType:  exec.TriggerType,
		TriggerTime:  exec.TriggerTime,
		StartTime:    exec.StartTime,
		EndTime:      exec.EndTime,
		DurationMs:   exec.DurationMs,
		Status:       exec.Status,
		ExitCode:     exec.ExitCode,
		Stdout:       exec.Stdout,
		Stderr:       exec.Stderr,
		RetryCount:   exec.RetryCount,
		ErrorMessage: exec.ErrorMessage,
		CreatedAt:    exec.CreatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    resp,
		"message": "ok",
	})
}
