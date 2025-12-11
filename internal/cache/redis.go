package cache

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/UnendingLoop/delayed-notifier/internal/repository"
	"github.com/wb-go/wbf/redis"
)

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache {
	var rc RedisCache
	if ttl <= 0 {
		rc.ttl = time.Minute
		log.Println("Got incorrect TTL for storing notifications in Redis. Continue with default value 1m(60 seconds)...")
	} else {
		rc.ttl = ttl
	}
	rc.client = client
	return &rc
}

func (r *RedisCache) SetByUID(ctx context.Context, uid string, pointer *repository.Notification) error {
	data, err := json.Marshal(pointer)
	if err != nil {
		return err
	}

	return r.client.SetWithExpiration(ctx, uid, data, r.ttl)
}

func (r *RedisCache) GetByUID(ctx context.Context, uid string) (*repository.Notification, error) {
	data, err := r.client.Get(ctx, uid)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, repository.ErrNotFound
	}

	var candidate repository.Notification
	if err := json.Unmarshal([]byte(data), &candidate); err != nil {
		return nil, err
	}
	return &candidate, nil
}

func (r *RedisCache) DeleteByUID(ctx context.Context, uid string) error {
	return r.client.Del(ctx, uid)
}
