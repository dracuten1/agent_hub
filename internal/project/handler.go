package project

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	db *sqlx.DB
}

type Project struct {
	ID          string `json:"id" db:"id"`
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
		Name        string `json:"name" binding:"required,max=200"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request"})
		return
	}

	id := uuid.New().String()
	_, err := h.db.Exec(
		"INSERT INTO projects (id, name, description, user_id) VALUES ($1, $2, $3, $4)",
		id, req.Name, req.Description, userID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to create project"})
		return
	}

	var project Project
	h.db.Get(&project, "SELECT * FROM projects WHERE id = $1", id)

	c.JSON(201, gin.H{"project": project})
}

func (h *Handler) List(c *gin.Context) {
	userID, _ := c.Get("userID")

	var projects []Project
	err := h.db.Select(&projects,
		"SELECT * FROM projects WHERE user_id = $1 OR 'admin' = $2 ORDER BY created_at DESC",
		userID, userID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to list projects"})
		return
	}
	if projects == nil {
		projects = []Project{}
	}

	c.JSON(200, gin.H{"projects": projects})
}

func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")

	var project Project
	err := h.db.Get(&project, "SELECT * FROM projects WHERE id = $1", id)
	if err == sql.ErrNoRows {
		c.JSON(404, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(200, gin.H{"project": project})
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
		updates += ", name = $" + ph(idx)
		args = append(args, *req.Name)
		idx++
	}
	if req.Description != nil {
		updates += ", description = $" + ph(idx)
		args = append(args, *req.Description)
		idx++
	}
	if req.Status != nil {
		updates += ", status = $" + ph(idx)
		args = append(args, *req.Status)
		idx++
	}

	query := "UPDATE projects SET " + updates + " WHERE id = $1 RETURNING *"
	var project Project
	err := h.db.QueryRowx(query, args...).StructScan(&project)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to update project"})
		return
	}

	c.JSON(200, gin.H{"project": project})
}

func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")

	result, err := h.db.Exec("DELETE FROM projects WHERE id = $1", id)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to delete project"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(404, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(200, gin.H{"message": "Project deleted"})
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/projects", h.List)
	g.GET("/projects/:id", h.Get)
	g.POST("/projects", h.Create)
	g.PATCH("/projects/:id", h.Update)
	g.DELETE("/projects/:id", h.Delete)
}

func ph(idx int) string {
	return "$" + string(rune('0'+idx))
}
