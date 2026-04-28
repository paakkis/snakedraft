# CS2 Tournament Team Balancer — Implementation Plan

## Overview

A web app for running CS2 tournaments where players sign up via Steam URL, stats are fetched automatically from Leetify, and balanced teams are generated using a snake draft + swap optimization algorithm.

---

## Tech Stack

- **Frontend:** React + TypeScript + Vite + Zustand + TailwindCSS
- **Backend:** Go + Gin
- **Database:** PostgreSQL 16
- **Local dev:** Docker Compose + Air (Go hot reload) + Vite HMR
- **External APIs:** Steam Web API + Leetify Public API (`https://api-public-docs.cs-prod.leetify.com`)

---

## Project Structure

```
cs2-tournament/
├── docker-compose.yml
├── .env
├── .env.example
│
├── backend/
│   ├── Dockerfile
│   ├── .air.toml
│   ├── go.mod
│   ├── go.sum
│   ├── main.go
│   ├── config/
│   │   └── config.go
│   ├── db/
│   │   ├── db.go
│   │   └── migrations/
│   │       ├── 001_tournaments.sql
│   │       ├── 002_players.sql
│   │       └── 003_teams.sql
│   ├── handlers/
│   │   ├── players.go
│   │   ├── tournaments.go
│   │   └── teams.go
│   ├── services/
│   │   ├── steam.go
│   │   ├── leetify.go
│   │   ├── scoring.go
│   │   └── balancer.go
│   └── models/
│       ├── player.go
│       ├── tournament.go
│       └── team.go
│
└── frontend/
    ├── Dockerfile
    ├── index.html
    ├── vite.config.ts
    ├── tailwind.config.ts
    ├── tsconfig.json
    ├── package.json
    └── src/
        ├── main.tsx
        ├── App.tsx
        ├── api/
        │   └── client.ts
        ├── components/
        │   ├── PlayerInput.tsx
        │   ├── PlayerList.tsx
        │   ├── PlayerCard.tsx
        │   ├── TeamBuilder.tsx
        │   ├── TeamView.tsx
        │   └── DraggablePlayer.tsx
        ├── store/
        │   └── tournament.ts
        └── types/
            └── index.ts
```

---

## Docker Setup

### `docker-compose.yml`

```yaml
services:
  db:
    image: postgres:16
    restart: unless-stopped
    environment:
      POSTGRES_DB: cs2tournament
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    volumes:
      - pgdata:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 5s
      retries: 5

  backend:
    build: ./backend
    restart: unless-stopped
    volumes:
      - ./backend:/app
    environment:
      DATABASE_URL: postgres://postgres:postgres@db:5432/cs2tournament?sslmode=disable
      STEAM_API_KEY: ${STEAM_API_KEY}
      PORT: 8080
    ports:
      - "8080:8080"
    depends_on:
      db:
        condition: service_healthy

  frontend:
    build: ./frontend
    restart: unless-stopped
    volumes:
      - ./frontend/src:/app/src
      - ./frontend/index.html:/app/index.html
    ports:
      - "5173:5173"
    environment:
      VITE_API_URL: http://localhost:8080

volumes:
  pgdata:
```

### `backend/Dockerfile`

```dockerfile
FROM golang:1.23-alpine

WORKDIR /app

RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .

CMD ["air", "-c", ".air.toml"]
```

### `backend/.air.toml`

```toml
root = "."
tmp_dir = "tmp"

[build]
cmd = "go build -o ./tmp/main ."
bin = "./tmp/main"
include_ext = ["go"]
exclude_dir = ["tmp", "vendor"]
delay = 1000

[log]
time = true

[misc]
clean_on_exit = true
```

### `frontend/Dockerfile`

```dockerfile
FROM node:20-alpine

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm install

COPY . .

EXPOSE 5173

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
```

### `.env`

```
STEAM_API_KEY=your_steam_api_key_here
```

### `.env.example`

```
STEAM_API_KEY=
```

---

## Database Schema

Run these migrations in order on startup. Use a simple sequential migration runner in `db/db.go` that tracks which migrations have been applied in a `schema_migrations` table.

### `001_tournaments.sql`

```sql
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tournaments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    team_size   INT NOT NULL DEFAULT 5,
    status      TEXT NOT NULL DEFAULT 'signup',
    created_at  TIMESTAMPTZ DEFAULT now(),
    updated_at  TIMESTAMPTZ DEFAULT now()
);
```

### `002_players.sql`

```sql
CREATE TABLE IF NOT EXISTS players (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id   UUID REFERENCES tournaments(id) ON DELETE CASCADE,
    steam64_id      TEXT NOT NULL,
    vanity_url      TEXT,
    steam_name      TEXT,
    avatar_url      TEXT,
    faceit_elo      INT,
    faceit_level    INT,
    premier_rating  INT,
    skill_score     FLOAT,
    is_banned       BOOLEAN DEFAULT false,
    ban_info        JSONB,
    privacy_mode    TEXT,
    raw_stats       JSONB,
    fetched_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT now(),
    UNIQUE (tournament_id, steam64_id)
);
```

### `003_teams.sql`

```sql
CREATE TABLE IF NOT EXISTS teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tournament_id   UUID REFERENCES tournaments(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    avg_skill_score FLOAT DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS team_players (
    team_id         UUID REFERENCES teams(id) ON DELETE CASCADE,
    player_id       UUID REFERENCES players(id) ON DELETE CASCADE,
    manually_moved  BOOLEAN DEFAULT false,
    PRIMARY KEY (team_id, player_id)
);
```

---

## Backend

### `config/config.go`

```go
package config

import "os"

type Config struct {
    DatabaseURL string
    SteamAPIKey string
    Port        string
}

func Load() Config {
    return Config{
        DatabaseURL: os.Getenv("DATABASE_URL"),
        SteamAPIKey: os.Getenv("STEAM_API_KEY"),
        Port:        getEnvOrDefault("PORT", "8080"),
    }
}

func getEnvOrDefault(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

### `main.go`

```go
package main

import (
    "log"

    "cs2-tournament/config"
    "cs2-tournament/db"
    "cs2-tournament/handlers"

    "github.com/gin-contrib/cors"
    "github.com/gin-gonic/gin"
)

func main() {
    cfg := config.Load()

    database, err := db.Connect(cfg.DatabaseURL)
    if err != nil {
        log.Fatal("Failed to connect to database:", err)
    }
    if err := db.Migrate(database); err != nil {
        log.Fatal("Failed to run migrations:", err)
    }

    r := gin.Default()
    r.Use(cors.New(cors.Config{
        AllowOrigins: []string{"http://localhost:5173"},
        AllowMethods: []string{"GET", "POST", "PATCH", "DELETE"},
        AllowHeaders: []string{"Content-Type"},
    }))

    h := handlers.New(database, cfg)

    api := r.Group("/api")
    {
        api.POST("/tournaments", h.CreateTournament)
        api.GET("/tournaments/:id", h.GetTournament)
        api.GET("/tournaments", h.ListTournaments)

        api.POST("/tournaments/:id/players", h.AddPlayer)
        api.DELETE("/tournaments/:id/players/:pid", h.RemovePlayer)
        api.POST("/tournaments/:id/players/:pid/refresh", h.RefreshPlayer)

        api.POST("/tournaments/:id/balance", h.BalanceTeams)
        api.PATCH("/tournaments/:id/teams/move", h.MovePlayer)
    }

    log.Printf("Server running on :%s", cfg.Port)
    r.Run(":" + cfg.Port)
}
```

### `db/db.go`

```go
package db

import (
    "database/sql"
    "fmt"
    "os"
    "path/filepath"
    "sort"

    _ "github.com/lib/pq"
)

func Connect(url string) (*sql.DB, error) {
    db, err := sql.Open("postgres", url)
    if err != nil {
        return nil, err
    }
    if err := db.Ping(); err != nil {
        return nil, err
    }
    return db, nil
}

func Migrate(db *sql.DB) error {
    _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version TEXT PRIMARY KEY,
        applied_at TIMESTAMPTZ DEFAULT now()
    )`)
    if err != nil {
        return err
    }

    files, err := filepath.Glob("db/migrations/*.sql")
    if err != nil {
        return err
    }
    sort.Strings(files)

    for _, f := range files {
        version := filepath.Base(f)
        var exists bool
        db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists)
        if exists {
            continue
        }

        content, err := os.ReadFile(f)
        if err != nil {
            return fmt.Errorf("reading migration %s: %w", f, err)
        }
        if _, err := db.Exec(string(content)); err != nil {
            return fmt.Errorf("applying migration %s: %w", f, err)
        }
        db.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, version)
    }
    return nil
}
```

### `services/steam.go`

```go
package services

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"
)

type SteamService struct {
    APIKey string
    client *http.Client
}

func NewSteamService(apiKey string) *SteamService {
    return &SteamService{
        APIKey: apiKey,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

type PlayerSummary struct {
    Steam64ID string `json:"steamid"`
    Name      string `json:"personaname"`
    AvatarURL string `json:"avatarfull"`
}

func (s *SteamService) ResolveVanityURL(vanityURL string) (string, error) {
    url := fmt.Sprintf(
        "https://api.steampowered.com/ISteamUser/ResolveVanityURL/v1/?key=%s&vanityurl=%s",
        s.APIKey, vanityURL,
    )
    resp, err := s.client.Get(url)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var result struct {
        Response struct {
            SteamID string `json:"steamid"`
            Success int    `json:"success"`
        } `json:"response"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return "", err
    }
    if result.Response.Success != 1 {
        return "", fmt.Errorf("vanity URL not found")
    }
    return result.Response.SteamID, nil
}

func (s *SteamService) GetPlayerSummary(steam64ID string) (*PlayerSummary, error) {
    url := fmt.Sprintf(
        "https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s",
        s.APIKey, steam64ID,
    )
    resp, err := s.client.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var result struct {
        Response struct {
            Players []PlayerSummary `json:"players"`
        } `json:"response"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    if len(result.Response.Players) == 0 {
        return nil, fmt.Errorf("player not found")
    }
    return &result.Response.Players[0], nil
}

// ParseSteamInput handles three input formats:
//   https://steamcommunity.com/id/vanityname     → resolve vanity URL
//   https://steamcommunity.com/profiles/76561... → extract ID directly
//   76561...                                     → use directly
func (s *SteamService) ParseSteamInput(input string) (steam64ID string, vanityURL string, err error) {
    input = strings.TrimSpace(input)

    if strings.Contains(input, "/profiles/") {
        parts := strings.Split(input, "/profiles/")
        steam64ID = strings.Trim(parts[len(parts)-1], "/")
        return steam64ID, "", nil
    }

    if strings.Contains(input, "/id/") {
        parts := strings.Split(input, "/id/")
        vanityURL = strings.Trim(parts[len(parts)-1], "/")
        steam64ID, err = s.ResolveVanityURL(vanityURL)
        return steam64ID, vanityURL, err
    }

    // Assume raw Steam64 ID
    if len(input) > 10 {
        return input, "", nil
    }

    return "", "", fmt.Errorf("could not parse steam input: %s", input)
}
```

### `services/leetify.go`

```go
package services

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type LeetifyProfile struct {
    PrivacyMode  string  `json:"privacy_mode"`
    Name         string  `json:"name"`
    Steam64ID    string  `json:"steam64_id"`
    Winrate      float64 `json:"winrate"`
    TotalMatches int     `json:"total_matches"`
    Bans         []struct {
        Platform string `json:"platform"`
    } `json:"bans"`
    Ranks struct {
        Leetify   float64 `json:"leetify"`
        Premier   int     `json:"premier"`
        Faceit    int     `json:"faceit"`
        FaceitElo *int    `json:"faceit_elo"` // nullable — must use pointer
    } `json:"ranks"`
    Rating struct {
        Aim         float64 `json:"aim"`
        Positioning float64 `json:"positioning"`
        Utility     float64 `json:"utility"`
        Clutch      float64 `json:"clutch"`
        Opening     float64 `json:"opening"`
        CtLeetify   float64 `json:"ct_leetify"`
        TLeetify    float64 `json:"t_leetify"`
    } `json:"rating"`
    Stats struct {
        AccuracyEnemySpotted           float64 `json:"accuracy_enemy_spotted"`
        AccuracyHead                   float64 `json:"accuracy_head"`
        CtOpeningDuelSuccessPercentage float64 `json:"ct_opening_duel_success_percentage"`
        TOpeningDuelSuccessPercentage  float64 `json:"t_opening_duel_success_percentage"`
        ReactionTimeMs                 float64 `json:"reaction_time_ms"`
        TradeKillsSuccessPercentage    float64 `json:"trade_kills_success_percentage"`
    } `json:"stats"`
    RecentMatches []RecentMatch `json:"recent_matches"`
}

type RecentMatch struct {
    LeetifyRating float64 `json:"leetify_rating"`
    FinishedAt    string  `json:"finished_at"`
    Outcome       string  `json:"outcome"`
    DataSource    string  `json:"data_source"`
}

func FetchLeetifyProfile(steam64ID string) (*LeetifyProfile, error) {
    url := fmt.Sprintf("https://api.leetify.com/v3/profile?steam64_id=%s", steam64ID)

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    if resp.StatusCode == 404 {
        return nil, fmt.Errorf("player not found on Leetify")
    }
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("leetify API error: %d", resp.StatusCode)
    }

    var profile LeetifyProfile
    if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
        return nil, err
    }
    return &profile, nil
}
```

### `services/scoring.go`

```go
package services

import "math"

// ComputeSkillScore returns a 0–100 normalized skill score for a player.
// Returns -1 if the profile is private (no stats available).
//
// Weights:
//   30% Faceit ELO (or level if ELO missing)
//   25% Premier rating
//   15% Leetify aim score
//   10% Leetify positioning score
//    8% Opening duel win % (avg CT + T side)
//    8% Recent form (exponentially decayed last 10 matches)
//    4% Leetify utility score
func ComputeSkillScore(p *LeetifyProfile) float64 {
    if p.PrivacyMode != "public" {
        return -1
    }

    var faceitNorm float64
    if p.Ranks.FaceitElo != nil {
        faceitNorm = normalize(float64(*p.Ranks.FaceitElo), 0, 4000)
    } else if p.Ranks.Faceit > 0 {
        // Fall back to level (1–10) if raw ELO not available
        faceitNorm = normalize(float64(p.Ranks.Faceit), 0, 10)
    }

    premierNorm := normalize(float64(p.Ranks.Premier), 0, 35000)
    aimNorm     := p.Rating.Aim / 100
    posNorm     := p.Rating.Positioning / 100
    utilNorm    := p.Rating.Utility / 100
    openingNorm := normalize(
        (p.Stats.CtOpeningDuelSuccessPercentage+p.Stats.TOpeningDuelSuccessPercentage)/2,
        0, 100,
    )
    recentNorm := normalize(computeRecentForm(p.RecentMatches), -0.2, 0.3)

    score := faceitNorm*0.30 +
        premierNorm*0.25 +
        aimNorm*0.15 +
        posNorm*0.10 +
        openingNorm*0.08 +
        recentNorm*0.08 +
        utilNorm*0.04

    return math.Round(score*10000) / 100 // 0–100 with 2 decimal places
}

func normalize(value, min, max float64) float64 {
    if max == min {
        return 0
    }
    n := (value - min) / (max - min)
    if n < 0 {
        return 0
    }
    if n > 1 {
        return 1
    }
    return n
}

// computeRecentForm returns an exponentially weighted average of the
// player's last 10 match leetify_ratings. Faceit matches are weighted
// 20% higher since the competition level is higher.
func computeRecentForm(matches []RecentMatch) float64 {
    if len(matches) == 0 {
        return 0
    }
    limit := 10
    if len(matches) < limit {
        limit = len(matches)
    }
    decay := 0.85
    var weightSum, valueSum float64
    for i, m := range matches[:limit] {
        multiplier := 1.0
        if m.DataSource == "faceit" {
            multiplier = 1.2
        }
        w := math.Pow(decay, float64(i)) * multiplier
        valueSum += m.LeetifyRating * w
        weightSum += w
    }
    return valueSum / weightSum
}
```

### `services/balancer.go`

```go
package services

import (
    "math"
    "math/rand"
    "sort"
)

type PlayerScore struct {
    PlayerID   string
    SkillScore float64
}

// SnakeDraft distributes players across K teams in snake draft order.
// Players must be pre-sorted by SkillScore descending.
//
// Example with 4 teams:
//   Round 1 (L→R): T1←P1,  T2←P2,  T3←P3,  T4←P4
//   Round 2 (R→L): T4←P5,  T3←P6,  T2←P7,  T1←P8
//   Round 3 (L→R): T1←P9,  T2←P10, ...
func SnakeDraft(players []PlayerScore, teamCount int) [][]PlayerScore {
    sorted := make([]PlayerScore, len(players))
    copy(sorted, players)
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].SkillScore > sorted[j].SkillScore
    })

    teams := make([][]PlayerScore, teamCount)
    for i := range teams {
        teams[i] = []PlayerScore{}
    }

    for i, p := range sorted {
        round := i / teamCount
        pos := i % teamCount
        var teamIdx int
        if round%2 == 0 {
            teamIdx = pos
        } else {
            teamIdx = teamCount - 1 - pos
        }
        teams[teamIdx] = append(teams[teamIdx], p)
    }
    return teams
}

// OptimizeTeams runs random pairwise player swaps between teams,
// keeping any swap that reduces the variance of team average scores.
// 10,000 iterations runs in < 10ms for 40 players across 8 teams.
func OptimizeTeams(teams [][]PlayerScore, iterations int) [][]PlayerScore {
    best := deepCopy(teams)
    bestVar := TeamVariance(best)

    for i := 0; i < iterations; i++ {
        t1 := rand.Intn(len(best))
        t2 := rand.Intn(len(best))
        if t1 == t2 || len(best[t1]) == 0 || len(best[t2]) == 0 {
            continue
        }
        p1 := rand.Intn(len(best[t1]))
        p2 := rand.Intn(len(best[t2]))

        candidate := deepCopy(best)
        candidate[t1][p1], candidate[t2][p2] = candidate[t2][p2], candidate[t1][p1]

        if v := TeamVariance(candidate); v < bestVar {
            best = candidate
            bestVar = v
        }
    }
    return best
}

// TeamVariance returns the variance of average skill scores across teams.
// Lower is better — 0 means perfectly balanced teams.
func TeamVariance(teams [][]PlayerScore) float64 {
    avgs := make([]float64, len(teams))
    for i, team := range teams {
        sum := 0.0
        for _, p := range team {
            sum += p.SkillScore
        }
        if len(team) > 0 {
            avgs[i] = sum / float64(len(team))
        }
    }
    mean := 0.0
    for _, a := range avgs {
        mean += a
    }
    mean /= float64(len(avgs))

    variance := 0.0
    for _, a := range avgs {
        variance += math.Pow(a-mean, 2)
    }
    return variance / float64(len(avgs))
}

func deepCopy(teams [][]PlayerScore) [][]PlayerScore {
    c := make([][]PlayerScore, len(teams))
    for i, t := range teams {
        c[i] = make([]PlayerScore, len(t))
        copy(c[i], t)
    }
    return c
}
```

### `models/player.go`

```go
package models

import "time"

type Player struct {
    ID            string     `json:"id" db:"id"`
    TournamentID  string     `json:"tournament_id" db:"tournament_id"`
    Steam64ID     string     `json:"steam64_id" db:"steam64_id"`
    VanityURL     string     `json:"vanity_url" db:"vanity_url"`
    SteamName     string     `json:"steam_name" db:"steam_name"`
    AvatarURL     string     `json:"avatar_url" db:"avatar_url"`
    FaceitElo     *int       `json:"faceit_elo" db:"faceit_elo"`
    FaceitLevel   *int       `json:"faceit_level" db:"faceit_level"`
    PremierRating *int       `json:"premier_rating" db:"premier_rating"`
    SkillScore    *float64   `json:"skill_score" db:"skill_score"`
    IsBanned      bool       `json:"is_banned" db:"is_banned"`
    BanInfo       []byte     `json:"ban_info" db:"ban_info"`       // JSONB
    PrivacyMode   string     `json:"privacy_mode" db:"privacy_mode"`
    RawStats      []byte     `json:"raw_stats" db:"raw_stats"`     // JSONB
    FetchedAt     *time.Time `json:"fetched_at" db:"fetched_at"`
    CreatedAt     time.Time  `json:"created_at" db:"created_at"`
}
```

### `models/tournament.go`

```go
package models

import "time"

type Tournament struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    TeamSize  int       `json:"team_size"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Players   []Player  `json:"players"`
    Teams     []Team    `json:"teams"`
    Variance  *float64  `json:"variance,omitempty"`
}
```

### `models/team.go`

```go
package models

import "time"

type Team struct {
    ID            string    `json:"id"`
    TournamentID  string    `json:"tournament_id"`
    Name          string    `json:"name"`
    AvgSkillScore float64   `json:"avg_skill_score"`
    CreatedAt     time.Time `json:"created_at"`
    Players       []Player  `json:"players"`
}
```

### `handlers/players.go`

```go
package handlers

import (
    "net/http"
    "time"

    "cs2-tournament/services"

    "github.com/gin-gonic/gin"
)

// POST /api/tournaments/:id/players
// Body: { "steam_url": "https://steamcommunity.com/id/username" }
//
// Pipeline:
//   1. Parse steam URL → steam64ID + vanityURL
//   2. Check player not already in tournament (return 409 if duplicate)
//   3. Fetch Steam summary (name, avatar) from Steam API
//   4. Fetch Leetify profile (GET /v3/profile?steam64_id=...)
//   5. Compute skill score
//   6. Save to DB
//   7. Return player with all stats
func (h *Handler) AddPlayer(c *gin.Context) {
    tournamentID := c.Param("id")

    var body struct {
        SteamURL string `json:"steam_url" binding:"required"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // 1. Parse steam URL
    steam64ID, vanityURL, err := h.steam.ParseSteamInput(body.SteamURL)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Could not resolve Steam URL: " + err.Error()})
        return
    }

    // 2. Check duplicate
    var exists bool
    h.db.QueryRow(
        `SELECT EXISTS(SELECT 1 FROM players WHERE tournament_id=$1 AND steam64_id=$2)`,
        tournamentID, steam64ID,
    ).Scan(&exists)
    if exists {
        c.JSON(http.StatusConflict, gin.H{"error": "Player already in tournament"})
        return
    }

    // 3. Fetch Steam summary
    summary, err := h.steam.GetPlayerSummary(steam64ID)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Could not fetch Steam profile: " + err.Error()})
        return
    }

    // 4. Fetch Leetify profile (100ms delay to be polite to the API)
    time.Sleep(100 * time.Millisecond)
    profile, err := services.FetchLeetifyProfile(steam64ID)
    if err != nil {
        // Don't fail hard — allow player with no Leetify data
        // They'll get a nil skill_score and be flagged in the UI
        profile = nil
    }

    // 5. Compute skill score
    var skillScore *float64
    var isBanned bool
    if profile != nil {
        score := services.ComputeSkillScore(profile)
        skillScore = &score
        isBanned = len(profile.Bans) > 0
    }

    // 6. Save to DB
    // Store full Leetify profile as JSONB for future recomputation
    var rawStats []byte
    // marshal profile to JSON if not nil ...

    var playerID string
    err = h.db.QueryRow(`
        INSERT INTO players (
            tournament_id, steam64_id, vanity_url, steam_name, avatar_url,
            skill_score, is_banned, privacy_mode, raw_stats, fetched_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
        RETURNING id`,
        tournamentID, steam64ID, vanityURL, summary.Name, summary.AvatarURL,
        skillScore, isBanned, func() string {
            if profile != nil {
                return profile.PrivacyMode
            }
            return "unknown"
        }(),
        rawStats, time.Now(),
    ).Scan(&playerID)

    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    // 7. Return player
    c.JSON(http.StatusCreated, gin.H{
        "id":             playerID,
        "steam64_id":     steam64ID,
        "steam_name":     summary.Name,
        "avatar_url":     summary.AvatarURL,
        "skill_score":    skillScore,
        "is_banned":      isBanned,
    })
}

// DELETE /api/tournaments/:id/players/:pid
func (h *Handler) RemovePlayer(c *gin.Context) {
    _, err := h.db.Exec(
        `DELETE FROM players WHERE id=$1 AND tournament_id=$2`,
        c.Param("pid"), c.Param("id"),
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/tournaments/:id/players/:pid/refresh
// Re-fetches Leetify + Steam data. Only runs if fetched_at > 24h ago unless forced.
func (h *Handler) RefreshPlayer(c *gin.Context) {
    // Same pipeline as AddPlayer steps 3–6
    // Query player from DB, re-fetch, update row
}
```

### `handlers/teams.go`

```go
package handlers

import (
    "net/http"
    "sort"

    "cs2-tournament/services"

    "github.com/gin-gonic/gin"
)

// POST /api/tournaments/:id/balance
// Body: { "team_size": 5 }
//
// Pipeline:
//   1. Load all players for tournament (must have skill_score)
//   2. Sort by skill_score descending, bench players that don't fit evenly
//   3. Determine teamCount = floor(playerCount / teamSize)
//   4. Run SnakeDraft → OptimizeTeams(10000 iterations)
//   5. Delete existing teams for this tournament
//   6. Save new teams + team_players rows
//   7. Return teams with variance metric
func (h *Handler) BalanceTeams(c *gin.Context) {
    tournamentID := c.Param("id")

    var body struct {
        TeamSize int `json:"team_size"`
    }
    if err := c.ShouldBindJSON(&body); err != nil || body.TeamSize == 0 {
        body.TeamSize = 5
    }

    // 1. Load players
    rows, err := h.db.Query(
        `SELECT id, skill_score FROM players WHERE tournament_id=$1 AND skill_score IS NOT NULL ORDER BY skill_score DESC`,
        tournamentID,
    )
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var players []services.PlayerScore
    for rows.Next() {
        var p services.PlayerScore
        rows.Scan(&p.PlayerID, &p.SkillScore)
        players = append(players, p)
    }

    teamCount := len(players) / body.TeamSize
    if teamCount < 2 {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Not enough players to form teams"})
        return
    }

    // Bench players that don't fit (lowest ranked sit out)
    activePlayers := players[:teamCount*body.TeamSize]
    benchPlayers := players[teamCount*body.TeamSize:]
    _ = benchPlayers // return these to frontend too

    // Sort descending for snake draft
    sort.Slice(activePlayers, func(i, j int) bool {
        return activePlayers[i].SkillScore > activePlayers[j].SkillScore
    })

    // 3+4. Snake draft + optimize
    drafted := services.SnakeDraft(activePlayers, teamCount)
    optimized := services.OptimizeTeams(drafted, 10000)
    variance := services.TeamVariance(optimized)

    // 5. Delete existing teams
    h.db.Exec(`DELETE FROM teams WHERE tournament_id=$1`, tournamentID)

    // 6. Save teams
    for i, team := range optimized {
        var teamID string
        avgScore := 0.0
        for _, p := range team {
            avgScore += p.SkillScore
        }
        avgScore /= float64(len(team))

        h.db.QueryRow(
            `INSERT INTO teams (tournament_id, name, avg_skill_score) VALUES ($1,$2,$3) RETURNING id`,
            tournamentID, fmt.Sprintf("Team %d", i+1), avgScore,
        ).Scan(&teamID)

        for _, p := range team {
            h.db.Exec(
                `INSERT INTO team_players (team_id, player_id) VALUES ($1,$2)`,
                teamID, p.PlayerID,
            )
        }
    }

    // Update tournament status
    h.db.Exec(`UPDATE tournaments SET status='balanced', updated_at=now() WHERE id=$1`, tournamentID)

    c.JSON(http.StatusOK, gin.H{
        "variance": variance,
        "teams":    optimized,
    })
}

// PATCH /api/tournaments/:id/teams/move
// Body: { "player_id": "...", "from_team_id": "...", "to_team_id": "..." }
//
// Moves a player between teams and recomputes avg_skill_score for both teams.
func (h *Handler) MovePlayer(c *gin.Context) {
    var body struct {
        PlayerID   string `json:"player_id" binding:"required"`
        FromTeamID string `json:"from_team_id" binding:"required"`
        ToTeamID   string `json:"to_team_id" binding:"required"`
    }
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    // Move player
    h.db.Exec(
        `UPDATE team_players SET team_id=$1, manually_moved=true WHERE player_id=$2 AND team_id=$3`,
        body.ToTeamID, body.PlayerID, body.FromTeamID,
    )

    // Recompute avg for both affected teams
    for _, teamID := range []string{body.FromTeamID, body.ToTeamID} {
        h.db.Exec(`
            UPDATE teams SET avg_skill_score = (
                SELECT AVG(p.skill_score)
                FROM team_players tp
                JOIN players p ON p.id = tp.player_id
                WHERE tp.team_id = $1
            ) WHERE id = $1`, teamID)
    }

    c.JSON(http.StatusOK, gin.H{"ok": true})
}
```

---

## Frontend

### `types/index.ts`

```typescript
export interface Player {
  id: string
  steam64_id: string
  steam_name: string
  avatar_url: string
  vanity_url?: string
  faceit_elo?: number
  faceit_level?: number
  premier_rating?: number
  skill_score?: number       // null if private profile or no Leetify data
  is_banned: boolean
  ban_info?: { platform: string }[]
  privacy_mode: string
  rating?: {
    aim: number
    positioning: number
    utility: number
    clutch: number
  }
  fetched_at?: string
}

export interface Team {
  id: string
  name: string
  avg_skill_score: number
  players: Player[]
}

export interface Tournament {
  id: string
  name: string
  team_size: number
  status: 'signup' | 'balanced' | 'active' | 'finished'
  players: Player[]
  teams: Team[]
  variance?: number
}
```

### `store/tournament.ts`

```typescript
import { create } from 'zustand'
import { Tournament, Player, Team } from '../types'
import * as api from '../api/client'

interface TournamentStore {
  tournament: Tournament | null
  loading: boolean
  addingPlayer: boolean
  error: string | null

  createTournament: (name: string, teamSize: number) => Promise<void>
  loadTournament: (id: string) => Promise<void>
  addPlayer: (steamUrl: string) => Promise<void>
  removePlayer: (playerId: string) => Promise<void>
  balanceTeams: () => Promise<void>
  movePlayer: (playerId: string, fromTeamId: string, toTeamId: string) => Promise<void>
  clearError: () => void
}

export const useTournamentStore = create<TournamentStore>((set, get) => ({
  tournament: null,
  loading: false,
  addingPlayer: false,
  error: null,

  clearError: () => set({ error: null }),

  createTournament: async (name, teamSize) => {
    set({ loading: true, error: null })
    try {
      const tournament = await api.createTournament(name, teamSize)
      set({ tournament: { ...tournament, players: [], teams: [] } })
    } catch (e: any) {
      set({ error: e.message })
    } finally {
      set({ loading: false })
    }
  },

  loadTournament: async (id) => {
    set({ loading: true })
    try {
      const tournament = await api.getTournament(id)
      set({ tournament })
    } catch (e: any) {
      set({ error: e.message })
    } finally {
      set({ loading: false })
    }
  },

  addPlayer: async (steamUrl) => {
    set({ addingPlayer: true, error: null })
    try {
      const { tournament } = get()
      const player = await api.addPlayer(tournament!.id, steamUrl)
      set(s => ({
        tournament: {
          ...s.tournament!,
          players: [...s.tournament!.players, player].sort(
            (a, b) => (b.skill_score ?? 0) - (a.skill_score ?? 0)
          )
        }
      }))
    } catch (e: any) {
      set({ error: e.message })
    } finally {
      set({ addingPlayer: false })
    }
  },

  removePlayer: async (playerId) => {
    const { tournament } = get()
    await api.removePlayer(tournament!.id, playerId)
    set(s => ({
      tournament: {
        ...s.tournament!,
        players: s.tournament!.players.filter(p => p.id !== playerId)
      }
    }))
  },

  balanceTeams: async () => {
    set({ loading: true, error: null })
    try {
      const { tournament } = get()
      const result = await api.balanceTeams(tournament!.id)
      set(s => ({
        tournament: {
          ...s.tournament!,
          teams: result.teams,
          variance: result.variance,
          status: 'balanced'
        }
      }))
    } catch (e: any) {
      set({ error: e.message })
    } finally {
      set({ loading: false })
    }
  },

  movePlayer: async (playerId, fromTeamId, toTeamId) => {
    const { tournament } = get()
    await api.movePlayer(tournament!.id, playerId, fromTeamId, toTeamId)
    // Optimistically update local state
    set(s => {
      const teams = s.tournament!.teams.map(team => {
        if (team.id === fromTeamId) {
          return { ...team, players: team.players.filter(p => p.id !== playerId) }
        }
        if (team.id === toTeamId) {
          const player = s.tournament!.teams
            .flatMap(t => t.players)
            .find(p => p.id === playerId)!
          return { ...team, players: [...team.players, player] }
        }
        return team
      })
      return { tournament: { ...s.tournament!, teams } }
    })
  },
}))
```

### `api/client.ts`

```typescript
const BASE = import.meta.env.VITE_API_URL

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/api${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({}))
    throw new Error(err.error || `HTTP ${res.status}`)
  }
  return res.json()
}

export const createTournament = (name: string, team_size: number) =>
  request('/tournaments', {
    method: 'POST',
    body: JSON.stringify({ name, team_size }),
  })

export const getTournament = (id: string) =>
  request(`/tournaments/${id}`)

export const listTournaments = () =>
  request('/tournaments')

export const addPlayer = (tournamentId: string, steam_url: string) =>
  request(`/tournaments/${tournamentId}/players`, {
    method: 'POST',
    body: JSON.stringify({ steam_url }),
  })

export const removePlayer = (tournamentId: string, playerId: string) =>
  request(`/tournaments/${tournamentId}/players/${playerId}`, {
    method: 'DELETE',
  })

export const refreshPlayer = (tournamentId: string, playerId: string) =>
  request(`/tournaments/${tournamentId}/players/${playerId}/refresh`, {
    method: 'POST',
  })

export const balanceTeams = (tournamentId: string, team_size = 5) =>
  request(`/tournaments/${tournamentId}/balance`, {
    method: 'POST',
    body: JSON.stringify({ team_size }),
  })

export const movePlayer = (
  tournamentId: string,
  player_id: string,
  from_team_id: string,
  to_team_id: string
) =>
  request(`/tournaments/${tournamentId}/teams/move`, {
    method: 'PATCH',
    body: JSON.stringify({ player_id, from_team_id, to_team_id }),
  })
```

---

## UI Layout & Components

### Overall layout

```
┌─────────────────────────────────────────────────────────┐
│  🎮 CS2 Tournament Balancer                              │
│  [Tournament name input]  Team size: [5 ▼]  [Create]    │
├─────────────────────────────────────────────────────────┤
│  Add Player                                             │
│  [steamcommunity.com/id/username...        ] [+ Add]    │
│  ↳ spinner while fetching, inline error if not found    │
├─────────────────────────────────────────────────────────┤
│  Players (14)                    sorted by skill score  │
│  ┌────────────────────────────────────────────────────┐ │
│  │ 🖼  zouse      FL10  23,807★  Score: 84.2  ⚠ban   │ │
│  │     aim:80 pos:78 util:65                          │ │
│  │ 🖼  player2    FL8   19,200★  Score: 71.5          │ │
│  │ 🖼  player3    —     —        ⚠ Private profile    │ │
│  └────────────────────────────────────────────────────┘ │
│                                                         │
│            [⚡ Generate Balanced Teams]                  │
├─────────────────────────────────────────────────────────┤
│  Teams  Variance: 0.08 ✅  (lower = more balanced)      │
│                                                         │
│  Team 1 (avg 74.1)    Team 2 (avg 73.8)                 │
│  ┌──────────────┐     ┌──────────────┐                  │
│  │ 🖼 zouse  84 │◄───►│ 🖼 plyr2  71 │  ← drag & drop  │
│  │ 🖼 ...    61 │     │ 🖼 ...    68 │                  │
│  │ 🖼 ...    77 │     │ 🖼 ...    75 │                  │
│  └──────────────┘     └──────────────┘                  │
│  Bench: player5 (58), player6 (55) — not enough for     │
│  a full team                                            │
└─────────────────────────────────────────────────────────┘
```

### Key component responsibilities

- **`PlayerInput.tsx`** — text input for Steam URL, calls `addPlayer`, shows spinner/error inline
- **`PlayerCard.tsx`** — avatar, steam name, faceit level/elo badge, premier badge, skill score, ban warning badge, private profile warning
- **`PlayerList.tsx`** — sorted list of `PlayerCard`, remove button per player
- **`TeamBuilder.tsx`** — "Generate Balanced Teams" button, shows variance after generation with a color-coded quality indicator (green < 1, yellow 1–5, red > 5)
- **`TeamView.tsx`** — renders all teams side by side, shows bench players separately
- **`DraggablePlayer.tsx`** — wraps `PlayerCard` with drag-and-drop (use `@dnd-kit/core`)

---

## Edge Cases to Handle

| Situation | Handling |
|---|---|
| Private Leetify profile | Save player with `skill_score = null`, show warning badge, exclude from balancing or use median score |
| Player not on Leetify | Same as above — show warning, let organizer decide |
| Banned player | Show ban badge with platform name, include in pool unless manually removed |
| Player count not divisible by team size | Bench the lowest-ranked overflow players, show them in a "Bench" section |
| Re-balance after manual moves | Warn that manual moves will be lost, require confirmation before re-running |
| Duplicate Steam URL | Backend returns 409, frontend shows inline error |
| Steam API rate limit | Sequential fetches with 100ms delay between players in backend |
| Leetify API down | Graceful fallback — save player with Steam data only, flag missing stats |
| Both Faceit ELO and Premier missing | Score computed from Leetify sub-scores only, flagged as low-confidence |

---

## Go Dependencies

```
github.com/gin-gonic/gin
github.com/gin-contrib/cors
github.com/lib/pq
```

## Frontend Dependencies

```
react, react-dom
typescript
vite
tailwindcss
zustand
@dnd-kit/core
@dnd-kit/sortable
```

---

## Getting Started

```bash
# 1. Get a free Steam API key
#    https://steamcommunity.com/dev/apikey

# 2. Set up environment
cp .env.example .env
# Edit .env and add your STEAM_API_KEY

# 3. Start everything
docker compose up --build

# Frontend:  http://localhost:5173  (Vite HMR — edits reflect instantly)
# Backend:   http://localhost:8080  (Air — rebuilds on .go file save)
# Database:  localhost:5432

# 4. To reset the database
docker compose down -v && docker compose up --build
```

---

## Implementation Order

Build in this sequence — each step is independently testable:

1. Docker Compose + DB migrations running and verified
2. Backend scaffold — `main.go`, `config`, `db.go` with migration runner
3. Steam service — vanity URL resolution + player summaries
4. Leetify service — profile fetch + struct mapping
5. Scoring service — `ComputeSkillScore` with all weights
6. Balancer service — `SnakeDraft` + `OptimizeTeams` with unit tests
7. All API handlers wired up and tested with curl/Postman
8. Frontend scaffold — Vite + Tailwind + Zustand store + API client
9. Player input + player list UI
10. Team view + drag-and-drop between teams
11. Polish — loading states, error toasts, ban badges, variance indicator, bench display
