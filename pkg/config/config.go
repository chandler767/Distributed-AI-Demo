package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration
type Config struct {
	Kafka  KafkaConfig
	OpenAI OpenAIConfig
}

// KafkaConfig holds Kafka/Redpanda connection configuration
type KafkaConfig struct {
	SeedBrokers   []string
	Topics        []string
	ConsumerGroup string
	SASL          SASLConfig
	TLS           TLSConfig
}

// SASLConfig holds SASL authentication configuration
type SASLConfig struct {
	Mechanism string
	Username  string
	Password  string
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled bool
}

// OpenAIConfig holds OpenAI API configuration
type OpenAIConfig struct {
	APIKey string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		return nil, err
	}

	// Parse Kafka configuration
	seedBrokers := strings.Split(os.Getenv("SEED_BROKERS"), ",")
	topics := strings.Split(os.Getenv("TOPICS"), ",")
	consumerGroup := os.Getenv("CONSUMER_GROUP")
	
	// Parse SASL configuration
	saslMechanism := os.Getenv("SASL_MECHANISM")
	saslUsername := os.Getenv("SASL_USERNAME")
	saslPassword := os.Getenv("SASL_PASSWORD")
	
	// Parse TLS configuration
	tlsEnabled, _ := strconv.ParseBool(os.Getenv("TLS_ENABLED"))
	
	// Parse OpenAI configuration
	openAIAPIKey := os.Getenv("OPENAI_API_KEY")

	// Create and return config
	return &Config{
		Kafka: KafkaConfig{
			SeedBrokers:   seedBrokers,
			Topics:        topics,
			ConsumerGroup: consumerGroup,
			SASL: SASLConfig{
				Mechanism: saslMechanism,
				Username:  saslUsername,
				Password:  saslPassword,
			},
			TLS: TLSConfig{
				Enabled: tlsEnabled,
			},
		},
		OpenAI: OpenAIConfig{
			APIKey: openAIAPIKey,
		},
	}, nil
}
