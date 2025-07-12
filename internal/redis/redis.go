package redis

import (
	"context"
	"encoding/json"
	"kinopoisk-bot/internal/model"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	client   *redis.Client
	stateTTL time.Duration
}

func NewRedisClient(addr string, password string, db int, stateTTL time.Duration) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx).Result(); err != nil {
		return nil, err
	}

	return &RedisClient{client: client, stateTTL: stateTTL}, nil
}

func (r *RedisClient) SaveState(chatID int64, state model.SearchState) error {
	ctx := context.Background()
	key := strconv.FormatInt(chatID, 10)
	data, err := json.Marshal(state)
	if err != nil {
		slog.Error("Error marshaling state", "error", err)
		return err
	}
	return r.client.Set(ctx, key, data, r.stateTTL).Err()
}

func (r *RedisClient) GetState(chatID int64) (*model.SearchState, error) {
	ctx := context.Background()
	key := strconv.FormatInt(chatID, 10)
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		slog.Error("Error getting state", "error", err)
		return nil, err
	}

	var state model.SearchState
	if err := json.Unmarshal(data, &state); err != nil {
		slog.Error("Error unmarshaling state", "error", err)
		return nil, err
	}
	return &state, nil
}

func (r *RedisClient) DeleteState(chatID int64) error {
	ctx := context.Background()
	key := strconv.FormatInt(chatID, 10)
	return r.client.Del(ctx, key).Err()
}
