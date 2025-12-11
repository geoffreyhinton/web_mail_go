package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/geoffreyhinton/mail_go/api/handlers"
	"github.com/geoffreyhinton/mail_go/api/middleware"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Config struct {
	Port        string
	MongoURL    string
	DatabaseName string
}

func main() {
	config := &Config{
		Port:        getEnv("PORT", "8080"),
		MongoURL:    getEnv("MONGO_URL", "mongodb://localhost:27017"),
		DatabaseName: getEnv("DB_NAME", "wildmail"),
	}

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.MongoURL))
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer client.Disconnect(ctx)

	// Ping the database
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatal("Failed to ping MongoDB:", err)
	}

	db := client.Database(config.DatabaseName)
	log.Println("Connected to MongoDB database:", config.DatabaseName)

	// Initialize handlers
	userHandler := handlers.NewUserHandler(db)
	mailboxHandler := handlers.NewMailboxHandler(db)
	messageHandler := handlers.NewMessageHandler(db)
	addressHandler := handlers.NewAddressHandler(db)

	// Setup Gin router
	router := gin.Default()

	// Global middleware
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.ErrorHandling())
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// API routes
	api := router.Group("/api")
	{
		// User routes
		users := api.Group("/users")
		{
			users.GET("", userHandler.GetUsers)
			users.POST("", userHandler.CreateUser)
			users.GET("/:id", userHandler.GetUser)
			users.PUT("/:id", userHandler.UpdateUser)
			users.DELETE("/:id", userHandler.DeleteUser)

			// User addresses
			users.GET("/:id/addresses", addressHandler.GetUserAddresses)
			users.POST("/:id/addresses", addressHandler.CreateUserAddress)
			users.GET("/:id/addresses/:addressId", addressHandler.GetUserAddress)
			users.PUT("/:id/addresses/:addressId", addressHandler.UpdateUserAddress)
			users.DELETE("/:id/addresses/:addressId", addressHandler.DeleteUserAddress)

			// User mailboxes
			users.GET("/:id/mailboxes", mailboxHandler.GetUserMailboxes)
			users.POST("/:id/mailboxes", mailboxHandler.CreateMailbox)
			users.GET("/:id/mailboxes/:mailboxId", mailboxHandler.GetMailbox)
			users.PUT("/:id/mailboxes/:mailboxId", mailboxHandler.UpdateMailbox)
			users.DELETE("/:id/mailboxes/:mailboxId", mailboxHandler.DeleteMailbox)

			// Messages in mailbox
			users.GET("/:id/mailboxes/:mailboxId/messages", messageHandler.GetMessages)
			users.GET("/:id/mailboxes/:mailboxId/messages/:messageId", messageHandler.GetMessage)
			users.PUT("/:id/mailboxes/:mailboxId/messages/:messageId", messageHandler.UpdateMessage)
			users.DELETE("/:id/mailboxes/:mailboxId/messages/:messageId", messageHandler.DeleteMessage)

			// Message attachments
			users.GET("/:id/mailboxes/:mailboxId/messages/:messageId/attachments/:attachmentId", messageHandler.GetAttachment)

			// Search messages
			users.GET("/:id/search", messageHandler.SearchMessages)

			// Quota management
			users.POST("/:id/quota/reset", userHandler.ResetUserQuota)
		}

		// Address routes
		addresses := api.Group("/addresses")
		{
			addresses.GET("", addressHandler.GetAddresses)
		}
	}

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "timestamp": time.Now().Unix()})
	})

	// Start server
	srv := &http.Server{
		Addr:    ":" + config.Port,
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	log.Printf("Mail API server started on port %s", config.Port)

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}