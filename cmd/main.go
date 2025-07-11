package main

import (
	"context"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"kinopoisk-bot/internal/api"
	"kinopoisk-bot/internal/bot"
	"kinopoisk-bot/internal/redis"
	"log/slog"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		slog.Error("Error loading .env file: %v", err)
		os.Exit(1)
	}

	if err := initConfig(); err != nil {
		slog.Error("init config err: %v", err)
		os.Exit(1)
	}

	if err := bot.InitImageCache(); err != nil {
		slog.Error("Failed to init image cache", "error", err)
		os.Exit(1)
	}

	ctx, cacheCancel := context.WithCancel(context.Background())
	defer cacheCancel()
	go bot.ClearImageCachePeriodically(ctx, viper.GetDuration("image.cache.ttl"))

	redisClient, err := redis.NewRedisClient(viper.GetString("redis.address"), viper.GetDuration("redis.ttl"))
	if err != nil {
		slog.Error("failed to create Redis client", err)
		os.Exit(1)
	}

	kinopoiskAPI := api.NewKinopoiskAPI(viper.GetString("APIKey"))
	tgBot, err := bot.NewBot(viper.GetString("TelegramToken"), redisClient, kinopoiskAPI)
	if err != nil {
		slog.Error("failed to create bot", slog.String("error", err.Error()))
		os.Exit(1)
	}

	go tgBot.Start()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	<-stopChan
	slog.Info("Shutting down gracefully...")
	tgBot.Stop()
	cacheCancel()
	slog.Info("Application shutdown complete")
}

func initConfig() error {
	viper.AddConfigPath("configs")
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	tokenErr := viper.BindEnv("TelegramToken", "TELEGRAM_TOKEN")
	if tokenErr != nil {
		slog.Error("failed to bind telegram token", "error", tokenErr)
	}
	apiErr := viper.BindEnv("APIKey", "API_KEY")
	if apiErr != nil {
		slog.Error("failed to bind kinopoisk api key", "error", apiErr)
	}
	return viper.ReadInConfig()
}
