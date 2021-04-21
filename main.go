package main

import (
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-redis/redis"
	"net/http"
	"time"
)

type RequestBody struct {
	Urls []string `json:"urls"`
}
type ResponseBody struct {
	RedirectUrl []string `json:"redirect_url"`
}

type UrlStats struct {
	Stats []UrlStat `json:"stats"`
}

type UrlStat struct {
	Url       string `json:"url"`
	Redirects string    `json:"redirects"`
}

const jwtSecretKey = "token"

var redisClient *redis.Client

func main() {
	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		r.Use(middleware.StripSlashes)
		r.Use(middleware.Timeout(time.Second * 60))
		r.Get("/", RedirectUrl)
		r.Post("/make", MakeUrl)
		r.Post("/stats", GetStats)
	})

	s := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:       "redis-19394.c261.us-east-1-4.ec2.cloud.redislabs.com:19394",
		Password:   "XiInJpwWbv5g2kri5J7IW96mTYXha9jm", // password set
		DB:         0,                                  // use default DB
		MaxRetries: 3,                                  //added after a google suggestion
	})

	fmt.Println(s.ListenAndServe())
}

func MakeUrl(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.Header().Add("Content-Type", "application/json")
	serviceUrl := r.Host
	body := RequestBody{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to create url: " + err.Error()))
		return
	}

	res := ResponseBody{
		RedirectUrl: make([]string, 0, 10),
	}

	for _, url := range body.Urls {
		tokenData := jwt.MapClaims{
			"redirect_url": url,
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenData)
		signedEvent, err := token.SignedString([]byte(jwtSecretKey))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to create url: " + err.Error()))
			return
		}

		res.RedirectUrl = append(res.RedirectUrl, serviceUrl+"/?event="+signedEvent)
	}
	json.NewEncoder(w).Encode(res)
}

func RedirectUrl(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	event := query.Get("event")
	if event == "" {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed redirect: empty event param"))
		return
	}

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(event, claims,
		func(tok *jwt.Token) (interface{}, error) {
			return []byte(jwtSecretKey), nil
		})

	if err != nil || !token.Valid {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed redirect: token parse fail or invalid"))
		return
	}

	url, _ := claims["redirect_url"].(string)

	res, err := redisClient.Incr(url).Result()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed redirect: " + err.Error()))
		return
	}
	fmt.Println(res)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func GetStats(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.Header().Add("Content-Type", "application/json")
	body := RequestBody{}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Failed to get stats: " + err.Error()))
		return
	}

	res := UrlStats{Stats: make([]UrlStat, 0, len(body.Urls))}

	for _, url := range body.Urls {
		result, err := redisClient.Get( url).Result()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to get stats: " + err.Error()))
			return
		}

		res.Stats = append(res.Stats, UrlStat{
			Url:       url,
			Redirects: result,
		})
	}

	json.NewEncoder(w).Encode(res)
}
