package handlers

import (
	"database/sql"

	"cs2-tournament/config"
	"cs2-tournament/services"
)

type Handler struct {
	db     *sql.DB
	cfg    config.Config
	steam  *services.SteamService
	faceit *services.FaceitService
}

func New(db *sql.DB, cfg config.Config) *Handler {
	return &Handler{
		db:     db,
		cfg:    cfg,
		steam:  services.NewSteamService(cfg.SteamAPIKey),
		faceit: services.NewFaceitService(cfg.FaceitAPIKey),
	}
}

func (h *Handler) fetchLeetify(steam64ID string) (*services.LeetifyProfile, error) {
	return services.FetchLeetifyProfile(steam64ID, h.cfg.LeetifyAPIKey)
}
