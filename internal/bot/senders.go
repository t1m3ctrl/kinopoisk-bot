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
		b.sendNoMoviesFound(chatID)
		return
	}

	tempMsg := b.sendTempMessage(chatID, "⏳ Подготавливаю постеры..")
	posters := b.loadPostersConcurrently(movies)
	mediaGroup := b.createMediaGroup(movies, posters)

	b.cleanupTempMessage(chatID, tempMsg)
	b.sendChatAction(chatID, tgbotapi.ChatUploadPhoto)
	b.sendMediaGroupOrFallback(chatID, mediaGroup, movies)
	b.sendMoviesDescription(chatID, movies)
	b.sendPagination(chatID, page, paginationPrefix)
}

func (b *Bot) sendNoMoviesFound(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Фильмы не найдены")
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("Error sending no movies found message", "error", err)
	}
}

func (b *Bot) sendTempMessage(chatID int64, text string) tgbotapi.Message {
	tempMsg, err := b.api.Send(tgbotapi.NewMessage(chatID, text))
	if err != nil {
		slog.Error("Error sending temp message", "error", err)
	}
	return tempMsg
}

func (b *Bot) loadPostersConcurrently(movies []model.Movie) []tgbotapi.RequestFileData {
	type posterResult struct {
		index  int
		poster tgbotapi.RequestFileData
	}

	results := make(chan posterResult, len(movies))
	var wg sync.WaitGroup

	for i, movie := range movies {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			poster := GetSafePoster(url)
			results <- posterResult{idx, poster}
		}(i, movie.Poster)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	posters := make([]tgbotapi.RequestFileData, len(movies))
	for res := range results {
		posters[res.index] = res.poster
	}
	return posters
}

func (b *Bot) createMediaGroup(movies []model.Movie, posters []tgbotapi.RequestFileData) []interface{} {
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
	return mediaGroup
}

func (b *Bot) cleanupTempMessage(chatID int64, tempMsg tgbotapi.Message) {
	if tempMsg.MessageID == 0 {
		return
	}
	_, err := b.api.Request(tgbotapi.NewDeleteMessage(chatID, tempMsg.MessageID))
	if err != nil {
		slog.Error("Error deleting temp message", "error", err)
	}
}

func (b *Bot) sendChatAction(chatID int64, action string) {
	_, err := b.api.Request(tgbotapi.NewChatAction(chatID, action))
	if err != nil {
		slog.Error("Error sending chat action", "action", action, "error", err)
	}
}

func (b *Bot) sendMediaGroupOrFallback(chatID int64, mediaGroup []interface{}, movies []model.Movie) {
	_, err := b.api.SendMediaGroup(tgbotapi.MediaGroupConfig{
		ChatID: chatID,
		Media:  mediaGroup,
	})
	if err != nil {
		slog.Error("SendMediaGroup error:", "error", err)
		for i, movie := range movies {
			b.sendSingleMovie(chatID, movie, i+1)
		}
	}
}

func (b *Bot) sendMoviesDescription(chatID int64, movies []model.Movie) {
	b.sendChatAction(chatID, tgbotapi.ChatTyping)

	var description string
	for i, movie := range movies {
		description += fmt.Sprintf("%d. %s\n", i+1, formatMovieDescription(movie))
	}

	msg := tgbotapi.NewMessage(chatID, description)
	msg.ParseMode = "HTML"
	msg.DisableWebPagePreview = true
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("Error sending description", "error", err)
	}
}

func (b *Bot) sendPersons(chatID int64, persons []model.Person, page int) {
	if len(persons) == 0 {
		b.sendNoPersonsFound(chatID)
		return
	}

	text := "Результаты поиска актеров/режиссеров:\n\n"
	for i, person := range persons {
		text += fmt.Sprintf("%d. %s\n", i+1, formatPersonDescription(person))
	}

	keyboard := b.createPersonsKeyboard(persons, page)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("SendMessage err:", "error", err)
	}
}

func (b *Bot) sendNoPersonsFound(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "Актеры/режиссеры не найдены")
	_, err := b.api.Send(msg)
	if err != nil {
		slog.Error("Error sending no persons found message", "error", err)
	}
}

func (b *Bot) createPersonsKeyboard(persons []model.Person, page int) tgbotapi.InlineKeyboardMarkup {
	var buttons []tgbotapi.InlineKeyboardButton
	for _, person := range persons {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			person.Name,
			"person_select:"+strconv.Itoa(person.Id),
		)
		buttons = append(buttons, btn)
	}

	paginationRow := b.createPersonPaginationRow(page)
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(buttons...),
		paginationRow,
	)
}

func (b *Bot) sendPagination(chatID int64, page int, prefix string) {
	buttons := []tgbotapi.InlineKeyboardButton{}
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
	poster := GetSafePoster(movie.Poster)
	photoMsg := tgbotapi.NewPhoto(chatID, poster)
	photoMsg.Caption = formatMovieCaption(movie)
	_, err := b.api.Send(photoMsg)
	if err != nil {
		slog.Error("Failed to send movie poster", "movie", movie.Title, "error", err)
	}

	text := fmt.Sprintf("%d. %s", index, formatMovieDescription(movie))
	textMsg := tgbotapi.NewMessage(chatID, text)
	textMsg.ParseMode = "HTML"
	textMsg.DisableWebPagePreview = true
	_, err = b.api.Send(textMsg)
	if err != nil {
		slog.Error("Failed to send movie description", "movie", movie.Title, "error", err)
	}
}
