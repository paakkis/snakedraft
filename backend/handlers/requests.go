package handlers

import (
	"database/sql"
	"fmt"
	"net/http"

	"cs2-tournament/models"
	"cs2-tournament/services"

	"github.com/gin-gonic/gin"
)

// POST /api/tournaments/:id/requests
// Body: { "requester_id": "...", "requested_id": "..." }
//
// Records that one player wants to be on the same team as another. Requests
// are wishes, not guarantees: the balancer drops them when they would make
// the teams unfair. A request is refused outright when it would chain enough
// friends together to fill a whole team.
func (h *Handler) CreateTeammateRequest(c *gin.Context) {
	tournamentID := c.Param("id")

	var body struct {
		RequesterID string `json:"requester_id" binding:"required"`
		RequestedID string `json:"requested_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.RequesterID == body.RequestedID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A player cannot request themselves"})
		return
	}

	var teamSize int
	err := h.db.QueryRow(`SELECT team_size FROM tournaments WHERE id=$1`, tournamentID).Scan(&teamSize)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "tournament not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var found int
	err = h.db.QueryRow(
		`SELECT COUNT(*) FROM players WHERE tournament_id=$1 AND id IN ($2,$3)`,
		tournamentID, body.RequesterID, body.RequestedID,
	).Scan(&found)
	if err != nil || found != 2 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Both players must be in this tournament"})
		return
	}

	existing, err := loadRequests(h.db, tournamentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	links := make([]services.TeammateRequest, len(existing))
	for i, r := range existing {
		if (r.RequesterID == body.RequesterID && r.RequestedID == body.RequestedID) ||
			(r.RequesterID == body.RequestedID && r.RequestedID == body.RequesterID) {
			c.JSON(http.StatusConflict, gin.H{"error": "These players are already linked by a teammate request"})
			return
		}
		links[i] = services.TeammateRequest{ID: r.ID, RequesterID: r.RequesterID, RequestedID: r.RequestedID}
	}
	if services.WouldFormFullTeam(links, body.RequesterID, body.RequestedID, teamSize) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": fmt.Sprintf(
			"This request would chain %d or more friends together and fill a whole team. "+
				"A group of friends can have at most %d players.", teamSize, teamSize-1)})
		return
	}

	r := models.TeammateRequest{
		TournamentID: tournamentID,
		RequesterID:  body.RequesterID,
		RequestedID:  body.RequestedID,
	}
	err = h.db.QueryRow(`
		INSERT INTO teammate_requests (tournament_id, requester_id, requested_id)
		VALUES ($1,$2,$3) RETURNING id, created_at`,
		tournamentID, body.RequesterID, body.RequestedID,
	).Scan(&r.ID, &r.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, r)
}

// DELETE /api/tournaments/:id/requests/:rid
func (h *Handler) DeleteTeammateRequest(c *gin.Context) {
	result, err := h.db.Exec(
		`DELETE FROM teammate_requests WHERE id=$1 AND tournament_id=$2`,
		c.Param("rid"), c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// loadRequests fetches all teammate requests for a tournament, oldest first.
func loadRequests(db *sql.DB, tournamentID string) ([]models.TeammateRequest, error) {
	rows, err := db.Query(`
		SELECT id, tournament_id, requester_id, requested_id, created_at
		FROM teammate_requests WHERE tournament_id=$1
		ORDER BY created_at, id`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	requests := []models.TeammateRequest{}
	for rows.Next() {
		var r models.TeammateRequest
		if err := rows.Scan(&r.ID, &r.TournamentID, &r.RequesterID, &r.RequestedID, &r.CreatedAt); err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}
	return requests, rows.Err()
}
