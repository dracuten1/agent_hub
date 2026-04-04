package feature

import (
	"database/sql"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	db *sqlx.DB
}

type Feature struct {
	ID          string `json:"id" db:"id"`
	ProjectID   string `json:"project_id" db:"project_id"`
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	UserID      string `json:"user_id" db:"user_id"`
	Status      string `json:"status" db:"status"`
	CreatedAt   string `json:"created_at" db:"created_at"`
	UpdatedAt   string `json:"updated_at" db:"updated_at"`
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) Create(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req struct {
		ProjectID   string `json:"project_id" binding:"required"`
		Name        string `json:"name" binding:"required,max=200"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	id := uuid.New().String()
	_, err := h.db.Exec(
		"INSERT INTO features (id, project_id, name, description, user_id) VALUES ($1, $2, $3, $4, $5)",
		id, req.ProjectID, req.Name, req.Description, userID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create feature"})
		return
	}

	var feature Feature
	h.db.Get(&feature, `SELECT id, project_id, name, description, user_id, status, created_at, updated_at
		FROM features WHERE id = $1`, id)

	c.JSON(201, gin.H{"feature": feature})
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")

	var feature Feature
	err := h.db.Get(&feature, `SELECT id, project_id, name, description, user_id, status, created_at, updated_at
		FROM features WHERE id = $1`, id)
	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "Feature not found"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to get feature"})
		return
	}

	c.JSON(200, gin.H{"feature": feature})
}

func (h *Handler) List(c *gin.Context) {
	projectID := c.Query("project_id")

	var features []Feature
	var err error
	if projectID != "" {
		err = h.db.Select(&features, `SELECT id, project_id, name, description, user_id, status, created_at, updated_at
		FROM features WHERE project_id = $1 ORDER BY created_at`, projectID)
	} else {
		err = h.db.Select(&features, `SELECT id, project_id, name, description, user_id, status, created_at, updated_at
		FROM features ORDER BY created_at DESC`)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to list features"})
		return
	}
	if features == nil {
		features = []Feature{}
	}

	c.JSON(200, gin.H{"features": features})
}

func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	updates := "updated_at = NOW()"
	args := []interface{}{id}
	idx := 2

	if req.Name != nil {
		updates += ", name = $" + fmt.Sprintf("%d", idx)
		args = append(args, *req.Name)
		idx++
	}
	if req.Description != nil {
		updates += ", description = $" + fmt.Sprintf("%d", idx)
		args = append(args, *req.Description)
		idx++
	}
	if req.Status != nil {
		updates += ", status = $" + fmt.Sprintf("%d", idx)
		args = append(args, *req.Status)
		idx++
	}

	query := "UPDATE features SET " + updates + " WHERE id = $1 RETURNING *"
	var feature Feature
	err := h.db.QueryRowx(query, args...).StructScan(&feature)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update feature"})
		return
	}

	c.JSON(200, gin.H{"feature": feature})
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	result, err := h.db.Exec("DELETE FROM features WHERE id = $1", id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete feature"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(404, gin.H{"error": "Feature not found"})
		return
	}

	c.JSON(200, gin.H{"message": "Feature deleted"})
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/features", h.List)
	g.GET("/features/:id", h.Get)
	g.POST("/features", h.Create)
	g.PATCH("/features/:id", h.Update)
	g.DELETE("/features/:id", h.Delete)
}
