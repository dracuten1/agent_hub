package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/tuyen/agenthub/internal/agent"
	"github.com/tuyen/agenthub/internal/auth"
	"github.com/tuyen/agenthub/internal/comment"
	"github.com/tuyen/agenthub/internal/dashboard"
	"github.com/tuyen/agenthub/internal/db"
	"github.com/tuyen/agenthub/internal/feature"
	"github.com/tuyen/agenthub/internal/health"
	"github.com/tuyen/agenthub/internal/ping"
	"github.com/tuyen/agenthub/internal/project"
	"github.com/tuyen/agenthub/internal/review"
	"github.com/tuyen/agenthub/internal/task"
	"github.com/tuyen/agenthub/internal/version"
	"github.com/tuyen/agenthub/internal/websocket"
	"github.com/tuyen/agenthub/internal/workflow"
	"github.com/tuyen/agenthub/middleware"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Record server start time for uptime tracking
	startTime := time.Now()

	// Config
	dbURL := os.Getenv("DATABASE_URL")
	jwtSecret := os.Getenv("JWT_SECRET")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	if dbURL == "" {
		dbURL = "postgres://agenthub:agenthub@localhost:5432/agenthub?sslmode=disable"
	}
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	// Database
	database, err := sqlx.Connect("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Run migrations
	if err := db.RunMigrations(database); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Seed admin user (upsert — no-op if already exists)
	adminPassword := os.Getenv("AGENTHUB_ADMIN_PASS")
	if adminPassword == "" {
		log.Fatal("AGENTHUB_ADMIN_PASS env var is required")
	}
	var adminExists int
	database.Get(&adminExists, "SELECT COUNT(*) FROM users WHERE username = 'admin'")
	if adminExists == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash admin password: %v", err)
		}
		_, err = database.Exec(
			`INSERT INTO users (id, username, email, password, role, created_at)
			 VALUES (gen_random_uuid(), 'admin', 'admin@agenthub.com', $1, 'admin', NOW())
			 ON CONFLICT (username) DO NOTHING`,
			string(hash))
		if err != nil {
			log.Fatalf("Failed to seed admin user: %v", err)
		}
	}
	if adminExists == 0 {
		log.Println("[Bootstrap] Admin user created")
	} else {
		log.Println("[Bootstrap] Admin user already exists — bcrypt skipped")
	}

	// Router
	r := gin.Default()

	// Request logging
	r.Use(middleware.Logging())
	// CORS
	r.Use(middleware.CORS())

	// Middleware
	authMiddleware := auth.NewMiddleware(jwtSecret)
	agentMiddleware := auth.NewAgentMiddleware(database)

	// WebSocket hub
	wsHub := websocket.NewHub()
	go wsHub.Run()
	wsHandler := websocket.NewHandler(wsHub)

	// Health handler (no auth required)
	pingHandler := ping.NewHandler()
	healthHandler := health.NewHandler(startTime)
	versionHandler := version.NewHandler()

	// Public routes (user auth + agent registration + health)
	public := r.Group("/api")
	{
		authHandler := auth.NewHandler(database, jwtSecret)
		agentHandler := agent.NewHandler(database)
		public.GET("/hello", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "hello from agenthub", "version": "1.1"})
		})
		public.GET("/health", healthHandler.Health)
		public.GET("/ping", pingHandler.Get)
		public.GET("/version", versionHandler.Get)
		public.POST("/auth/register", authHandler.Register)
		public.POST("/auth/login", authHandler.Login)
		// Dashboard API (used by React SPA)
		dashboardHandler := dashboard.NewHandler(database)
		public.GET("/dashboard/summary", dashboardHandler.Summary)
		public.GET("/dashboard/agents", dashboardHandler.Agents)
		public.GET("/dashboard/tasks", dashboardHandler.Tasks)
		public.GET("/dashboard", dashboardHandler.Summary)
		public.POST("/agent/register", agentHandler.RegisterAgent)
		public.GET("/ws", wsHandler.HandleWS)
	}

	// Workflow engine (created early so task handlers can reference it)
	wfEngine := workflow.NewEngine(database)

	// Agent routes (API key auth) — registration excluded (chicken-and-egg)
	agentGroup := r.Group("/api/agent")
	agentGroup.Use(agentMiddleware)
	{
		agentHandler := agent.NewHandler(database)
		taskHandler := task.NewHandler(database, wsHub)
		taskHandler.WorkflowAdvancer = wfEngine

		agentHandler.RegisterRoutes(agentGroup)
		agentHandler.RegisterAgentRoutes(agentGroup)
		taskHandler.RegisterAgentRoutes(agentGroup)
	}

	// Comment handler (shared across agent + user routes)
	commentHandler := comment.NewHandler(database)
	commentHandler.RegisterAgentRoutes(agentGroup)

	// User routes (JWT auth)
	user := r.Group("/api")
	user.Use(authMiddleware)
	{
		projectHandler := project.NewHandler(database)
		featureHandler := feature.NewHandler(database)
		taskHandler := task.NewHandler(database, wsHub)
		taskHandler.WorkflowAdvancer = wfEngine
		reviewHandler := review.NewHandler(database)
		agentHandler := agent.NewHandler(database)

		projectHandler.RegisterRoutes(user)
		featureHandler.RegisterRoutes(user)
		taskHandler.RegisterUserRoutes(user)
		commentHandler.RegisterUserRoutes(user)
		reviewHandler.RegisterRoutes(user)
		agentHandler.RegisterUserRoutes(user)

		// Workflow engine routes
		workflow.RegisterRoutes(r, database, wfEngine)
	}

	// Start health monitor
	go agent.StartHealthMonitor(database, 5*time.Minute)

	// Start stale task monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go task.StartStaleTaskMonitor(ctx, database, wsHub)

	log.Printf("AgentHub starting on :%s", port)

	// Serve React app from web/dist/
	serveApp(r)

	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func serveApp(r *gin.Engine) {
	distDir := "./web/dist"
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		filePath := distDir + path
		// Check if file exists, if not serve index.html for SPA routing
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			c.File(distDir + "/index.html")
			return
		}
		c.File(filePath)
	})
	log.Println("[app] React SPA served from web/dist")
}
