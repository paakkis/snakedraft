package handlers

import (
	"net/http"

	"cs2-tournament/models"

	"github.com/gin-gonic/gin"
)

// POST /api/tournaments
func (h *Handler) CreateTournament(c *gin.Context) {
	var body struct {
		Name     string `json:"name" binding:"required"`
		TeamSize int    `json:"team_size"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.TeamSize == 0 {
		body.TeamSize = 5
	}

	var t models.Tournament
	err := h.db.QueryRow(`
		INSERT INTO tournaments (name, team_size) VALUES ($1, $2)
		RETURNING id, name, team_size, status, created_at, updated_at`,
		body.Name, body.TeamSize,
	).Scan(&t.ID, &t.Name, &t.TeamSize, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	t.Players = []models.Player{}
	t.Teams = []models.Team{}
	t.Requests = []models.TeammateRequest{}
	c.JSON(http.StatusCreated, t)
}

// GET /api/tournaments
func (h *Handler) ListTournaments(c *gin.Context) {
	rows, err := h.db.Query(
		`SELECT id, name, team_size, status, created_at, updated_at FROM tournaments ORDER BY created_at DESC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	tournaments := []models.Tournament{}
	for rows.Next() {
		var t models.Tournament
		rows.Scan(&t.ID, &t.Name, &t.TeamSize, &t.Status, &t.CreatedAt, &t.UpdatedAt)
		t.Players = []models.Player{}
		t.Teams = []models.Team{}
		t.Requests = []models.TeammateRequest{}
		tournaments = append(tournaments, t)
	}
	c.JSON(http.StatusOK, tournaments)
}

// GET /api/tournaments/:id
func (h *Handler) GetTournament(c *gin.Context) {
	id := c.Param("id")

	var t models.Tournament
	err := h.db.QueryRow(
		`SELECT id, name, team_size, status, created_at, updated_at FROM tournaments WHERE id=$1`, id,
	).Scan(&t.ID, &t.Name, &t.TeamSize, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tournament not found"})
		return
	}

	players, err := loadPlayers(h.db, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	t.Players = players

	teams, err := loadTeams(h.db, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	t.Teams = teams

	requests, err := loadRequests(h.db, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	t.Requests = requests

	c.JSON(http.StatusOK, t)
}
