package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "math/rand"
    "net/http"
    "time"
    "sort"

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
        log.Fatalf("Failed to parse Redis URL: %v", err)
    }

    // Create Redis client
    client = redis.NewClient(opt)

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
            log.Fatalf("Failed to start server: %v", err)
        }
    }()

    // Print a message indicating that the server is running
    fmt.Println("Server is running on http://localhost:5001")

    // Keep the main function alive
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

    // Validate username
    if request.Username == "" {
        http.Error(w, "Username cannot be empty", http.StatusBadRequest)
        return
    }

    // Initialize user data in Redis
    err := client.HSet(ctx, request.Username, "points", request.Points, "gameProgress", "{}").Err()
    if err != nil {
        http.Error(w, "Failed to create user", http.StatusInternalServerError)
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

    // Draw the last card from the deck
    drawnCard := request.Deck[len(request.Deck)-1]
    request.Deck = request.Deck[:len(request.Deck)-1] // Remove the last card from the deck

    // Update the game progress in Redis
    gameProgress := map[string]interface{}{"deck": request.Deck, "currentCard": drawnCard}
    client.HSet(ctx, request.Username, "gameProgress", gameProgress)

    // Add points based on the drawn card
    currentPoints, err := client.HGet(ctx, request.Username, "points").Int()
    if err != nil {
        http.Error(w, "Failed to get user points", http.StatusInternalServerError)
        return
    }

    // Define points logic based on card type
    pointsToAdd := 0
    if drawnCard == "cat" {
        pointsToAdd = 1 // Example: Gain 1 point for drawing a cat
    } else if drawnCard == "bomb" {
        pointsToAdd = -currentPoints // Example: Lose all points for drawing a bomb
    }

    newPoints := currentPoints + pointsToAdd
    err = client.HSet(ctx, request.Username, "points", newPoints).Err()
    if err != nil {
        http.Error(w, "Failed to update points", http.StatusInternalServerError)
        return
    }

    // Check if the drawn card is a bomb and end the game if so
    response := map[string]interface{}{
        "deck":   request.Deck,
        "points": newPoints, // Send back updated points
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
    
    // Fetch points for each user
    for _, user := range users {
        pointsStr, err := client.HGet(ctx, user, "points").Result()
        if err == nil {
            points := 0
            fmt.Sscanf(pointsStr, "%d", &points) // Convert string points to int
            leaderboard = append(leaderboard, map[string]interface{}{
                "username": user,
                "points":   points,
            })
        }
    }

    // Sort the leaderboard by points in descending order
    sort.Slice(leaderboard, func(i, j int) bool {
        return leaderboard[i]["points"].(int) > leaderboard[j]["points"].(int)
    })

    json.NewEncoder(w).Encode(leaderboard)
}


// shuffleDeck generates a random deck of cards
func shuffleDeck() []string {
    deck := []string{"cat", "bomb", "defuse", "shuffle"}
    rand.Seed(time.Now().UnixNano()) // Seed random number generator
    rand.Shuffle(len(deck), func(i, j int) {
        deck[i], deck[j] = deck[j], deck[i]
    })
    return deck
}
