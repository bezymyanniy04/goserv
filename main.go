package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"strings"
	_ "github.com/lib/pq"
)

//+marshal custom

func marsh[str any](bod str, code int, w http.ResponseWriter) {
	dat, err := json.Marshal(bod)
	if err != nil{
		w.Write([]byte(fmt.Sprintf("Error marshalling JSON: %s", err)))
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(dat)
}


//middleware

type apiConfig struct{ fileserverHits atomic.Int32 }

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

//requests

func Readiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK\n"))
}

func (cfg *apiConfig) Hitinc(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html")
	w.WriteHeader(200)
	w.Write([]byte(fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())))
}

func (cfg *apiConfig) Hitres(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Swap(0)
	w.Write([]byte(fmt.Sprintf("Hits: %v", cfg.fileserverHits.Load())))
}

//json

func validate_chirp(w http.ResponseWriter, r *http.Request) {
	type b struct {
		Body string `json:"body"`
	}
	bod := b{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&bod); err != nil {
		err_chirp("Something went wrong", 400, w)
		return
	}
	
	if len(bod.Body) > 140{
		err_chirp("Chirp is too long", 400, w)
		return
	}
	bodd := clean_chirp(strings.Split(bod.Body, " "))
	valid_chirp(bodd, w)
	

}

func clean_chirp(bod []string) string{
	li := []string{}
	for _, word := range bod{
		if strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax"{
			li = append(li, "****")
		}else{
			li = append(li, word)
		}
	}
	return strings.Join(li, " ")
}

func valid_chirp(chirp string, w http.ResponseWriter){
	type v struct {
		Cleaned string `json:"cleaned_body"`
	}
	bod := v{Cleaned:chirp}
	marsh(bod, 200, w)
}

func err_chirp(err string, code int, w http.ResponseWriter) {
	type e struct{
		Error string `json:"error"`
	}
	bod := e{Error:err}
	marsh(bod, code, w)
}

func main() {

	apiCfg := apiConfig{}

	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	// mux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	mux.HandleFunc("GET /api/healthz", Readiness)
	mux.HandleFunc("GET /admin/metrics", apiCfg.Hitinc)
	mux.HandleFunc("POST /admin/reset", apiCfg.Hitres)
	mux.HandleFunc("POST /api/validate_chirp", validate_chirp)
	srv.ListenAndServe()
}
