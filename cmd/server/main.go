package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/tuyen/agenthub/internal/agent"
	"github.com/tuyen/agenthub/internal/auth"
	"github.com/tuyen/agenthub/internal/dashboard"
	"github.com/tuyen/agenthub/internal/db"
	"github.com/tuyen/agenthub/internal/feature"
	"github.com/tuyen/agenthub/internal/project"
	"github.com/tuyen/agenthub/internal/review"
	"github.com/tuyen/agenthub/internal/task"
	"github.com/tuyen/agenthub/middleware"
	"golang.org/x/crypto/bcrypt"
)

func main() {
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
	// Only hash password if admin doesn't exist yet
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

	// Request logging (first — before any processing)
	r.Use(middleware.Logging())
	// Rate limiting
	// r.Use(middleware.RateLimit()) // Disabled: rate limiter too aggressive for localhost testing
	// CORS
	r.Use(middleware.CORS())

	// Middleware
	authMiddleware := auth.NewMiddleware(jwtSecret)
	agentMiddleware := auth.NewAgentMiddleware(database)

	// Health
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Public routes (user auth + agent registration)
	public := r.Group("/api")
	{
		authHandler := auth.NewHandler(database, jwtSecret)
		agentHandler := agent.NewHandler(database)
		public.GET("/hello", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "hello from agenthub", "version": "1.1"})
		})
		public.POST("/auth/register", authHandler.Register)
		public.POST("/auth/login", authHandler.Login)
		public.POST("/agent/register", agentHandler.RegisterAgent)
	}

	// Agent routes (API key auth) — registration excluded (chicken-and-egg)
	agentGroup := r.Group("/api/agent")
	agentGroup.Use(agentMiddleware)
	{
		agentHandler := agent.NewHandler(database)
		taskHandler := task.NewHandler(database)

		agentHandler.RegisterRoutes(agentGroup)
		agentHandler.RegisterAgentRoutes(agentGroup)
		taskHandler.RegisterAgentRoutes(agentGroup)
	}

	// User routes (JWT auth)
	user := r.Group("/api")
	user.Use(authMiddleware)
	{
		projectHandler := project.NewHandler(database)
		featureHandler := feature.NewHandler(database)
		taskHandler := task.NewHandler(database)
		reviewHandler := review.NewHandler(database)
		dashboardHandler := dashboard.NewHandler(database)
		agentHandler := agent.NewHandler(database)

		projectHandler.RegisterRoutes(user)
		featureHandler.RegisterRoutes(user)
		taskHandler.RegisterUserRoutes(user)
		reviewHandler.RegisterRoutes(user)
		dashboardHandler.RegisterRoutes(user)
		agentHandler.RegisterUserRoutes(user)
	}

	// Start health monitor
	go agent.StartHealthMonitor(database, 5*time.Minute)

	log.Printf("AgentHub starting on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
