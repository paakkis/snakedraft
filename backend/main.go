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
		AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Content-Type"},
	}))

	h := handlers.New(database, cfg)

	api := r.Group("/api")
	{
		api.POST("/tournaments", h.CreateTournament)
		api.GET("/tournaments", h.ListTournaments)
		api.GET("/tournaments/:id", h.GetTournament)

		api.POST("/tournaments/:id/players", h.AddPlayer)
		api.DELETE("/tournaments/:id/players/:pid", h.RemovePlayer)
		api.POST("/tournaments/:id/players/:pid/refresh", h.RefreshPlayer)

		api.POST("/tournaments/:id/requests", h.CreateTeammateRequest)
		api.DELETE("/tournaments/:id/requests/:rid", h.DeleteTeammateRequest)

		api.POST("/tournaments/:id/balance", h.BalanceTeams)
		api.PATCH("/tournaments/:id/teams/move", h.MovePlayer)
	}

	log.Printf("Server running on :%s", cfg.Port)
	r.Run(":" + cfg.Port)
}
