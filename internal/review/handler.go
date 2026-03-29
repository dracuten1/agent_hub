package review

import (
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/tuyen/agenthub/internal/db"
)

type Handler struct {
	db *sqlx.DB
}

type ReviewTask struct {
	ID             string         `json:"id" db:"id"`
	Title          string         `json:"title" db:"title"`
	Description    string         `json:"description" db:"description"`
	Priority       string         `json:"priority" db:"priority"`
	Assignee       string         `json:"assignee" db:"assignee"`
	RetryCount     int            `json:"retry_count" db:"retry_count"`
	RequiredSkills db.StringArray `json:"required_skills" db:"required_skills"`
	CreatedAt      string         `json:"created_at" db:"created_at"`
}

func NewHandler(db *sqlx.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) ListReviewQueue(c *gin.Context) {
	var tasks []ReviewTask

	h.db.Select(&tasks,
		`SELECT id, title, description, priority, COALESCE(assignee, '') as assignee,
		        retry_count, required_skills, created_at
		 FROM tasks WHERE status = 'review'
		 ORDER BY
		   CASE priority WHEN 'critical' THEN 1 WHEN 'high' THEN 2 WHEN 'medium' THEN 3 END,
		   created_at ASC`)

	if tasks == nil {
		tasks = []ReviewTask{}
	}

	c.JSON(200, gin.H{"review_queue": tasks})
}

func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/review/queue", h.ListReviewQueue)
}
