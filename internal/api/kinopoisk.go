package api

import (
	"encoding/json"
	"fmt"
	"io"
	"kinopoisk-bot/internal/model"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

type KinopoiskAPI struct {
	apiKey  string
	baseUrl string
}

func NewKinopoiskAPI(apiKey string) *KinopoiskAPI {
	return &KinopoiskAPI{
		apiKey:  apiKey,
		baseUrl: "https://api.kinopoisk.dev",
	}
}

func (k *KinopoiskAPI) doRequest(url string, result interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("accept", "application/json")
	req.Header.Add("X-API-KEY", k.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(body, result)
}

func (k *KinopoiskAPI) SearchMovie(query string, page int) ([]model.Movie, error) {
	slog.Debug("Started SearchMovie")
	searchUrl := fmt.Sprintf("%s/v1.4/movie/search?page=%d&limit=10&query=%s",
		k.baseUrl, page, url.QueryEscape(query))

	var data MovieResponse
	if err := k.doRequest(searchUrl, &data); err != nil {
		slog.Error("SearchMovie fetch err:", err)
		return nil, err
	}

	var movies []model.Movie
	for _, doc := range data.Docs {
		if doc.Name == "" {
			continue
		}
		movies = append(movies, model.Movie{
			Id:          doc.Id,
			Title:       doc.Name,
			Year:        fmt.Sprintf("%d", doc.Year),
			Rating:      fmt.Sprintf("%.1f", doc.Rating.Kp),
			Description: doc.Description,
			Poster:      doc.Poster.Url,
		})
	}
	slog.Debug("Ended SearchMovie")
	return movies, nil
}

func (k *KinopoiskAPI) SearchPerson(query string, page int) ([]model.Person, error) {
	slog.Debug("Started SearchPerson")
	searchUrl := fmt.Sprintf("%s/v1.4/person/search?page=%d&limit=10&query=%s",
		k.baseUrl, page, url.QueryEscape(query))

	var data PersonResponse
	if err := k.doRequest(searchUrl, &data); err != nil {
		slog.Error("SearchPerson fetch err:", err)
		return nil, err
	}

	var persons []model.Person
	for _, doc := range data.Docs {
		if doc.Photo == "" || doc.Birthday == "" {
			continue
		}

		t, err := time.Parse(time.RFC3339, doc.Birthday)
		if err != nil {
			panic(err)
		}
		persons = append(persons, model.Person{
			Id:     doc.Id,
			Name:   doc.Name,
			EnName: doc.EnName,
			Sex:    doc.Sex,
			Photo:  doc.Photo,
			Birth:  t.Format("02 Jan 2006"),
		})
	}
	slog.Debug("Ended SearchPerson")
	return persons, nil
}

func (k *KinopoiskAPI) SearchMoviesByPerson(personId int, page int) ([]model.Movie, error) {
	slog.Debug("Started SearchMoviesByPerson")
	searchUrl := fmt.Sprintf("%s/v1.4/movie?page=%d&limit=10&sortField=votes.imdb&sortType=-1&persons.id=%d",
		k.baseUrl, page, personId)

	var data MovieResponse
	if err := k.doRequest(searchUrl, &data); err != nil {
		slog.Error("SearchMoviesByPerson fetch err:", err)
		return nil, err
	}

	var movies []model.Movie
	for _, doc := range data.Docs {
		//if doc.Rating.Kp == 0 || doc.Description == "" || doc.Poster.Url == "" {
		//	continue
		//}
		if doc.Name == "" {
			continue
		}
		movies = append(movies, model.Movie{
			Id:          doc.Id,
			Title:       doc.Name,
			Year:        fmt.Sprintf("%d", doc.Year),
			Rating:      fmt.Sprintf("%.1f", doc.Rating.Kp),
			Description: doc.Description,
			Poster:      doc.Poster.Url,
		})
	}
	slog.Debug("Ended SearchMoviesByPerson")
	return movies, nil
}
