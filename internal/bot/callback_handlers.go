package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kinopoisk-bot/internal/model"
	"log/slog"
	"strconv"
	"strings"
)

func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	if query.Message == nil {
		slog.Warn("Received callback without message", "data", query.Data)
		return
	}

	callbackConfig := tgbotapi.CallbackConfig{
		CallbackQueryID: query.ID,
	}
	if _, err := b.api.Request(callbackConfig); err != nil {
		slog.Error("Error sending callback response", "error", err)
	}

	data := query.Data
	parts := strings.Split(data, ":")
	chatID := query.Message.Chat.ID

	requireSecondParam := map[string]bool{
		"movie_page":         true,
		"person_page":        true,
		"person_select":      true,
		"person_movies_page": true,
	}

	if requireSecondParam[parts[0]] && len(parts) < 2 {
		slog.Warn("Invalid callback format", "data", data)
		return
	}

	switch parts[0] {
	case "cancel_search":
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
		page, _ := strconv.Atoi(parts[1])
		b.handleMoviePagination(chatID, page)
	case "person_page":
		page, _ := strconv.Atoi(parts[1])
		b.handlePersonPagination(chatID, page)
	case "person_select":
		personID, _ := strconv.Atoi(parts[1])
		b.handlePersonSelect(chatID, personID)
	case "person_movies_page":
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

	state.Page = page
	if err := b.redis.SaveState(chatID, *state); err != nil {
		slog.Error("Error saving state to Redis", "error", err)
	}

	b.sendPersons(chatID, persons, page)
}

func (b *Bot) handlePersonSelect(chatID int64, personID int) {
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

	state.Page = page
	if err := b.redis.SaveState(chatID, *state); err != nil {
		slog.Error("Error saving state to Redis", "error", err)
	}

	b.sendMovies(chatID, movies, page, "person_movies_page")
}
