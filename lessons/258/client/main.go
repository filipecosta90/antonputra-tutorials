package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/antonputra/go-utils/util"
	"github.com/redis/go-redis/v9"

	"github.com/prometheus/client_golang/prometheus"
)

var host string

func init() {
	host = os.Getenv("REDIS_HOST")
	if host == "" {
		log.Fatalln("You MUST set REDIS_HOST env variable!")
	}
}

func main() {
	cfg := new(Config)
	cfg.loadConfig("config.yaml")

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	StartPrometheusServer(cfg, reg)

	runTest(*cfg, m)
}

func runTest(cfg Config, m *metrics) {
	latencyCh := make(chan int64, 100_000)

	startTime := time.Now()
	activeClients := 0

	go func() {
		h := hdrhistogram.New(1, 10_000_000, 3) // 1µs to 10s
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case d := <-latencyCh:
				h.RecordValue(d)

			case <-ticker.C:
				elapsed := int(time.Since(startTime).Seconds())
				fmt.Printf(
					"[Summary] Time: %3ds | QPS: %5d | p90 Latency: %6d µs | Active Clients: %d\n",
					elapsed, h.TotalCount(), h.ValueAtQuantile(90.0), activeClients,
				)
				h.Reset()
			}
		}
	}()

	var ctx = context.Background()
	currentClients := cfg.Test.MinClients

	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:6379", host),
		Password: "",
		DB:       0,
	})

	for {
		clients := make(chan struct{}, currentClients)
		m.stage.Set(float64(currentClients))

		now := time.Now()
		for {
			activeClients = len(clients)

			clients <- struct{}{}
			go func() {
				var micros int64
				var err error

				util.Sleep(cfg.Test.RequestDelayMs)

				u := NewUser()

				err, micros = u.SaveToRedis(ctx, rdb, m, cfg.Redis.Expiration, cfg.Debug)
				util.Warn(err, "u.SaveToRedis failed")

				err, micros = u.GetFromRedis(ctx, rdb, m, cfg.Debug)
				util.Warn(err, "u.GetFromRedis failed")

				select {
				case latencyCh <- micros:
				default:
				}

				<-clients
			}()

			if time.Since(now).Seconds() >= float64(cfg.Test.StageIntervalS) {
				break
			}
		}

		if currentClients == cfg.Test.MaxClients {
			break
		}
		currentClients += 1
	}
}
