package bot

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"html"
	"kinopoisk-bot/internal/api"
	"kinopoisk-bot/internal/model"
	"kinopoisk-bot/internal/redis"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
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

func (b *Bot) createMainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🎬 Поиск фильмов"),
			tgbotapi.NewKeyboardButton("👤 Поиск актеров/режиссеров"),
		),
	)
}

func (b *Bot) createCancelKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить поиск", "cancel_search"),
		),
	)
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

func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	if query.Message == nil {
		slog.Warn("Received callback without message", "data", query.Data)
		return
	}

	// Отправляем подтверждение callback
	callbackConfig := tgbotapi.CallbackConfig{
		CallbackQueryID: query.ID,
	}
	if _, err := b.api.Request(callbackConfig); err != nil {
		slog.Error("Error sending callback response", "error", err)
	}

	data := query.Data
	parts := strings.Split(data, ":")
	chatID := query.Message.Chat.ID

	switch parts[0] {
	case "cancel_search":
		// Удаляем состояние из Redis
		if err := b.redis.DeleteState(chatID); err != nil {
			slog.Error("Error deleting state from Redis", "error", err)
		}
		reply := tgbotapi.NewMessage(chatID, "Поиск отменен")
		reply.ReplyMarkup = b.createMainMenuKeyboard()
		_, err := b.api.Send(reply)
		if err != nil {
			slog.Error("Error sending cancel message", "error", err)
		}
	case "movie_page":
		if len(parts) < 2 {
			return
		}
		page, _ := strconv.Atoi(parts[1])
		b.handleMoviePagination(chatID, page)
	case "person_page":
		if len(parts) < 2 {
			return
		}
		page, _ := strconv.Atoi(parts[1])
		b.handlePersonPagination(chatID, page)
	case "person_select":
		if len(parts) < 2 {
			return
		}
		personID, _ := strconv.Atoi(parts[1])
		b.handlePersonSelect(chatID, personID)
	case "person_movies_page":
		if len(parts) < 2 {
			return
		}
		page, _ := strconv.Atoi(parts[1])
		b.handlePersonMoviesPagination(chatID, page)
	}
}

func (b *Bot) handleMoviePagination(chatID int64, page int) {
	state, err := b.redis.GetState(chatID)
	if err != nil {
		slog.Error("Error getting state in handleMoviePagination", "error", err)
		b.sendStateExpired(chatID)
		return
	}
	if state == nil || state.Query == "" {
		b.sendStateExpired(chatID)
		return
	}

	movies, _ := b.kinopoisk.SearchMovie(state.Query, page)
	if len(movies) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Больше фильмов не найдено")
		_, err := b.api.Send(msg)
		if err != nil {
			slog.Error("Error sending no more movies message", "error", err)
		}
		return
	}

	// Обновляем состояние
	state.Page = page
	if err := b.redis.SaveState(chatID, *state); err != nil {
		slog.Error("Error saving state to Redis", "error", err)
	}

	b.sendMovies(chatID, movies, page, "movie_page")
}

func (b *Bot) handlePersonPagination(chatID int64, page int) {
	state, err := b.redis.GetState(chatID)
	if err != nil {
		slog.Error("Error getting state in handlePersonPagination", "error", err)
		b.sendStateExpired(chatID)
		return
	}
	if state == nil || state.Query == "" {
		b.sendStateExpired(chatID)
		return
	}

	persons, _ := b.kinopoisk.SearchPerson(state.Query, page)
	if len(persons) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Больше актеров/режиссеров не найдено")
		_, err := b.api.Send(msg)
		if err != nil {
			slog.Error("Error sending no more persons message", "error", err)
		}
		return
	}

	// Обновляем состояние
	state.Page = page
	if err := b.redis.SaveState(chatID, *state); err != nil {
		slog.Error("Error saving state to Redis", "error", err)
	}

	b.sendPersons(chatID, persons, page)
}

func (b *Bot) handlePersonSelect(chatID int64, personID int) {
	// Создаем новое состояние для фильмов по персоне
	state := model.SearchState{
		Type:     searchTypePersonMovies,
		PersonID: personID,
		Page:     1,
	}
	if err := b.redis.SaveState(chatID, state); err != nil {
		slog.Error("Error saving person state to Redis", "error", err)
		return
	}

	movies, _ := b.kinopoisk.SearchMoviesByPerson(personID, 1)
	b.sendMovies(chatID, movies, 1, "person_movies_page")
}

func (b *Bot) handlePersonMoviesPagination(chatID int64, page int) {
	state, err := b.redis.GetState(chatID)
	if err != nil {
		slog.Error("Error getting state in handlePersonMoviesPagination", "error", err)
		b.sendStateExpired(chatID)
		return
	}
	if state == nil || state.Type != searchTypePersonMovies {
		slog.Warn("Invalid state for person movies", "state", state)
		b.sendStateExpired(chatID)
		return
	}

	movies, _ := b.kinopoisk.SearchMoviesByPerson(state.PersonID, page)
	if len(movies) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Больше фильмов не найдено")
		_, err := b.api.Send(msg)
		if err != nil {
			slog.Error("Error sending no more movies message", "error", err)
		}
		return
	}

	// Обновляем состояние
	state.Page = page
	if err := b.redis.SaveState(chatID, *state); err != nil {
		slog.Error("Error saving state to Redis", "error", err)
	}

	b.sendMovies(chatID, movies, page, "person_movies_page")
}

func (b *Bot) sendStateExpired(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Сессия поиска истекла. Пожалуйста, начните поиск заново.")
	msg.ReplyMarkup = b.createMainMenuKeyboard()
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("Error sending session expired message", "error", err)
	}
}

func (b *Bot) sendMovies(chatID int64, movies []model.Movie, page int, paginationPrefix string) {
	start := time.Now()
	defer func() {
		slog.Debug("sendMovies executed",
			"duration", time.Since(start).Seconds(),
			"movies", len(movies))
	}()

	if len(movies) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Фильмы не найдены")
		_, err := b.api.Send(msg)
		if err != nil {
			slog.Error("Error sending no movies found message", "error", err)
		}
		return
	}

	tempMsg, err := b.api.Send(tgbotapi.NewMessage(chatID, "⏳ Подготавливаю постеры.."))
	if err != nil {
		slog.Error("Error sending session expired message", "error", err)
	}

	// Параллельная загрузка изображений с сохранением порядка
	type posterResult struct {
		index  int
		poster tgbotapi.RequestFileData
	}

	results := make(chan posterResult, len(movies))
	var wg sync.WaitGroup

	// Запускаем горутины для загрузки каждого изображения
	for i, movie := range movies {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			poster := GetSafePoster(url)
			results <- posterResult{idx, poster}
		}(i, movie.Poster)
	}

	// Закрываем канал после завершения всех горутин
	go func() {
		wg.Wait()
		close(results)
	}()

	// Собираем результаты в правильном порядке
	posters := make([]tgbotapi.RequestFileData, len(movies))
	for res := range results {
		posters[res.index] = res.poster
	}

	slog.Debug("movies downloaded",
		"duration", time.Since(start).Seconds(),
		"movies", len(movies))

	// Формируем и отправляем медиагруппу
	var mediaGroup []interface{}
	for i, movie := range movies {
		photo := tgbotapi.NewInputMediaPhoto(posters[i])
		caption := formatMovieCaption(movie)
		if len(caption) > telegramCaptionLimit {
			caption = caption[:telegramCaptionLimit-3] + "..."
		}
		photo.Caption = caption
		mediaGroup = append(mediaGroup, photo)
	}

	slog.Debug("formed groups",
		"duration", time.Since(start).Seconds(),
		"movies", len(movies))

	if tempMsg.MessageID != 0 {
		_, err := b.api.Request(tgbotapi.NewDeleteMessage(chatID, tempMsg.MessageID))
		if err != nil {
			slog.Error("Error deleting temp message", "error", err)
		}
	}

	_, err = b.api.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatUploadPhoto))
	if err != nil {
		slog.Error("Error sending Chat Upload Photo", "error", err)
	}

	_, err = b.api.SendMediaGroup(tgbotapi.MediaGroupConfig{
		ChatID: chatID,
		Media:  mediaGroup,
	})
	if err != nil {
		slog.Error("SendMediaGroup error:", "error", err)
		for i, movie := range movies {
			b.sendSingleMovie(chatID, movie, i+1)
		}
	}

	slog.Debug("Sent poster result",
		"duration", time.Since(start).Seconds(),
		"movies", len(movies))

	// Отправка текстового описания и пагинации (остается без изменений)
	_, err = b.api.Request(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))
	if err != nil {
		slog.Error("Error sending Chat Typing", "error", err)
	}
	var description string
	for i, movie := range movies {
		description += fmt.Sprintf("%d. %s\n", i+1, formatMovieDescription(movie))
	}
	msg := tgbotapi.NewMessage(chatID, description)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err = b.api.Send(msg)
	if err != nil {
		slog.Error("Error sending description", "error", err)
	}

	b.sendPagination(chatID, page, paginationPrefix)
}

func (b *Bot) sendPersons(chatID int64, persons []model.Person, page int) {
	if len(persons) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Актеры/режиссеры не найдены")
		_, err := b.api.Send(msg)
		if err != nil {
			slog.Error("Error sending no persons found message", "error", err)
		}
		return
	}

	text := "Результаты поиска актеров/режиссеров:\n\n"
	var buttons []tgbotapi.InlineKeyboardButton

	for i, person := range persons {
		text += fmt.Sprintf("%d. %s\n", i+1, formatPersonDescription(person))
		btn := tgbotapi.NewInlineKeyboardButtonData(
			person.Name,
			"person_select:"+strconv.Itoa(person.Id),
		)
		buttons = append(buttons, btn)
	}

	// Кнопки пагинации
	paginationRow := b.createPersonPaginationRow(page)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(buttons...),
		paginationRow,
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("SendMessage err:", "error", err)
	}
}

func (b *Bot) sendPagination(chatID int64, page int, prefix string) {
	var buttons []tgbotapi.InlineKeyboardButton
	if page > 1 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("⬅", prefix+":"+strconv.Itoa(page-1)))
	}
	buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("➡", prefix+":"+strconv.Itoa(page+1)))

	msg := tgbotapi.NewMessage(chatID, "Страница: "+strconv.Itoa(page))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(buttons)
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("Send pagination buttons err:", "error", err)
	}
}

func (b *Bot) createPersonPaginationRow(page int) []tgbotapi.InlineKeyboardButton {
	var buttons []tgbotapi.InlineKeyboardButton
	if page > 1 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("⬅", "person_page:"+strconv.Itoa(page-1)))
	}
	buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("➡", "person_page:"+strconv.Itoa(page+1)))
	return buttons
}

// Вспомогательные функции
func formatMovieCaption(movie model.Movie) string {
	caption := fmt.Sprintf("🎬 %s (%s)\n⭐ %s\n📖 %s",
		movie.Title, movie.Year, movie.Rating, movie.Description)
	if len(caption) > telegramCaptionLimit {
		return caption[:telegramCaptionLimit-3] + "..."
	}
	return caption
}

func formatMovieDescription(movie model.Movie) string {
	return fmt.Sprintf(
		`<a href="https://www.kinopoisk.ru/film/%d/">%s (%s)</a>`,
		movie.Id,
		html.EscapeString(movie.Title),
		movie.Year,
	)
}

func formatPersonDescription(person model.Person) string {
	return fmt.Sprintf("%s (%s), %s", person.Name, person.EnName, person.Birth)
}

func (b *Bot) sendSingleMovie(chatID int64, movie model.Movie, index int) {
	// Отправка постера
	poster := GetSafePoster(movie.Poster)
	photoMsg := tgbotapi.NewPhoto(chatID, poster)
	photoMsg.Caption = formatMovieCaption(movie)
	_, err := b.api.Send(photoMsg)
	if err != nil {
		slog.Error("Failed to send single movie poster",
			"movie", movie.Title,
			"error", err)
	}

	// Отправка текстового описания
	textMsg := tgbotapi.NewMessage(chatID,
		fmt.Sprintf("%d. %s", index, formatMovieDescription(movie)))
	textMsg.ParseMode = "HTML"
	textMsg.DisableWebPagePreview = true
	send, err := b.api.Send(textMsg)
	if err != nil {
		slog.Error("Failed to send single movie message")
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
