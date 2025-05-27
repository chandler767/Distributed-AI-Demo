package kafka

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chandler767/Distributed-AI-Demo/pkg/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// Client represents a Kafka client
type Client struct {
	client *kgo.Client
	config *config.KafkaConfig
}

// Message represents a Kafka message
type Message struct {
	Topic   string
	Key     string
	Value   string
	Headers map[string]string
}

// NewClient creates a new Kafka client
func NewClient(cfg *config.KafkaConfig) (*Client, error) {
	log.Printf("Creating Kafka client with brokers: %v, topics: %v", cfg.SeedBrokers, cfg.Topics)

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.SeedBrokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.Topics...),
		kgo.DisableAutoCommit(),
		// Add connection resilience options
		kgo.RetryTimeout(time.Minute * 2),
		kgo.RetryBackoffFn(func(attempt int) time.Duration {
			return time.Second * time.Duration(attempt)
		}),
	}

	// Add SASL if configured
	if cfg.SASL.Mechanism != "" {
		log.Printf("Configuring SASL authentication with mechanism: %s", cfg.SASL.Mechanism)
		var mechanism sasl.Mechanism

		switch cfg.SASL.Mechanism {
		case "SCRAM-SHA-256":
			mechanism = scram.Sha256(func(ctx context.Context) (scram.Auth, error) {
				return scram.Auth{
					User: cfg.SASL.Username,
					Pass: cfg.SASL.Password,
				}, nil
			})
		case "SCRAM-SHA-512":
			mechanism = scram.Sha512(func(ctx context.Context) (scram.Auth, error) {
				return scram.Auth{
					User: cfg.SASL.Username,
					Pass: cfg.SASL.Password,
				}, nil
			})
		default:
			return nil, fmt.Errorf("unsupported SASL mechanism: %s", cfg.SASL.Mechanism)
		}

		opts = append(opts, kgo.SASL(mechanism))
	}

	// Add TLS if enabled
	if cfg.TLS.Enabled {
		log.Printf("Enabling TLS for Kafka connection")
		opts = append(opts, kgo.DialTLS())
	}

	// Create client with retry logic
	var client *kgo.Client
	var err error

	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		client, err = kgo.NewClient(opts...)
		if err == nil {
			break
		}

		log.Printf("Failed to create Kafka client (attempt %d/%d): %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			backoff := time.Duration(i+1) * time.Second
			log.Printf("Retrying in %v...", backoff)
			time.Sleep(backoff)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka client after %d attempts: %w", maxRetries, err)
	}

	log.Printf("Kafka client created successfully")
	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// Consume consumes messages from Kafka
func (c *Client) Consume(ctx context.Context, handler func(Message) error) error {
	log.Printf("Starting to consume messages from topics: %v", c.config.Topics)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Context canceled, stopping Kafka consumer")
			return ctx.Err()
		default:
			fetches := c.client.PollFetches(ctx)
			if errs := fetches.Errors(); len(errs) > 0 {
				// Log errors but continue consuming
				for _, err := range errs {
					log.Printf("Error consuming from Kafka: %v", err)
				}
				continue
			}

			numRecords := fetches.NumRecords()
			if numRecords > 0 {
				log.Printf("Received %d records from Kafka", numRecords)
			}

			fetches.EachRecord(func(record *kgo.Record) {
				headers := make(map[string]string)
				for _, header := range record.Headers {
					headers[header.Key] = string(header.Value)
				}

				msg := Message{
					Topic:   record.Topic,
					Key:     string(record.Key),
					Value:   string(record.Value),
					Headers: headers,
				}

				if err := handler(msg); err != nil {
					log.Printf("Error handling message: %v", err)
				}

				// Commit after processing
				c.client.MarkCommitRecords(record)
			})

			// Commit any marked records
			if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
				log.Printf("Error committing offsets: %v", err)
			}

			// If no records were fetched, sleep briefly to avoid CPU spinning
			if fetches.NumRecords() == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
}

// Produce produces a message to Kafka
func (c *Client) Produce(ctx context.Context, msg Message) error {
	log.Printf("Producing message to topic %s", msg.Topic)

	record := &kgo.Record{
		Topic: msg.Topic,
		Key:   []byte(msg.Key),
		Value: []byte(msg.Value),
	}

	// Add headers if any
	for k, v := range msg.Headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{
			Key:   k,
			Value: []byte(v),
		})
	}

	// Produce the record with retry logic
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := c.client.ProduceSync(ctx, record).FirstErr()
		if err == nil {
			log.Printf("Message successfully produced to topic %s", msg.Topic)
			return nil
		}

		log.Printf("Failed to produce message (attempt %d/%d): %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			backoff := time.Duration(i+1) * 500 * time.Millisecond
			log.Printf("Retrying in %v...", backoff)
			time.Sleep(backoff)
		} else {
			return fmt.Errorf("failed to produce message after %d attempts: %w", maxRetries, err)
		}
	}

	// This should never be reached due to the return in the loop above
	return fmt.Errorf("unexpected error in produce retry logic")
}

// Close closes the Kafka client
func (c *Client) Close() {
	log.Printf("Closing Kafka client")
	c.client.Close()
}
