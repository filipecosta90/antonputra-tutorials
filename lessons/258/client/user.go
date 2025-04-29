package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/antonputra/go-utils/util"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

type User struct {
	Uuid      string `json:"uuid"`
	Username  string `json:"username"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Address   string `json:"address"`
}

func NewUser() *User {
	id := uuid.New()

	u := User{
		Uuid:      id.String(),
		Username:  util.GenString(10),
		FirstName: util.GenString(5),
		LastName:  util.GenString(10),
		Address:   util.GenString(20),
	}

	return &u
}

func (u *User) SaveToRedis(ctx context.Context, rdb *redis.Client, m *metrics, exp int32, debug bool) (err error, micros int64) {
	b, err := json.Marshal(u)
	if err != nil {
		return util.Annotate(err, "json.Marshal failed"), -1
	}

	expr := time.Duration(time.Duration(exp) * time.Second)
	now := time.Now()

	err = rdb.Set(ctx, u.Uuid, b, expr).Err()
	if err != nil {
		util.Warn(err, "rdb.Set failed")
	}
	dur := time.Since(now)
	micros = dur.Microseconds()

	m.duration.With(prometheus.Labels{"op": "set", "db": "redis"}).Observe(dur.Seconds())

	if debug {
		fmt.Printf("item saved in redis, key: %s, value: %s\n", u.Uuid, string(b))
	}

	return nil, micros

}

func (u *User) GetFromRedis(ctx context.Context, rdb *redis.Client, m *metrics, debug bool) (err error, micros int64) {
	now := time.Now()
	defer func() {
		if err == nil {
			dur := time.Since(now)
			micros = dur.Microseconds()
			m.duration.With(prometheus.Labels{"op": "get", "db": "redis"}).Observe(dur.Seconds())
		}
	}()

	val, err := rdb.Get(ctx, u.Uuid).Result()
	if err != nil {
		util.Warn(err, "rdb.Get failed")
	}

	if debug {
		fmt.Printf("item fetched from redis: %+v\n", val)
	}

	return nil, micros
}
