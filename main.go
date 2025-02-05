package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

type AppState struct {
	MsgClient *messaging.Client
}

type PublishInput struct {
	Token        string       `json:"to"`
	Notification Notification `json:"notification"`
}

type BroadCastInput struct {
	Topic        string       `json:"topic"`
	Notification Notification `json:"notification"`
}

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type SubscribeInput struct {
	Tokens []string `json:"tokens"`
	Topic  string   `json:"topic"`
}

func main() {
	envErr := godotenv.Load()
	if envErr != nil {
		log.Fatal("Cannot read .env file", "error", envErr)
	}
	app, err := firebase.NewApp(context.Background(), nil)
	if err != nil {
		log.Fatal("error while starting app", "error", err)
	}

	log.Info("started app")

	ctx := context.Background()
	client, err := app.Messaging(ctx)

	state := &AppState{MsgClient: client}

	if err != nil {
		log.Fatal("Error getting messaging client", "error", err)
	}

	router := gin.Default()
	router.Use(APIKeyAuthMiddleware())
	router.Use(StateMiddleware(state))
	router.POST("/publish", publishDryRun)
	router.POST("/broadcast", BroadcastMsg)
	router.POST("/subscribe", SubscribeToTopic)
	router.POST("/unsubscribe", UnsubscribeFromTopic)
	router.Run("0.0.0.0:42069")
}

func StateMiddleware(state *AppState) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("state", state)
		c.Next()
	}
}

func publishDryRun(ctx *gin.Context) {
	var p PublishInput
	ctx.Bind(&p)
	registrationToken := p.Token
	notification := messaging.Notification{Title: p.Notification.Title, Body: p.Notification.Body}
	log.Info(fmt.Sprintf("notification is %v", notification))
	message := &messaging.Message{
		Notification: &notification,
		Token:        registrationToken,
	}

	appState, _ := ctx.Get("state")
	state := appState.(*AppState)

	response, err := state.MsgClient.Send(ctx, message)
	if err != nil {
		log.Error("error sending message", "error", err)
		ctx.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("error found while publishing message: %s", err)})
		return
	}
	log.Info(fmt.Sprintf("Successfully sent message: %v", response))
	ctx.Status(http.StatusAccepted)
}

func BroadcastMsg(c *gin.Context) {
	var b BroadCastInput
	c.Bind(&b)
	notification := messaging.Notification{Title: b.Notification.Title, Body: b.Notification.Body}
	message := &messaging.Message{
		Notification: &notification,
		Topic:        b.Topic,
	}

	appState, _ := c.Get("state")
	state := appState.(*AppState)
	response, err := state.MsgClient.Send(c, message)
	if err != nil {
		log.Error("error broadcasting message", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("error found while broadcasting message: %s", err)})
		return
	}
	log.Info("Successfully broadcasted message", "resp", response)
	c.Status(http.StatusAccepted)
}

func SubscribeToTopic(c *gin.Context) {
	var s SubscribeInput
	c.Bind(&s)

	appState, _ := c.Get("state")
	state := appState.(*AppState)
	response, err := state.MsgClient.SubscribeToTopic(c, s.Tokens, s.Topic)
	if err != nil {
		log.Error("error while subscribing to topic", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("error found while subscribing to topic: %s", err)})
		return
	}
	if response.FailureCount != 0 {
		var sb strings.Builder
		for _, err := range response.Errors {
			fmt.Fprintf(&sb, "Code: %d, Message: %s", err.Index, err.Reason)
		}
		log.Error("error while subscribing to topic", "errors", sb.String())
		c.JSON(http.StatusBadGateway, gin.H{"errors": fmt.Sprintf("errors while subscribing to topic: %v", sb.String())})
		return
	}
	log.Info("Successfully subbed to topic", "resp", response)
	c.Status(http.StatusAccepted)
}

func UnsubscribeFromTopic(c *gin.Context) {
	var s SubscribeInput
	c.Bind(&s)

	appState, _ := c.Get("state")
	state := appState.(*AppState)
	response, err := state.MsgClient.UnsubscribeFromTopic(c, s.Tokens, s.Topic)
	if err != nil {
		log.Error("error while unsubscribing from topic", "error", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("error found while unsubscribing from topic: %s", err)})
		return
	}
	if response.FailureCount != 0 {
		var sb strings.Builder
		for _, err := range response.Errors {
			fmt.Fprintf(&sb, "Code: %d, Message: %s", err.Index, err.Reason)
		}
		log.Error("error while subscribing to topic", "errors", sb.String())
		c.JSON(http.StatusBadGateway, gin.H{"errors": fmt.Sprintf("errors while subscribing to topic: %v", sb.String())})
		return
	}
	log.Info("Successfully unsubbed from topic", "resp", response)
	c.Status(http.StatusAccepted)
}

func APIKeyAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Missing or invalid token"})
			c.Abort()
			return
		}

		appApiKey := os.Getenv("API_KEY")

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")
		if apiKey != appApiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid API Key"})
			c.Abort()
			return
		}

		c.Next()
	}
}
