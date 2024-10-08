package main

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/go-redis/redis/v8"
    "github.com/gorilla/mux"
    "github.com/rs/cors"
)

var ctx = context.Background()
var client *redis.Client

func main() {
    // Parse the Upstash Redis URL
    opt, err := redis.ParseURL("rediss://default:AWvBAAIjcDE5NDdmMDVhNWQ2OTU0ZTAwYmViZTk1ZTg1MDdiOGFjY3AxMA@divine-urchin-27585.upstash.io:6379")
    if err != nil {
        fmt.Println("Failed to parse Redis URL:", err)
        return
    }

    // Create Redis client
    client = redis.NewClient(opt)

    // Set a test value in Redis
    err = client.Set(ctx, "foo", "bar", 0).Err()
    if err != nil {
        fmt.Println("Failed to set value in Redis:", err)
        return
    }

    // Get the test value from Redis
    val, err := client.Get(ctx, "foo").Result()
    if err != nil {
        fmt.Println("Failed to get value from Redis:", err)
        return
    }
    fmt.Println("Value from Redis:", val) // Output the value to console

    // Set up HTTP router
    r := mux.NewRouter()

    // Set up CORS
    c := cors.New(cors.Options{
        AllowedOrigins:   []string{"*"}, // Allow all origins
        AllowCredentials: true,
        AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
        AllowedHeaders:   []string{"Content-Type"},
    })

    // Define API endpoints
    r.HandleFunc("/api/startGame", startGameHandler).Methods("POST")
    r.HandleFunc("/api/drawCard", drawCardHandler).Methods("POST")
    r.HandleFunc("/api/getLeaderboard", getLeaderboardHandler).Methods("GET")

    // Start the server with CORS middleware
    go func() {
        if err := http.ListenAndServe(":5001", c.Handler(r)); err != nil {
            fmt.Println("Failed to start server:", err)
        }
    }()
    
    // Print a message indicating that the server is running
    fmt.Println("Server is running on http://localhost:5001")

    // Keep the main function alive
    select {}
}

func startGameHandler(w http.ResponseWriter, r *http.Request) {
    var request struct {
        Username string `json:"username"`
    }
    json.NewDecoder(r.Body).Decode(&request)
    user, err := client.HGetAll(ctx, request.Username).Result()
    if err != nil {
        http.Error(w, "Failed to get user", http.StatusInternalServerError)
        return
    }

    if len(user) == 0 {
        err = client.HSet(ctx, request.Username, "points", 0, "gameProgress", "{}").Err()
        if err != nil {
            http.Error(w, "Failed to create user", http.StatusInternalServerError)
            return
        }
        user = map[string]string{"points": "0", "gameProgress": "{}"}
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
    json.NewDecoder(r.Body).Decode(&request)

    if len(request.Deck) == 0 {
        http.Error(w, "Deck is empty or not provided!", http.StatusBadRequest)
        return
    }

    drawnCard := request.Deck[len(request.Deck)-1]
    request.Deck = request.Deck[:len(request.Deck)-1] // Remove the last card
    gameProgress := map[string]interface{}{"deck": request.Deck, "currentCard": drawnCard}
    client.HSet(ctx, request.Username, "gameProgress", gameProgress)

    if drawnCard == "bomb" {
        response := map[string]interface{}{
            "deck":     request.Deck,
            "message":  "Game Over! You lost!",
            "gameOver": true,
        }
        json.NewEncoder(w).Encode(response)
    } else {
        response := map[string]interface{}{
            "deck":     request.Deck,
            "message":  fmt.Sprintf("You drew a %s card!", drawnCard),
            "gameOver": false,
        }
        json.NewEncoder(w).Encode(response)
    }
}

func getLeaderboardHandler(w http.ResponseWriter, r *http.Request) {
    users, err := client.Keys(ctx, "*").Result()
    if err != nil {
        http.Error(w, "Failed to get users", http.StatusInternalServerError)
        return
    }

    leaderboard := []map[string]interface{}{}
    for _, user := range users {
        points, err := client.HGet(ctx, user, "points").Result()
        if err == nil {
            leaderboard = append(leaderboard, map[string]interface{}{
                "username": user,
                "points":   points,
            })
        }
    }

    json.NewEncoder(w).Encode(leaderboard)
}

func shuffleDeck() []string {
    deck := []string{"cat", "bomb", "defuse", "shuffle"}
    return deck
}
