package api

type MovieResponse struct {
	Docs []struct {
		Id          int    `json:"id"`
		Name        string `json:"name"`
		Year        int    `json:"year"`
		Description string `json:"description"`
		Poster      struct {
			Url string `json:"url"`
		} `json:"poster"`
		Rating struct {
			Kp float64 `json:"kp"`
		} `json:"rating"`
	} `json:"docs"`
}

type PersonResponse struct {
	Docs []struct {
		Id       int    `json:"id"`
		Name     string `json:"name"`
		Photo    string `json:"photo"`
		Sex      string `json:"sex"`
		EnName   string `json:"enName"`
		Birthday string `json:"birthday"`
	} `json:"docs"`
}
