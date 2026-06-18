package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type CreateTaskRequest struct {
	Name             string         `json:"name"`
	CronExpr         string         `json:"cron_expr"`
	Command          string         `json:"command"`
	TimeoutSec       int            `json:"timeout_sec"`
	TimeoutStrategy  string         `json:"timeout_strategy"`
	MaxRetries       int            `json:"max_retries"`
	RetryStrategy    string         `json:"retry_strategy"`
	RetryIntervalSec int            `json:"retry_interval_sec"`
	Priority         int            `json:"priority"`
	Dependencies     pq.StringArray `json:"dependencies"`
	TriggerCondition string         `json:"trigger_condition"`
	Enabled          bool           `json:"enabled"`
	Tags             pq.StringArray `json:"tags"`
	Compensation     string         `json:"compensation"`
	AlertEnabled     bool           `json:"alert_enabled"`
}

type UpdateTaskRequest struct {
	Name             string         `json:"name"`
	CronExpr         string         `json:"cron_expr"`
	Command          string         `json:"command"`
	TimeoutSec       int            `json:"timeout_sec"`
	TimeoutStrategy  string         `json:"timeout_strategy"`
	MaxRetries       int            `json:"max_retries"`
	RetryStrategy    string         `json:"retry_strategy"`
	RetryIntervalSec int            `json:"retry_interval_sec"`
	Priority         int            `json:"priority"`
	Dependencies     pq.StringArray `json:"dependencies"`
	TriggerCondition string         `json:"trigger_condition"`
	Enabled          bool           `json:"enabled"`
	Tags             pq.StringArray `json:"tags"`
	Compensation     string         `json:"compensation"`
	AlertEnabled     bool           `json:"alert_enabled"`
}

type TaskResponse struct {
	ID               uuid.UUID      `json:"id"`
	Name             string         `json:"name"`
	CronExpr         string         `json:"cron_expr"`
	Command          string         `json:"command"`
	TimeoutSec       int            `json:"timeout_sec"`
	TimeoutStrategy  string         `json:"timeout_strategy"`
	MaxRetries       int            `json:"max_retries"`
	RetryStrategy    string         `json:"retry_strategy"`
	RetryIntervalSec int            `json:"retry_interval_sec"`
	Priority         int            `json:"priority"`
	Dependencies     pq.StringArray `json:"dependencies"`
	TriggerCondition string         `json:"trigger_condition"`
	Enabled          bool           `json:"enabled"`
	Tags             pq.StringArray `json:"tags"`
	Compensation     string         `json:"compensation"`
	AlertEnabled     bool           `json:"alert_enabled"`
	LastRunAt        *time.Time     `json:"last_run_at"`
	NextRunAt        *time.Time     `json:"next_run_at"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	LastResult       string         `json:"last_result"`
	NextTriggerTimes []time.Time    `json:"next_trigger_times"`
}

type TriggerTaskRequest struct {
	TaskName string `json:"task_name"`
}

type ExecutionResponse struct {
	ID           uuid.UUID  `json:"id"`
	TaskID       uuid.UUID  `json:"task_id"`
	TaskName     string     `json:"task_name"`
	TriggerType  string     `json:"trigger_type"`
	TriggerTime  time.Time  `json:"trigger_time"`
	StartTime    *time.Time `json:"start_time"`
	EndTime      *time.Time `json:"end_time"`
	DurationMs   *int64     `json:"duration_ms"`
	Status       string     `json:"status"`
	ExitCode     *int       `json:"exit_code"`
	Stdout       string     `json:"stdout"`
	Stderr       string     `json:"stderr"`
	RetryCount   int        `json:"retry_count"`
	ErrorMessage string     `json:"error_message"`
	CreatedAt    time.Time  `json:"created_at"`
}

type SystemSettingsRequest map[string]string

type CronPreviewRequest struct {
	Expr  string `json:"expr"`
	Count int    `json:"count"`
}

type CronPreviewResponse struct {
	Valid bool        `json:"valid"`
	Error string      `json:"error"`
	Times []time.Time `json:"times"`
}

type DAGNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Color  string `json:"color"`
}

type DAGEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type DAGResponse struct {
	Nodes []DAGNode `json:"nodes"`
	Edges []DAGEdge `json:"edges"`
}

type BatchTaskNamesRequest struct {
	TaskNames []string `json:"task_names"`
}

type BatchOperationResult struct {
	SuccessCount int      `json:"success_count"`
	FailedCount  int      `json:"failed_count"`
	FailedTasks  []string `json:"failed_tasks"`
}

type WebhookTestResponse struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Error      string `json:"error,omitempty"`
	Message    string `json:"message,omitempty"`
}
