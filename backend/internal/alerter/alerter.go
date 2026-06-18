package alerter

import (
	"bytes"
	"cron-scheduler/internal/models"
	"cron-scheduler/internal/repository"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type WebhookPayload struct {
	TaskName     string    `json:"task_name"`
	ExecutionID  string    `json:"execution_id"`
	Status       string    `json:"status"`
	ErrorMessage string    `json:"error_message"`
	Stdout       string    `json:"stdout"`
	Stderr       string    `json:"stderr"`
	ExitCode     *int      `json:"exit_code"`
	TriggerTime  time.Time `json:"trigger_time"`
	DurationMs   *int64    `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
}

type Alerter struct {
	repo                *repository.Repository
	webhookURL          string
	consecutiveFailures int
	silentMinutes       int
	lastAlertTime       map[string]time.Time
	failureCount        map[string]int
	mu                  sync.Mutex
}

func NewAlerter(repo *repository.Repository, webhookURL string, consecutiveFailures, silentMinutes int) *Alerter {
	return &Alerter{
		repo:                repo,
		webhookURL:          webhookURL,
		consecutiveFailures: consecutiveFailures,
		silentMinutes:       silentMinutes,
		lastAlertTime:       make(map[string]time.Time),
		failureCount:        make(map[string]int),
	}
}

func (a *Alerter) CheckAndAlert(task *models.Task, exec *models.ExecutionHistory) error {
	if exec.Status != models.StatusFailed && exec.Status != models.StatusTimeout {
		return nil
	}

	if !task.AlertEnabled {
		return nil
	}

	a.mu.Lock()
	a.failureCount[task.Name]++
	_ = a.failureCount[task.Name]
	a.mu.Unlock()

	if !a.shouldAlert(task.Name) {
		return nil
	}

	alert := &models.Alert{
		ID:          uuid.New(),
		TaskID:      task.ID,
		TaskName:    task.Name,
		ExecutionID: exec.ID,
		AlertType:   exec.Status,
		Message:     exec.ErrorMessage,
		WebhookURL:  a.webhookURL,
		Sent:        false,
		CreatedAt:   time.Now(),
	}

	if err := a.repo.CreateAlert(alert); err != nil {
		return fmt.Errorf("创建告警记录失败: %w", err)
	}

	response, sendErr := a.SendWebhook(task, exec, a.webhookURL)

	errMsg := ""
	if sendErr != nil {
		errMsg = sendErr.Error()
	}

	if markErr := a.repo.MarkAlertSent(alert.ID, errMsg); markErr != nil {
		return fmt.Errorf("标记告警发送状态失败: %w", markErr)
	}

	a.mu.Lock()
	a.lastAlertTime[task.Name] = time.Now()
	a.mu.Unlock()

	_ = response

	return nil
}

func (a *Alerter) SendWebhook(task *models.Task, exec *models.ExecutionHistory, webhookURL string) (string, error) {
	if webhookURL == "" {
		return "", fmt.Errorf("webhook URL 为空")
	}

	payload := WebhookPayload{
		TaskName:     task.Name,
		ExecutionID:  exec.ID.String(),
		Status:       exec.Status,
		ErrorMessage: exec.ErrorMessage,
		Stdout:       exec.Stdout,
		Stderr:       exec.Stderr,
		ExitCode:     exec.ExitCode,
		TriggerTime:  exec.TriggerTime,
		DurationMs:   exec.DurationMs,
		Timestamp:    time.Now(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化 webhook payload 失败: %w", err)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送 webhook 请求失败: %w", err)
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	responseBody := buf.String()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseBody, fmt.Errorf("webhook 返回非成功状态码: %d, 响应: %s", resp.StatusCode, responseBody)
	}

	return responseBody, nil
}

func (a *Alerter) OnTaskSuccess(taskName string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failureCount[taskName] = 0
}

func (a *Alerter) UpdateConfig(webhookURL string, consecutiveFailures, silentMinutes int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.webhookURL = webhookURL
	a.consecutiveFailures = consecutiveFailures
	a.silentMinutes = silentMinutes
}

func (a *Alerter) shouldAlert(taskName string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	count, ok := a.failureCount[taskName]
	if !ok || count < a.consecutiveFailures {
		return false
	}

	lastAlert, ok := a.lastAlertTime[taskName]
	if !ok {
		return true
	}

	return time.Since(lastAlert) > time.Duration(a.silentMinutes)*time.Minute
}

type TestWebhookResult struct {
	Success    bool
	StatusCode int
	DurationMs int64
	Error      string
}

func (a *Alerter) SendTestWebhook(webhookURL string) TestWebhookResult {
	result := TestWebhookResult{
		Success:    false,
		StatusCode: 0,
		DurationMs: 0,
		Error:      "",
	}

	if webhookURL == "" {
		result.Error = "Webhook URL 为空"
		return result
	}

	payload := map[string]interface{}{
		"task_name":      "test_task",
		"execution_id":   "test-execution-id",
		"status":         "test",
		"error_message":  "这是一条测试告警消息,用于验证Webhook配置是否正确",
		"stdout":         "test output",
		"stderr":         "test error",
		"exit_code":      nil,
		"trigger_time":   time.Now(),
		"duration_ms":    1234,
		"timestamp":      time.Now(),
		"alert_type":     "test",
		"message":        "Webhook测试消息 - 如果您收到此消息,说明配置正确",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		result.Error = fmt.Sprintf("序列化 payload 失败: %v", err)
		return result
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(body))
	if err != nil {
		result.Error = fmt.Sprintf("创建 HTTP 请求失败: %v", err)
		return result
	}
	req.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	resp, err := client.Do(req)
	result.DurationMs = time.Since(startTime).Milliseconds()

	if err != nil {
		result.Error = fmt.Sprintf("发送请求失败: %v", err)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
	} else {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(resp.Body)
		responseBody := buf.String()
		if len(responseBody) > 500 {
			responseBody = responseBody[:500]
		}
		result.Error = fmt.Sprintf("返回状态码异常: %d, 响应体: %s", resp.StatusCode, responseBody)
	}

	return result
}
