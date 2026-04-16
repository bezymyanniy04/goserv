package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bezymyanniy04/goserv/internal/auth"
	"github.com/bezymyanniy04/goserv/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

//structs

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	JWTSecret      string
	polka_key      string
}

type UserPass struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Password  string    `json:"password"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	Refresh   string    `json:"refresh_token"`
	IsRed     bool      `json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	User_id   uuid.UUID `json:"user_id"`
}

//middleware

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})

}

//+marshal custom

func marsh[str any](bod str, code int, w http.ResponseWriter) {
	dat, err := json.Marshal(bod)
	if err != nil {
		w.Write([]byte(fmt.Sprintf("Error marshalling JSON: %s", err)))
		w.WriteHeader(500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

//json

func clean_chirp(bod []string) string {
	li := []string{}
	for _, word := range bod {
		if strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax" {
			li = append(li, "****")
		} else {
			li = append(li, word)
		}
	}
	return strings.Join(li, " ")
}

func err_mes(err string, code int, w http.ResponseWriter) {
	type e struct {
		Error string `json:"error"`
	}
	bod := e{Error: err}
	marsh(bod, code, w)
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

func (cfg *apiConfig) admin_res(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		err_mes("Forbidden", 403, w)
		return
	}
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	cfg.fileserverHits.Swap(0)
	w.Write([]byte(fmt.Sprintf("Hits: %v", cfg.fileserverHits.Load())))
	err := cfg.db.ResetUser(r.Context())
	if err != nil {
		err_mes("Something went wrong", 400, w)
		return

	}
}

func (cfg *apiConfig) post_user(w http.ResponseWriter, r *http.Request) {
	param := UserPass{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&param); err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}
	hashed_password, err := auth.HashPassword(param.Password)
	if err != nil {
		err_mes("Something went wrong with hash", 400, w)
		return
	}

	userdb, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{Email: param.Email, HashedPassword: hashed_password})
	if err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}

	user := User{
		ID:        userdb.ID,
		CreatedAt: userdb.CreatedAt,
		UpdatedAt: userdb.UpdatedAt,
		Email:     userdb.Email,
		IsRed:     userdb.IsChirpyRed.Bool,
	}
	marsh(user, 201, w)

}

func (cfg *apiConfig) post_chirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		err_mes("No token", 401, w)
		return
	}
	userid, err := auth.ValidateJWT(token, cfg.JWTSecret)
	if err != nil {
		err_mes("Invalid jwt", 401, w)
		return
	}

	param := Chirp{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&param); err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}

	if len(param.Body) > 140 {
		err_mes("Chirp is too long", 400, w)
		return
	}
	bodd := clean_chirp(strings.Split(param.Body, " "))

	chirpdb, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{Body: bodd, UserID: userid})
	if err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}
	chirp := Chirp{
		ID:        chirpdb.ID,
		CreatedAt: chirpdb.CreatedAt,
		UpdatedAt: chirpdb.UpdatedAt,
		Body:      chirpdb.Body,
		User_id:   chirpdb.UserID,
	}
	marsh(chirp, 201, w)
}

func (cfg *apiConfig) get_chirps(w http.ResponseWriter, r *http.Request) {
	s := r.URL.Query().Get("author_id")
	chirpsdb, err := cfg.db.GetChirps(r.Context())
	if s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			err_mes("authou uuid incorrect", 400, w)
			return
		}
		chirpsdb, err = cfg.db.GetChirpsWithAuthor(r.Context(), id)
	}

	if err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}

	order := r.URL.Query().Get("sort")
	if order == "desc" {
		sort.Slice(chirpsdb, func(i, j int) bool { return chirpsdb[i].CreatedAt.After(chirpsdb[j].CreatedAt) })
	}
	chirps := []Chirp{}
	for _, chirpdb := range chirpsdb {
		chirp := Chirp{
			ID:        chirpdb.ID,
			CreatedAt: chirpdb.CreatedAt,
			UpdatedAt: chirpdb.UpdatedAt,
			Body:      chirpdb.Body,
			User_id:   chirpdb.UserID,
		}
		chirps = append(chirps, chirp)
	}
	marsh(chirps, 200, w)
}

func (cfg *apiConfig) get_chirp(w http.ResponseWriter, r *http.Request) {
	chirpIdString := r.PathValue("chirpId")
	chirpId, err := uuid.Parse(chirpIdString)
	if err != nil {
		err_mes("bad uuid", 400, w)
		return
	}
	chirpdb, err := cfg.db.GetChirp(r.Context(), chirpId)
	if err != nil {
		err_mes("failed to get chirp", 404, w)
		return
	}
	chirp := Chirp{
		ID:        chirpdb.ID,
		CreatedAt: chirpdb.CreatedAt,
		UpdatedAt: chirpdb.UpdatedAt,
		Body:      chirpdb.Body,
		User_id:   chirpdb.UserID,
	}
	marsh(chirp, 200, w)
}

func (cfg *apiConfig) login(w http.ResponseWriter, r *http.Request) {
	param := UserPass{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&param); err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}
	userdb, err := cfg.db.GetUserByEmail(r.Context(), param.Email)

	if err != nil {
		err_mes("Something went wrong with db", 400, w)
		return
	}

	valid_password, err := auth.CheckPasswordHash(param.Password, userdb.HashedPassword)

	if err != nil {
		err_mes("Incorrect email or password", 401, w)
		return
	}
	if !valid_password {
		err_mes("Incorrect email or password", 401, w)
		return
	}

	tok, err := auth.MakeJWT(userdb.ID, cfg.JWTSecret, time.Hour)
	if err != nil {
		err_mes("Something went wrong with the jwt token", 400, w)
		return
	}
	refresh_str := auth.MakeRefreshToken()
	refresh, err := cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{Token: refresh_str, UserID: userdb.ID})
	if err != nil {
		err_mes("couldn't add refresh token", 400, w)
		return
	}
	user := User{
		ID:        userdb.ID,
		CreatedAt: userdb.CreatedAt,
		UpdatedAt: userdb.UpdatedAt,
		Email:     userdb.Email,
		Token:     tok,
		Refresh:   refresh.Token,
		IsRed:     userdb.IsChirpyRed.Bool,
	}
	marsh(user, 200, w)
}

func (cfg *apiConfig) refresh(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		err_mes("No authorization", 400, w)
		return
	}
	dbtoken, err := cfg.db.GetRefreshToken(r.Context(), token)
	if err != nil {
		err_mes("No token", 401, w)
		return
	}
	if dbtoken.RevokedAt.Valid {
		err_mes("Token revoked", 401, w)
		return
	}
	if dbtoken.ExpiresAt.Before(time.Now()) {
		err_mes("Token expired", 401, w)
		return
	}

	refreshed, err := auth.MakeJWT(dbtoken.UserID, cfg.JWTSecret, time.Hour)
	if err != nil {
		err_mes("Something went wrong with the jwt token", 400, w)
		return
	}

	type RefreshedToken struct {
		Token string `json:"token"`
	}
	rt := RefreshedToken{Token: refreshed}
	marsh(rt, 200, w)
}

func (cfg *apiConfig) revoke_token(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		err_mes("No authorization", 400, w)
		return
	}
	dbtoken, err := cfg.db.GetRefreshToken(r.Context(), token)
	if err != nil {
		err_mes("No token", 401, w)
		return
	}
	cfg.db.RevokeRefreshToken(r.Context(), dbtoken.Token)
	w.WriteHeader(204)
}

func (cfg *apiConfig) edit_userinfo(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		err_mes("No token", 401, w)
		return
	}
	userid, err := auth.ValidateJWT(token, cfg.JWTSecret)
	if err != nil {
		err_mes("Invalid jwt", 401, w)
		return
	}

	param := UserPass{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&param); err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}
	hashed_password, err := auth.HashPassword(param.Password)
	if err != nil {
		err_mes("Something went wrong with hash", 400, w)
		return
	}
	userinfodb, err := cfg.db.EditUserInfo(r.Context(), database.EditUserInfoParams{Email: param.Email, HashedPassword: hashed_password, ID: userid})
	if err != nil {
		err_mes("Something went wrong with db", 400, w)
		return
	}
	userinfo := User{
		ID:        userinfodb.ID,
		CreatedAt: userinfodb.CreatedAt,
		UpdatedAt: userinfodb.UpdatedAt,
		Email:     userinfodb.Email,
		IsRed:     userinfodb.IsChirpyRed.Bool,
	}
	marsh(userinfo, 200, w)
}

func (cfg *apiConfig) delete_chirp(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		err_mes("No token", 401, w)
		return
	}
	userid, err := auth.ValidateJWT(token, cfg.JWTSecret)
	if err != nil {
		err_mes("Invalid jwt", 401, w)
		return
	}
	chirpIdString := r.PathValue("chirpID")
	chirpId, err := uuid.Parse(chirpIdString)
	if err != nil {
		err_mes("bad uuid", 400, w)
		return
	}
	chirpdb, err := cfg.db.GetChirp(r.Context(), chirpId)
	if err != nil {
		err_mes("failed to get chirp", 404, w)
		return
	}
	if chirpdb.UserID != userid {
		err_mes("You're not the author", 403, w)
		return
	}
	cfg.db.DeleteChirp(r.Context(), chirpId)
	w.WriteHeader(204)
}

func (cfg *apiConfig) polkahook(w http.ResponseWriter, r *http.Request) {

	apikey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		err_mes("Something went wrong", 401, w)
		return
	}
	if apikey != cfg.polka_key {
		err_mes("wrong apikey", 401, w)
		return
	}
	type Datastruct struct {
		Id uuid.UUID `json:"user_id"`
	}
	type Req struct {
		Event string     `json:"event"`
		Data  Datastruct `json:"data"`
	}

	param := Req{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&param); err != nil {
		err_mes("Something went wrong", 400, w)
		return
	}
	if param.Event != "user.upgraded" {
		w.WriteHeader(204)
		return
	}
	if err := cfg.db.ChirpyRed(r.Context(), param.Data.Id); err != nil {
		err_mes("user not found", 404, w)
		return
	}
	w.WriteHeader(204)

}

func main() {
	godotenv.Load()

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("DB not opening")
	}
	dbQueries := database.New(db)
	apiCfg := apiConfig{}
	apiCfg.platform = os.Getenv("PLATFORM")
	apiCfg.db = dbQueries
	jwtSecret := os.Getenv("JWT_SECRET")
	apiCfg.JWTSecret = jwtSecret
	polka := os.Getenv("POLKA_KEY")
	apiCfg.polka_key = polka

	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", Readiness)
	mux.HandleFunc("GET /admin/metrics", apiCfg.Hitinc)
	mux.HandleFunc("POST /admin/reset", apiCfg.admin_res)
	mux.HandleFunc("POST /api/chirps", apiCfg.post_chirp)
	mux.HandleFunc("POST /api/users", apiCfg.post_user)
	mux.HandleFunc("GET /api/chirps", apiCfg.get_chirps)
	mux.HandleFunc("GET /api/chirps/{chirpId}", apiCfg.get_chirp)
	mux.HandleFunc("POST /api/login", apiCfg.login)
	mux.HandleFunc("POST /api/refresh", apiCfg.refresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.revoke_token)
	mux.HandleFunc("PUT /api/users", apiCfg.edit_userinfo)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.delete_chirp)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.polkahook)
	srv.ListenAndServe()
}
