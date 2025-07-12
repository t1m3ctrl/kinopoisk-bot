package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kinopoisk-bot/internal/model"
	"log/slog"
)

func (b *Bot) handleStartCommand(msg *tgbotapi.Message) {
	text := "Привет! Я бот для поиска фильмов и актеров/режиссеров в Кинопоиске.\n\n" +
		"Выберите тип поиска:"
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyMarkup = b.createMainMenuKeyboard()
	_, err := b.api.Send(reply)
	if err != nil {
		slog.Error("Error sending message in handleStartCommand", err)
	}
}

func (b *Bot) handleHelpCommand(msg *tgbotapi.Message) {
	text := "Как использовать бота:\n\n" +
		"1. Выберите тип поиска\n" +
		"2. Введите запрос для поиска\n\n" +
		"Доступные команды:\n" +
		"/start - начать работу\n" +
		"/help - показать справку"
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyMarkup = b.createMainMenuKeyboard()
	_, err := b.api.Send(reply)
	if err != nil {
		slog.Error("Error sending message in handleHelpCommand", err)
	}
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	switch msg.Text {
	case "🎬 Поиск фильмов":
		b.awaitingQuery(msg.Chat.ID, searchTypeMovie)
	case "👤 Поиск актеров/режиссеров":
		b.awaitingQuery(msg.Chat.ID, searchTypePerson)
	default:
		b.processSearchQuery(msg)
	}
}

func (b *Bot) awaitingQuery(chatID int64, searchType string) {
	// Сохраняем тип поиска в Redis
	state := model.SearchState{Type: searchType}
	if err := b.redis.SaveState(chatID, state); err != nil {
		slog.Error("Error saving state to Redis", err)
		return
	}

	message := "Введите название фильма для поиска:"
	if searchType == searchTypePerson {
		message = "Введите имя актера или режиссера для поиска:"
	}

	reply := tgbotapi.NewMessage(chatID, message)
	reply.ReplyMarkup = b.createCancelKeyboard()
	_, err := b.api.Send(reply)
	if err != nil {
		slog.Error("Error sending message", "type", searchType, "error", err)
	}
}

func (b *Bot) processSearchQuery(msg *tgbotapi.Message) {
	// Получаем состояние из Redis
	state, err := b.redis.GetState(msg.Chat.ID)
	if err != nil {
		slog.Error("Error getting state from Redis", "error", err)
		return
	}

	// Если состояние не найдено, просим выбрать тип поиска
	if state == nil {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Пожалуйста, выберите тип поиска с помощью кнопок ниже 👇")
		reply.ReplyMarkup = b.createMainMenuKeyboard()
		_, err := b.api.Send(reply)
		if err != nil {
			slog.Error("Error sending choose search type message", err)
		}
		return
	}

	query := msg.Text
	if query == "" {
		reply := tgbotapi.NewMessage(msg.Chat.ID, "Пожалуйста, укажите запрос для поиска")
		reply.ReplyMarkup = b.createMainMenuKeyboard()
		_, err := b.api.Send(reply)
		if err != nil {
			slog.Error("Error sending empty query message", err)
		}
		return
	}

	// Обновляем состояние
	state.Query = query
	state.Page = 1
	if err := b.redis.SaveState(msg.Chat.ID, *state); err != nil {
		slog.Error("Error saving state to Redis", err)
		return
	}

	switch state.Type {
	case searchTypeMovie:
		movies, _ := b.kinopoisk.SearchMovie(query, 1)
		if len(movies) == 0 {
			reply := tgbotapi.NewMessage(msg.Chat.ID, "Фильмы не найдены")
			reply.ReplyMarkup = b.createMainMenuKeyboard()
			_, err := b.api.Send(reply)
			if err != nil {
				slog.Error("Error sending no movies message", err)
			}
			return
		}
		b.sendMovies(msg.Chat.ID, movies, 1, "movie_page")
	case searchTypePerson:
		persons, _ := b.kinopoisk.SearchPerson(query, 1)
		if len(persons) == 0 {
			reply := tgbotapi.NewMessage(msg.Chat.ID, "Актеры/режиссеры не найдены")
			reply.ReplyMarkup = b.createMainMenuKeyboard()
			_, err := b.api.Send(reply)
			if err != nil {
				slog.Error("Error sending no persons message", err)
			}
			return
		}
		b.sendPersons(msg.Chat.ID, persons, 1)
	}
}
