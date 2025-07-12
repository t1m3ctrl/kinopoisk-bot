package bot

import (
	"fmt"
	"html"
	"kinopoisk-bot/internal/model"
)

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
