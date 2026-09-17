package config

import (
	"github.com/redis/go-redis/v9"
)

// RedisClient 按配置创建 Redis 客户端（含建连/读写超时）。
func RedisClient(cfg RedisConfig) *redis.Client {
	return redis.NewClient(cfg.ClientOptions())
}

// ClientOptions Redis 连接参数；Protocol=2 + UnstableResp3 供向量检索使用。
func (c RedisConfig) ClientOptions() *redis.Options {
	return &redis.Options{
		Addr:          c.Addr,
		Password:      c.Password,
		DB:            c.DB,
		Protocol:      2,
		UnstableResp3: true,
		DialTimeout:   c.DialTimeout(),
		ReadTimeout:   c.ReadTimeout(),
		WriteTimeout:  c.WriteTimeout(),
	}
}
