package main

import (
	"flag"
	"log"
	"os"

	"wire-socket-auth/internal/api"
	"wire-socket-auth/internal/database"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Config represents the auth service configuration
type Config struct {
	Server struct {
		Address string `yaml:"address"`
	} `yaml:"server"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	Auth struct {
		JWTSecret   string `yaml:"jwt_secret"`
		MasterToken string `yaml:"master_token"` // For tunnel registration
	} `yaml:"auth"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	initDB := flag.Bool("init-db", false, "Initialize database with default admin user")
	flag.Parse()

	// Load config
	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := database.NewDB(config.Database.Path)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Auto-migrate
	if err := db.AutoMigrate(); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Init admin user if requested
	if *initDB {
		passwordHash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}
		if err := db.InitAdmin(string(passwordHash)); err != nil {
			log.Fatalf("Failed to init admin: %v", err)
		}
		log.Println("Database initialized successfully")
		return
	}

	// Set master token
	api.SetMasterToken(config.Auth.MasterToken)

	// Setup Gin
	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()

	// Setup routes
	router := api.NewRouter(db, config.Auth.JWTSecret)
	router.SetupRoutes(engine)

	// Serve static admin UI
	engine.Static("/admin", "./internal/admin/static")

	// Start server
	log.Printf("Starting auth server on %s", config.Server.Address)
	if err := engine.Run(config.Server.Address); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.Server.Address == "" {
		config.Server.Address = ":8080"
	}
	if config.Database.Path == "" {
		config.Database.Path = "auth.db"
	}
	if config.Auth.JWTSecret == "" {
		config.Auth.JWTSecret = "change-this-secret"
	}

	return &config, nil
}
