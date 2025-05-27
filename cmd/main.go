package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/user/ai-debate-agent/pkg/agent"
	"github.com/user/ai-debate-agent/pkg/config"
	"github.com/user/ai-debate-agent/pkg/kafka"
	"github.com/user/ai-debate-agent/pkg/types"
	"github.com/user/ai-debate-agent/pkg/ui"
)

func main() {
	// Set random seed
	rand.Seed(time.Now().UnixNano())
	log.SetOutput(io.Discard)

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

// generateAgentID generates a unique agent ID
func generateAgentID() string {
	// Generate a random adjective
	adjectives := []string{
		"Curious", "Thoughtful", "Analytical", "Logical", "Creative",
		"Wise", "Eloquent", "Insightful", "Rational", "Skeptical",
		"Balanced", "Objective", "Critical", "Reflective", "Philosophical",
	}
	adjective := adjectives[rand.Intn(len(adjectives))]

	// Generate a random noun
	nouns := []string{
		"Thinker", "Scholar", "Philosopher", "Debater", "Reasoner",
		"Mind", "Intellect", "Sage", "Logician", "Analyst",
		"Orator", "Inquirer", "Examiner", "Investigator", "Theorist",
	}
	noun := nouns[rand.Intn(len(nouns))]

	// Generate a short UUID suffix
	uuidSuffix := strings.Split(uuid.New().String(), "-")[0]

	// Combine to form the agent ID
	return fmt.Sprintf("%s%s-%s", adjective, noun, uuidSuffix)
}
