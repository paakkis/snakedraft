package handlers

import (
	"database/sql"

	"cs2-tournament/config"
	"cs2-tournament/services"
)

type Handler struct {
	db    *sql.DB
	cfg   config.Config
	steam *services.SteamService
}

func New(db *sql.DB, cfg config.Config) *Handler {
	return &Handler{
		db:    db,
		cfg:   cfg,
		steam: services.NewSteamService(cfg.SteamAPIKey),
	}
}
