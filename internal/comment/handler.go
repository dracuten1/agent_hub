package comment

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Handler struct {
	db *sqlx.DB
}

// NewHandler creates a new comment handler
func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}


type AddCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

type Comment struct {
	ID        string `json:"id" db:"id"`
	AuthorName string `json:"author_name" db:"agent"`
	Content   string `json:"content" db:"content"`
	CreatedAt string `json:"created_at" db:"created_at"`
}

type PaginatedComments struct {
	Comments []Comment `json:"comments"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	Limit    int       `json:"limit"`
}

// AddComment POST /tasks/:id/comments — agent auth
func (h *Handler) AddComment(c *gin.Context) {
	taskID := c.Param("id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	var req AddCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Validate content
	if len(req.Content) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content cannot be empty"})
		return
	}
	if len(req.Content) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content exceeds 2000 characters"})
		return
	}

	// Verify task exists
	var taskExists bool
	h.db.Get(&taskExists, "SELECT EXISTS(SELECT 1 FROM tasks WHERE id = $1)", taskID)
	if !taskExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	commentID := uuid.New().String()
	var createdAt string
	err := h.db.QueryRow(
		`INSERT INTO comments (id, task_id, agent, content)
		 VALUES ($1, $2, $3, $4) RETURNING created_at`,
		commentID, taskID, agentNameStr, req.Content,
	).Scan(&createdAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add comment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         commentID,
		"content":    req.Content,
		"author_name": agentNameStr,
		"created_at": createdAt,
	})
}

// GetComments GET /tasks/:id/comments — user/JWT auth
func (h *Handler) GetComments(c *gin.Context) {
	taskID := c.Param("id")

	page := 1
	limit := 20
	if p := c.Query("page"); p != "" {
		if pv, err := strconv.Atoi(p); err == nil && pv > 0 {
			page = pv
		}
	}
	if l := c.Query("limit"); l != "" {
		if lv, err := strconv.Atoi(l); err == nil && lv > 0 && lv <= 100 {
			limit = lv
		}
	}

	offset := (page - 1) * limit

	var total int
	h.db.Get(&total, "SELECT COUNT(*) FROM comments WHERE task_id = $1", taskID)

	var comments []Comment
	err := h.db.Select(&comments,
		`SELECT id, agent, content, created_at FROM comments
		 WHERE task_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		taskID, limit, offset)
	if err != nil && err != sql.ErrNoRows {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}
	if comments == nil {
		comments = []Comment{}
	}

	c.JSON(http.StatusOK, PaginatedComments{
		Comments: comments,
		Total:    total,
		Page:     page,
		Limit:    limit,
	})
}

// DeleteComment DELETE /tasks/:id/comments/:comment_id — agent auth
func (h *Handler) DeleteComment(c *gin.Context) {
	taskID := c.Param("id")
	commentID := c.Param("comment_id")
	agentName, _ := c.Get("agentName")
	agentNameStr := agentName.(string)

	// Check comment exists
	var owner string
	err := h.db.Get(&owner,
		`SELECT agent FROM comments WHERE id = $1 AND task_id = $2`,
		commentID, taskID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Comment not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check comment"})
		return
	}
	// Ownership check — 403 for not the author
	if owner != agentNameStr {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to delete this comment"})
		return
	}

	h.db.Exec("DELETE FROM comments WHERE id = $1", commentID)
	c.JSON(http.StatusOK, gin.H{"message": "Comment deleted"})
}

// RegisterAgentRoutes registers agent-authenticated routes (POST/DELETE comments)
func (h *Handler) RegisterAgentRoutes(g *gin.RouterGroup) {
	g.POST("/tasks/:id/comments", h.AddComment)
	g.DELETE("/tasks/:id/comments/:comment_id", h.DeleteComment)
}

// RegisterUserRoutes registers user-authenticated routes (GET comments)
func (h *Handler) RegisterUserRoutes(g *gin.RouterGroup) {
	g.GET("/tasks/:id/comments", h.GetComments)
}
