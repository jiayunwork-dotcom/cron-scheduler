package main

import (
	"cron-scheduler/internal/alerter"
	"cron-scheduler/internal/api"
	"cron-scheduler/internal/missed"
	"cron-scheduler/internal/redis"
	"cron-scheduler/internal/repository"
	"cron-scheduler/internal/scheduler"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func connectDB(dsn string) (*gorm.DB, error) {
	var db *gorm.DB
	var err error
	maxRetries := 5
	retryInterval := 3 * time.Second

	for i := 0; i < maxRetries; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			return db, nil
		}
		log.Printf("数据库连接失败 (尝试 %d/%d): %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(retryInterval)
		}
	}
	return nil, fmt.Errorf("数据库连接失败，已重试%d次: %w", maxRetries, err)
}

func connectRedis(addr string) (*redis.RedisClient, error) {
	var client *redis.RedisClient
	var err error
	maxRetries := 5
	retryInterval := 3 * time.Second

	for i := 0; i < maxRetries; i++ {
		client, err = redis.NewRedisClient(addr, "", 0)
		if err == nil {
			return client, nil
		}
		log.Printf("Redis连接失败 (尝试 %d/%d): %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(retryInterval)
		}
	}
	return nil, fmt.Errorf("Redis连接失败，已重试%d次: %w", maxRetries, err)
}

func main() {
	dbHost := getEnv("DB_HOST", "postgres")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "cron_user")
	dbPassword := getEnv("DB_PASSWORD", "cron_password")
	dbName := getEnv("DB_NAME", "cron_scheduler")
	redisHost := getEnv("REDIS_HOST", "redis")
	redisPort := getEnv("REDIS_PORT", "6379")
	serverPort := getEnv("SERVER_PORT", "8080")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err := connectDB(dsn)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	log.Println("数据库连接成功")

	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)
	redisClient, err := connectRedis(redisAddr)
	if err != nil {
		log.Fatalf("连接Redis失败: %v", err)
	}
	log.Println("Redis连接成功")

	repo := repository.NewRepository(db)

	maxConcurrentStr := repo.GetSetting("max_concurrent_jobs", "5")
	maxConcurrent, err := strconv.Atoi(maxConcurrentStr)
	if err != nil {
		log.Printf("解析max_concurrent_jobs失败，使用默认值5: %v", err)
		maxConcurrent = 5
	}

	defaultTimeoutStr := repo.GetSetting("default_timeout_sec", "60")
	defaultTimeout, err := strconv.Atoi(defaultTimeoutStr)
	if err != nil {
		log.Printf("解析default_timeout_sec失败，使用默认值60: %v", err)
		defaultTimeout = 60
	}

	webhookURL := repo.GetSetting("alert_webhook_url", "")

	defaultCompensation := repo.GetSetting("default_compensation", "skip")

	consecutiveFailuresStr := repo.GetSetting("consecutive_failures_for_alert", "1")
	consecutiveFailures, err := strconv.Atoi(consecutiveFailuresStr)
	if err != nil {
		log.Printf("解析consecutive_failures_for_alert失败，使用默认值1: %v", err)
		consecutiveFailures = 1
	}

	silentMinutesStr := repo.GetSetting("alert_silent_minutes", "5")
	silentMinutes, err := strconv.Atoi(silentMinutesStr)
	if err != nil {
		log.Printf("解析alert_silent_minutes失败，使用默认值5: %v", err)
		silentMinutes = 5
	}

	_ = defaultTimeout
	_ = defaultCompensation

	alerter := alerter.NewAlerter(repo, webhookURL, consecutiveFailures, silentMinutes)
	scheduler := scheduler.NewScheduler(repo, redisClient, alerter, maxConcurrent)
	missedDetector := missed.NewMissedDetector(repo)

	err = scheduler.Start()
	if err != nil {
		log.Fatalf("启动调度器失败: %v", err)
	}
	log.Println("调度器启动成功")

	log.Println("开始检测错过的执行...")
	missedList, err := missedDetector.DetectAllMissed()
	if err != nil {
		log.Printf("检测错过执行失败: %v", err)
	} else {
		log.Printf("检测到%d条错过的执行记录", len(missedList))
	}

	uncompensated, err := repo.ListUncompensatedMissed()
	if err != nil {
		log.Printf("获取未补偿错过执行记录失败: %v", err)
	} else {
		log.Printf("处理%d条未补偿的错过执行记录", len(uncompensated))
		for i := range uncompensated {
			err := missedDetector.ProcessCompensation(&uncompensated[i], scheduler)
			if err != nil {
				log.Printf("处理补偿执行失败 (任务: %s, 计划时间: %v): %v",
					uncompensated[i].TaskName, uncompensated[i].ScheduledTime, err)
			}
		}
	}

	handler := api.NewHandler(repo, scheduler, missedDetector, alerter)
	router := api.SetupRouter(handler)

	go func() {
		log.Printf("HTTP服务启动，监听端口: %s", serverPort)
		if err := router.Run(":" + serverPort); err != nil {
			log.Fatalf("HTTP服务启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("收到信号 %v，正在优雅关闭...", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
		log.Println("调度器已优雅关闭")
	case <-ctx.Done():
		log.Println("关闭超时，强制退出")
	}

	log.Println("服务已退出")
}
