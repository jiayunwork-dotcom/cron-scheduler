package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	StatusPending     = "pending"
	StatusRunning     = "running"
	StatusSuccess     = "success"
	StatusFailed      = "failed"
	StatusTimeout     = "timeout"
	StatusSkipped     = "skipped"
	StatusInterrupted = "interrupted"
)

const (
	TriggerCron         = "cron"
	TriggerManual       = "manual"
	TriggerCompensation = "compensation"
	TriggerSkipped      = "skipped"
)

const (
	CondAllSuccess  = "all_success"
	CondAnySuccess  = "any_success"
	CondAnyComplete = "any_complete"
)

const (
	CompSkip         = "skip"
	CompExecuteOnce  = "execute_once"
	CompQueue        = "queue"
)

const (
	RetryFixed       = "fixed"
	RetryExponential = "exponential"
)

const (
	TimeoutKillAndFail  = "kill_and_fail"
	TimeoutWaitMark     = "wait_and_mark"
	TimeoutAlertAndWait = "alert_and_wait"
)

type Task struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name             string         `gorm:"type:varchar(255);uniqueIndex;not null"`
	CronExpr         string         `gorm:"type:varchar(255);not null;column:cron_expr"`
	Command          string         `gorm:"type:text;not null"`
	TimeoutSec       int            `gorm:"default:60;column:timeout_sec"`
	TimeoutStrategy  string         `gorm:"type:varchar(30);default:'kill_and_fail';column:timeout_strategy"`
	MaxRetries       int            `gorm:"default:0;column:max_retries"`
	RetryStrategy    string         `gorm:"type:varchar(20);default:'fixed';column:retry_strategy"`
	RetryIntervalSec int            `gorm:"default:60;column:retry_interval_sec"`
	Priority         int            `gorm:"default:5"`
	Dependencies     pq.StringArray `gorm:"type:text[];default:'{}'::text[]"`
	TriggerCondition string         `gorm:"type:varchar(20);default:'all_success';column:trigger_condition"`
	Enabled          bool           `gorm:"default:true"`
	Tags             pq.StringArray `gorm:"type:text[];default:'{}'::text[]"`
	Compensation     string         `gorm:"type:varchar(20);default:'skip'"`
	AlertEnabled     bool           `gorm:"default:true;column:alert_enabled"`
	LastRunAt        *time.Time     `gorm:"column:last_run_at"`
	NextRunAt        *time.Time     `gorm:"column:next_run_at"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Task) TableName() string { return "tasks" }

type ExecutionHistory struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TaskID       uuid.UUID  `gorm:"type:uuid;index;column:task_id"`
	TaskName     string     `gorm:"type:varchar(255);not null;index;column:task_name"`
	TriggerType  string     `gorm:"type:varchar(20);default:'cron';column:trigger_type"`
	TriggerTime  time.Time  `gorm:"not null;column:trigger_time"`
	StartTime    *time.Time `gorm:"column:start_time"`
	EndTime      *time.Time `gorm:"column:end_time"`
	DurationMs   *int64     `gorm:"column:duration_ms"`
	Status       string     `gorm:"type:varchar(20);default:'pending';index"`
	ExitCode     *int       `gorm:"column:exit_code"`
	Stdout       string     `gorm:"type:text;default:''"`
	Stderr       string     `gorm:"type:text;default:''"`
	RetryCount   int        `gorm:"default:0;column:retry_count"`
	ErrorMessage string     `gorm:"type:text;default:'';column:error_message"`
	CreatedAt    time.Time
}

func (ExecutionHistory) TableName() string { return "execution_history" }

type Alert struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TaskID       uuid.UUID  `gorm:"type:uuid;index;column:task_id"`
	TaskName     string     `gorm:"type:varchar(255);not null;column:task_name"`
	ExecutionID  uuid.UUID  `gorm:"type:uuid;column:execution_id"`
	AlertType    string     `gorm:"type:varchar(20);not null;column:alert_type"`
	Message      string     `gorm:"type:text;default:''"`
	WebhookURL   string     `gorm:"type:varchar(1024);default:'';column:webhook_url"`
	Sent         bool       `gorm:"default:false;index"`
	SentAt       *time.Time `gorm:"column:sent_at"`
	ErrorMessage string     `gorm:"type:text;default:'';column:error_message"`
	CreatedAt    time.Time
}

func (Alert) TableName() string { return "alerts" }

type SystemSetting struct {
	Key       string `gorm:"type:varchar(100);primaryKey"`
	Value     string `gorm:"type:text;default:''"`
	UpdatedAt time.Time
}

func (SystemSetting) TableName() string { return "system_settings" }

type MissedExecution struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TaskID         uuid.UUID  `gorm:"type:uuid;index;column:task_id"`
	TaskName       string     `gorm:"type:varchar(255);not null;column:task_name"`
	ScheduledTime  time.Time  `gorm:"not null;column:scheduled_time"`
	DetectedAt     time.Time  `gorm:"default:now();column:detected_at"`
	Compensation   string     `gorm:"type:varchar(20);default:'skip'"`
	Compensated    bool       `gorm:"default:false;index"`
	CompensatedAt  *time.Time `gorm:"column:compensated_at"`
	CreatedAt      time.Time
}

func (MissedExecution) TableName() string { return "missed_executions" }
