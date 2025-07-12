package bot

import (
	"fmt"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kinopoisk-bot/internal/model"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

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
	_, err = b.api.Send(textMsg)
	if err != nil {
		slog.Error("Failed to send single movie message")
	}
}
