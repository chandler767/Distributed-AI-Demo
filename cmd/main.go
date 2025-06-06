package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chandler767/Distributed-AI-Demo/pkg/agent"
	"github.com/chandler767/Distributed-AI-Demo/pkg/config"
	"github.com/chandler767/Distributed-AI-Demo/pkg/kafka"
	"github.com/chandler767/Distributed-AI-Demo/pkg/types"
	"github.com/chandler767/Distributed-AI-Demo/pkg/ui"
	"github.com/google/uuid"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Ensure we have at least one topic configured
	if len(cfg.Kafka.Topics) == 0 {
		log.Fatalf("No Kafka topics configured. Please set TOPICS in .env file")
	}

	// Use the first topic from the configuration
	topicName := cfg.Kafka.Topics[0]
	log.Printf("Using Kafka topic: %s", topicName)

	// Generate a unique agent ID
	agentID := generateAgentID()

	// Use agentID as the log file name
	logFileName := fmt.Sprintf("%s.log", agentID)
	// Ensure logs directory exists
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("Failed to create logs directory: %v", err)
	}
	logFilePath := fmt.Sprintf("%s/%s", logDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)

	// Create a unique consumer group for this agent instance
	// This ensures each agent receives all messages
	uniqueConsumerGroup := fmt.Sprintf("%s-%s", cfg.Kafka.ConsumerGroup, agentID)
	cfg.Kafka.ConsumerGroup = uniqueConsumerGroup
	log.Printf("Using unique consumer group: %s", uniqueConsumerGroup)

	// Create Kafka client
	kafkaClient, err := kafka.NewClient(&cfg.Kafka)
	if err != nil {
		log.Fatalf("Failed to create Kafka client: %v", err)
	}

	// Create agent with random debate style
	debateStyle := types.AllDebateStyles()[rand.Intn(len(types.AllDebateStyles()))]
	agentCfg := agent.AgentConfig{
		ID:              agentID,
		Style:           debateStyle,
		Personality:     "", // Empty by default, can be set by user
		OpenAIAPIKey:    cfg.OpenAI.APIKey,
		KafkaClient:     kafkaClient,
		ResponseTimeout: 60 * time.Second,
		TopicName:       topicName,
	}
	debateAgent := agent.NewAgent(agentCfg)

	// Create UI
	ui := ui.NewUI(debateAgent)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Start the UI
	log.Printf("Starting AI debate agent with ID: %s and style: %s on topic: %s", agentID, debateStyle, topicName)
	if err := ui.Start(ctx); err != nil {
		log.Fatalf("UI error: %v", err)
	}
}

// generateAgentID generates a unique agent ID with a panda or redpanda theme
func generateAgentID() string {
	// Panda or Redpanda themed adjectives
	adjectives := []string{
		"Playful", "Gentle", "Bamboo", "Cuddly", "Red",
		"Chubby", "Curious", "Lazy", "Fluffy", "Mischievous",
		"Peaceful", "Adorable", "Sleepy", "Friendly", "Cheerful",
	}
	// Panda or Redpanda themed nouns
	nouns := []string{
		"Panda", "Redpanda", "Cub", "Bear", "BambooEater",
		"TreeClimber", "Napster", "Furball", "Snuggler", "LeafLover",
		"ForestDweller", "PandaPal", "BambooBuddy", "PandaSage", "RedTail",
	}
	adjective := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	uuidSuffix := strings.Split(uuid.New().String(), "-")[0]
	return fmt.Sprintf("%s%s-%s", adjective, noun, uuidSuffix)
}
