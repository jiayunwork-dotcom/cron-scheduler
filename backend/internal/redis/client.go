package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	keyPrefix     = "cron:scheduler:"
	lockValue     = "lock_value"
	readyQueueKey = keyPrefix + "ready_queue"
	runningSetKey = keyPrefix + "running_tasks"
)

var unlockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
else
	return 0
end
`)

var popScript = redis.NewScript(`
local keys = redis.call("ZREVRANGE", KEYS[1], 0, ARGV[1] - 1)
if #keys > 0 then
	redis.call("ZREM", KEYS[1], unpack(keys))
end
return keys
`)

type RedisClient struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis 连接失败: %w", err)
	}

	return &RedisClient{
		client: client,
		ctx:    ctx,
	}, nil
}

func (r *RedisClient) Lock(key string, ttl time.Duration) (bool, error) {
	fullKey := keyPrefix + key
	ok, err := r.client.SetNX(r.ctx, fullKey, lockValue, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("获取分布式锁失败: %w", err)
	}
	return ok, nil
}

func (r *RedisClient) Unlock(key string) error {
	fullKey := keyPrefix + key
	if err := r.client.Del(r.ctx, fullKey).Err(); err != nil {
		return fmt.Errorf("释放分布式锁失败: %w", err)
	}
	return nil
}

func (r *RedisClient) UnlockWithCheck(key string, value string) error {
	fullKey := keyPrefix + key
	result, err := unlockScript.Run(r.ctx, r.client, []string{fullKey}, value).Result()
	if err != nil {
		return fmt.Errorf("安全释放分布式锁失败: %w", err)
	}
	if result.(int64) == 0 {
		return fmt.Errorf("锁不存在或持有者不匹配")
	}
	return nil
}

func (r *RedisClient) GenerateLockValue() string {
	return uuid.New().String()
}

func (r *RedisClient) ReadyQueueAdd(taskID string, priority int) error {
	err := r.client.ZAdd(r.ctx, readyQueueKey, &redis.Z{
		Score:  float64(priority),
		Member: taskID,
	}).Err()
	if err != nil {
		return fmt.Errorf("添加任务到就绪队列失败: %w", err)
	}
	return nil
}

func (r *RedisClient) ReadyQueuePop(max int) ([]string, error) {
	if max <= 0 {
		return []string{}, nil
	}
	result, err := popScript.Run(r.ctx, r.client, []string{readyQueueKey}, max).Result()
	if err != nil {
		return nil, fmt.Errorf("从就绪队列取出任务失败: %w", err)
	}

	raw, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("就绪队列返回结果格式错误")
	}

	taskIDs := make([]string, 0, len(raw))
	for _, item := range raw {
		id, ok := item.(string)
		if ok {
			taskIDs = append(taskIDs, id)
		}
	}
	return taskIDs, nil
}

func (r *RedisClient) ReadyQueueSize() (int64, error) {
	size, err := r.client.ZCard(r.ctx, readyQueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("获取就绪队列大小失败: %w", err)
	}
	return size, nil
}

func (r *RedisClient) ReadyQueueRemove(taskID string) error {
	err := r.client.ZRem(r.ctx, readyQueueKey, taskID).Err()
	if err != nil {
		return fmt.Errorf("从就绪队列移除任务失败: %w", err)
	}
	return nil
}

func (r *RedisClient) RunningTaskAdd(taskID string, ttl time.Duration) error {
	fullKey := keyPrefix + "running:" + taskID
	ok, err := r.client.SetNX(r.ctx, fullKey, "1", ttl).Result()
	if err != nil {
		return fmt.Errorf("添加运行中任务失败: %w", err)
	}
	if !ok {
		return fmt.Errorf("任务已在运行中: %s", taskID)
	}

	err = r.client.SAdd(r.ctx, runningSetKey, taskID).Err()
	if err != nil {
		return fmt.Errorf("添加任务到运行集合失败: %w", err)
	}
	return nil
}

func (r *RedisClient) RunningTaskRemove(taskID string) error {
	fullKey := keyPrefix + "running:" + taskID
	err := r.client.Del(r.ctx, fullKey).Err()
	if err != nil {
		return fmt.Errorf("删除运行中任务键失败: %w", err)
	}

	err = r.client.SRem(r.ctx, runningSetKey, taskID).Err()
	if err != nil {
		return fmt.Errorf("从运行集合移除任务失败: %w", err)
	}
	return nil
}

func (r *RedisClient) RunningTaskCount() (int64, error) {
	count, err := r.client.SCard(r.ctx, runningSetKey).Result()
	if err != nil {
		return 0, fmt.Errorf("获取运行中任务数量失败: %w", err)
	}
	return count, nil
}

func (r *RedisClient) RunningTaskExists(taskID string) (bool, error) {
	exists, err := r.client.SIsMember(r.ctx, runningSetKey, taskID).Result()
	if err != nil {
		return false, fmt.Errorf("检查运行中任务是否存在失败: %w", err)
	}
	return exists, nil
}

func (r *RedisClient) Get(key string) (string, error) {
	fullKey := keyPrefix + key
	value, err := r.client.Get(r.ctx, fullKey).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("键不存在: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("获取键值失败: %w", err)
	}
	return value, nil
}

func (r *RedisClient) Set(key string, value interface{}, ttl time.Duration) error {
	fullKey := keyPrefix + key
	err := r.client.Set(r.ctx, fullKey, value, ttl).Err()
	if err != nil {
		return fmt.Errorf("设置键值失败: %w", err)
	}
	return nil
}

func (r *RedisClient) Del(key string) error {
	fullKey := keyPrefix + key
	err := r.client.Del(r.ctx, fullKey).Err()
	if err != nil {
		return fmt.Errorf("删除键失败: %w", err)
	}
	return nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
