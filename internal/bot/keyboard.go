package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"strconv"
)

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

func (b *Bot) createPersonPaginationRow(page int) []tgbotapi.InlineKeyboardButton {
	var buttons []tgbotapi.InlineKeyboardButton
	if page > 1 {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("⬅", "person_page:"+strconv.Itoa(page-1)))
	}
	buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("➡", "person_page:"+strconv.Itoa(page+1)))
	return buttons
}
