package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kinopoisk-bot/internal/api"
	"kinopoisk-bot/internal/redis"
	"log/slog"
	"sync"
)

const (
	telegramCaptionLimit   = 1024
	searchTypeMovie        = "movie"
	searchTypePerson       = "person"
	searchTypePersonMovies = "person_movies"
)

type Bot struct {
	api       *tgbotapi.BotAPI
	kinopoisk *api.KinopoiskAPI
	redis     *redis.RedisClient
	stopChan  chan struct{}  // Channel to signal stopping
	wg        sync.WaitGroup // WaitGroup for graceful shutdown
}

func NewBot(token string, redisClient *redis.RedisClient, kinopoiskAPI *api.KinopoiskAPI) (*Bot, error) {
	botAPI, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	return &Bot{
		api:       botAPI,
		kinopoisk: kinopoiskAPI,
		redis:     redisClient,
		stopChan:  make(chan struct{}),
	}, nil
}

func (b *Bot) Start() {
	slog.Info("Authorized on account", slog.String("username", b.api.Self.UserName))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	b.wg.Add(1)
	defer b.wg.Done()

	for {
		select {
		case <-b.stopChan:
			slog.Info("Stopping bot update processing")
			return
		case update, ok := <-updates:
			if !ok {
				slog.Info("Updates channel closed")
				return
			}

			if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
				continue
			}

			if update.Message == nil {
				continue
			}

			if !update.Message.IsCommand() {
				b.handleMessage(update.Message)
				continue
			}

			switch update.Message.Command() {
			case "start":
				b.handleStartCommand(update.Message)
			case "help":
				b.handleHelpCommand(update.Message)
			}
		}
	}
}

func (b *Bot) Stop() {
	slog.Info("Initiating bot shutdown...")
	close(b.stopChan) // Signal to stop processing updates
	b.wg.Wait()       // Wait for all goroutines to finish

	// Close the bot API connection
	if b.api != nil {
		b.api.StopReceivingUpdates()
	}

	slog.Info("Bot shutdown complete")
}
