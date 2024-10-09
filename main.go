package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "math/rand"
    "net/http"
    "os"
    "sort"
    "time"

    "github.com/go-redis/redis/v8"
    "github.com/gorilla/mux"
    "github.com/joho/godotenv"
    "github.com/rs/cors"
)

var ctx = context.Background()
var client *redis.Client

func main() {
    err := godotenv.Load(".env")
    if err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }
    redisURL := os.Getenv("REDIS_URL")
    serverPort := os.Getenv("SERVER_PORT")
    opt, err := redis.ParseURL(redisURL)
    if err != nil {
        log.Fatalf("Failed to parse Redis URL: %v", err)
    }
    client = redis.NewClient(opt)
    r := mux.NewRouter()
    c := cors.New(cors.Options{
        AllowedOrigins:   []string{"*"}, // Allow all origins
        AllowCredentials: true,
        AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
        AllowedHeaders:   []string{"Content-Type"},
    })
    r.HandleFunc("/api/startGame", startGameHandler).Methods("POST")
    r.HandleFunc("/api/drawCard", drawCardHandler).Methods("POST")
    r.HandleFunc("/api/getLeaderboard", getLeaderboardHandler).Methods("GET")
    go func() {
        if err := http.ListenAndServe(":"+serverPort, c.Handler(r)); err != nil {
            log.Fatalf("Failed to start server: %v", err)
        }
    }()

    fmt.Printf("Server is running on http://localhost:%s\n", serverPort)
    select {}
}
func startGameHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
        return
    }

    var request struct {
        Username string `json:"username"`
        Points   int    `json:"points"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }
    if request.Username == "" {
        http.Error(w, "Username cannot be empty", http.StatusBadRequest)
        return
    }
    existingPoints, err := client.HGet(ctx, request.Username, "points").Int()
    if err == redis.Nil {
        existingPoints = 0
    } else if err != nil {
        http.Error(w, "Failed to check user points", http.StatusInternalServerError)
        return
    }
    newPoints := existingPoints + request.Points

    err = client.HSet(ctx, request.Username, "points", newPoints, "gameProgress", "{}").Err()
    if err != nil {
        http.Error(w, "Failed to update user points", http.StatusInternalServerError)
        return
    }
    deck := shuffleDeck()
    gameProgress := map[string]interface{}{"deck": deck, "currentCard": nil}
    err = client.HSet(ctx, request.Username, "gameProgress", gameProgress).Err()
    if err != nil {
        http.Error(w, "Failed to update game progress", http.StatusInternalServerError)
        return
    }
    response := map[string]interface{}{
        "deck":     deck,
        "points":   newPoints,
        "message":  "Game Started!",
        "gameOver": false,
    }
    json.NewEncoder(w).Encode(response)
}

func drawCardHandler(w http.ResponseWriter, r *http.Request) {
    var request struct {
        Username string   `json:"username"`
        Deck     []string `json:"deck"`
    }
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    if len(request.Deck) == 0 {
        http.Error(w, "Deck is empty or not provided!", http.StatusBadRequest)
        return
    }

    drawnCard := request.Deck[len(request.Deck)-1]
    request.Deck = request.Deck[:len(request.Deck)-1]

    gameProgress := map[string]interface{}{"deck": request.Deck, "currentCard": drawnCard}
    client.HSet(ctx, request.Username, "gameProgress", gameProgress)

    currentPoints, err := client.HGet(ctx, request.Username, "points").Int()
    if err != nil {
        http.Error(w, "Failed to get user points", http.StatusInternalServerError)
        return
    }

    pointsToAdd := 0
    if drawnCard == "cat" {
        pointsToAdd = 1
    } else if drawnCard == "bomb" {
        pointsToAdd = -currentPoints
    }

    newPoints := currentPoints + pointsToAdd
    err = client.HSet(ctx, request.Username, "points", newPoints).Err()
    if err != nil {
        http.Error(w, "Failed to update points", http.StatusInternalServerError)
        return
    }

    response := map[string]interface{}{
        "deck":   request.Deck,
        "points": newPoints,
    }

    if drawnCard == "bomb" {
        response["message"] = "Game Over! You lost!"
        response["gameOver"] = true
    } else {
        response["message"] = fmt.Sprintf("You drew a %s card!", drawnCard)
        response["gameOver"] = false
    }

    json.NewEncoder(w).Encode(response)
}
func getLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
    users, err := client.Keys(ctx, "*").Result()
    if err != nil {
        http.Error(w, "Failed to get users", http.StatusInternalServerError)
        return
    }

    leaderboard := make([]map[string]interface{}, 0)
    for _, user := range users {
        pointsStr, err := client.HGet(ctx, user, "points").Result()
        if err == nil {
            points := 0
            fmt.Sscanf(pointsStr, "%d", &points)
            leaderboard = append(leaderboard, map[string]interface{}{
                "username": user,
                "points":   points,
            })
        }
    }

    sort.Slice(leaderboard, func(i, j int) bool {
        return leaderboard[i]["points"].(int) > leaderboard[j]["points"].(int)
    })

    json.NewEncoder(w).Encode(leaderboard)
}
func shuffleDeck() []string {
    deck := []string{"cat", "bomb", "defuse", "shuffle"}
    rand.Seed(time.Now().UnixNano())
    rand.Shuffle(len(deck), func(i, j int) {
        deck[i], deck[j] = deck[j], deck[i]
    })
    return deck
}
