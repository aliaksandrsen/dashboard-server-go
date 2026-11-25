package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store abstracts key-value caching operations (implemented by Redis/Valkey and test mocks).
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Close() error
}

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(redisURL string) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}
	client := redis.NewClient(opts)
	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	val, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

func (s *RedisStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *RedisStore) Del(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}
