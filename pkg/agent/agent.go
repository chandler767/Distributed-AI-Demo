package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/chandler767/Distributed-AI-Demo/pkg/kafka"
	"github.com/chandler767/Distributed-AI-Demo/pkg/types"

	"github.com/sashabaranov/go-openai"
)

// Agent represents an AI debate agent
type Agent struct {
	ID              string
	Style           types.DebateStyle
	Personality     string
	OpenAIClient    *openai.Client
	KafkaClient     *kafka.Client
	LastMessageTime time.Time
	ResponseTimeout time.Duration
	DebateHistory   []types.DebateMessage
	DebateTopic     string
	TopicName       string // The Kafka topic name
	mu              sync.Mutex
	JoinedDebate    bool // Flag to track if agent has joined an existing debate
	messagesChan    chan types.DebateMessage
	commandsChan    chan string
	commandFeedback chan types.DebateMessage // Channel for command feedback
}

// AgentConfig holds configuration for an agent
type AgentConfig struct {
	ID              string
	Style           types.DebateStyle
	Personality     string
	OpenAIAPIKey    string
	KafkaClient     *kafka.Client
	ResponseTimeout time.Duration
	TopicName       string // The Kafka topic name
}

// NewAgent creates a new agent
func NewAgent(cfg AgentConfig) *Agent {
	if cfg.Style == "" {
		cfg.Style = RandomDebateStyle()
	}

	if cfg.ResponseTimeout == 0 {
		cfg.ResponseTimeout = 60 * time.Second
	}

	return &Agent{
		ID:              cfg.ID,
		Style:           cfg.Style,
		Personality:     cfg.Personality,
		OpenAIClient:    openai.NewClient(cfg.OpenAIAPIKey),
		KafkaClient:     cfg.KafkaClient,
		LastMessageTime: time.Now(),
		ResponseTimeout: cfg.ResponseTimeout,
		DebateHistory:   []types.DebateMessage{},
		TopicName:       cfg.TopicName,
		JoinedDebate:    false,
		commandFeedback: make(chan types.DebateMessage, 10),
	}
}

// RandomDebateStyle returns a random debate style
func RandomDebateStyle() types.DebateStyle {
	styles := types.AllDebateStyles()
	return styles[rand.Intn(len(styles))]
}

// Start starts the agent
func (a *Agent) Start(ctx context.Context, messagesChan chan types.DebateMessage, commandsChan chan string) error {
	log.Printf("Starting agent with ID %s on topic %s", a.ID, a.TopicName)

	// Store channels for later use
	a.messagesChan = messagesChan
	a.commandsChan = commandsChan

	// Forward command feedback to the UI
	go func() {
		for feedback := range a.commandFeedback {
			a.messagesChan <- feedback
		}
	}()

	// Start consuming messages from Kafka
	go func() {
		err := a.KafkaClient.Consume(ctx, func(msg kafka.Message) error {
			if msg.Topic != a.TopicName {
				log.Printf("Received message from topic %s, expected %s", msg.Topic, a.TopicName)
				return nil
			}

			var debateMsg types.DebateMessage
			if err := json.Unmarshal([]byte(msg.Value), &debateMsg); err != nil {
				return fmt.Errorf("failed to unmarshal debate message: %w", err)
			}

			log.Printf("Received message from %s: %s", debateMsg.AgentID, debateMsg.Content)

			// Update last message time and debate history
			a.mu.Lock()
			a.LastMessageTime = time.Now()
			a.DebateHistory = append(a.DebateHistory, debateMsg)

			// Extract debate topic if it's the first message and we don't have one
			if a.DebateTopic == "" && len(a.DebateHistory) == 1 {
				a.DebateTopic = extractDebateTopic(debateMsg.Content)
				log.Printf("Detected debate topic: %s", a.DebateTopic)
				a.JoinedDebate = true
			}

			// If we receive a message and haven't joined a debate yet, mark as joined
			if !a.JoinedDebate && len(a.DebateHistory) > 0 {
				log.Printf("Joining existing debate with %d messages", len(a.DebateHistory))
				a.JoinedDebate = true
			}
			a.mu.Unlock()

			// Send message to UI
			a.messagesChan <- debateMsg

			// Decide whether to respond - only if the message is not from this agent
			if debateMsg.AgentID != a.ID {
				go a.decideToRespond(ctx)
			}

			return nil
		})
		if err != nil && err != context.Canceled {
			log.Printf("Error consuming messages: %v", err)
		}
	}()

	// Start processing user commands
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case cmd, ok := <-commandsChan:
				if !ok {
					return
				}
				a.processCommand(ctx, cmd)
			}
		}
	}()

	// Start a timer to check for response timeout
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.checkResponseTimeout(ctx)
			}
		}
	}()

	// If no debate is ongoing, start one after a delay
	go func() {
		// Wait longer to give time to discover existing debates
		time.Sleep(10 * time.Second)

		a.mu.Lock()
		historyEmpty := len(a.DebateHistory) == 0
		hasJoinedDebate := a.JoinedDebate
		a.mu.Unlock()

		if historyEmpty && !hasJoinedDebate {
			log.Printf("No existing debate found after 10 seconds, starting a new one")
			a.startNewDebate(ctx)
		} else {
			log.Printf("Found existing debate or already joined one, not starting a new debate")
		}
	}()

	return nil
}

// decideToRespond decides whether to respond to the debate
func (a *Agent) decideToRespond(ctx context.Context) {
	a.mu.Lock()
	// Don't respond to our own messages
	if len(a.DebateHistory) > 0 && a.DebateHistory[len(a.DebateHistory)-1].AgentID == a.ID {
		a.mu.Unlock()
		return
	}

	// Get the debate history
	history := make([]types.DebateMessage, len(a.DebateHistory))
	copy(history, a.DebateHistory)
	a.mu.Unlock()

	// Ask OpenAI whether to respond
	shouldRespond, err := a.askOpenAIShouldRespond(ctx, history)
	if err != nil {
		log.Printf("Error asking OpenAI if agent should respond: %v", err)
		return
	}

	if shouldRespond {
		log.Printf("Agent %s decided to respond to the debate", a.ID)
		// Generate response
		response, err := a.generateResponse(ctx, history)
		if err != nil {
			log.Printf("Error generating response: %v", err)
			return
		}

		// Send response
		a.sendResponse(ctx, response)
	} else {
		log.Printf("Agent %s decided not to respond at this time", a.ID)
	}
}

// checkResponseTimeout checks if it's time to respond due to timeout
func (a *Agent) checkResponseTimeout(ctx context.Context) {
	a.mu.Lock()

	// If there's no debate history, don't respond
	if len(a.DebateHistory) == 0 {
		a.mu.Unlock()
		return
	}

	// If the last message was from us, don't respond
	if a.DebateHistory[len(a.DebateHistory)-1].AgentID == a.ID {
		a.mu.Unlock()
		return
	}

	// Check if timeout has elapsed
	timeoutElapsed := time.Since(a.LastMessageTime) > a.ResponseTimeout

	// Get a copy of the debate history
	history := make([]types.DebateMessage, len(a.DebateHistory))
	copy(history, a.DebateHistory)

	a.mu.Unlock()

	// If the timeout has elapsed, respond
	if timeoutElapsed {
		log.Printf("Response timeout reached, generating response")
		// Generate response
		response, err := a.generateResponse(ctx, history)
		if err != nil {
			log.Printf("Error generating response: %v", err)
			return
		}

		// Send response
		a.sendResponse(ctx, response)
	}
}

// askOpenAIShouldRespond asks OpenAI whether the agent should respond
func (a *Agent) askOpenAIShouldRespond(ctx context.Context, history []types.DebateMessage) (bool, error) {
	a.mu.Lock()
	debateTopic := a.DebateTopic
	style := a.Style
	personality := a.Personality
	a.mu.Unlock()

	// Prepare the prompt
	prompt := fmt.Sprintf(`You are an AI debate agent with the following style: %s
Additional personality traits: %s

Current debate topic: %s

Recent debate history:
%s

Should you respond to this debate now? Consider:
1. Do you have something valuable to add?
2. Is your perspective needed?
3. Has enough time passed since your last message?

Respond with only "yes" or "no".`,
		style,
		personality,
		debateTopic,
		formatDebateHistory(history),
	)

	// Create the OpenAI request
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo, // Using GPT-3.5 Turbo as requested
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
		},
		MaxTokens:   10,
		Temperature: 0.7,
	}

	// Call OpenAI
	resp, err := a.OpenAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return false, fmt.Errorf("failed to call OpenAI: %w", err)
	}

	// Parse the response
	response := strings.ToLower(strings.TrimSpace(resp.Choices[0].Message.Content))
	return response == "yes", nil
}

// generateResponse generates a response to the debate
// Todo replace with agent framework for responses
func (a *Agent) generateResponse(ctx context.Context, history []types.DebateMessage) (string, error) {
	a.mu.Lock()
	debateTopic := a.DebateTopic
	style := a.Style
	personality := a.Personality
	a.mu.Unlock()

	// Prepare the prompt
	prompt := fmt.Sprintf(`You are an AI debate agent with the following style: %s
Additional personality traits: %s

Current debate topic: %s

Recent debate history:
%s

Generate a thoughtful response for the debate. Your response should:
1. Be in character with your debate style
2. Add value to the conversation
3. Be concise but substantive
4. Avoid repeating points already made
5. Be engaging and thought-provoking
6. Directly respond to points made by others when appropriate

Your response:`,
		style,
		personality,
		debateTopic,
		formatDebateHistory(history),
	)

	// Create the OpenAI request
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo, // Using GPT-3.5 Turbo as requested
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
		},
		MaxTokens:   500,
		Temperature: 0.8,
	}

	// Call OpenAI
	resp, err := a.OpenAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

// sendResponse sends a response to the debate
func (a *Agent) sendResponse(ctx context.Context, content string) {
	// Create the message
	msg := types.DebateMessage{
		AgentID:   a.ID,
		Content:   content,
		Timestamp: time.Now(),
	}

	// Marshal the message
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}

	log.Printf("Sending message to topic %s: %s", a.TopicName, content)

	// Send the message to Kafka
	err = a.KafkaClient.Produce(ctx, kafka.Message{
		Topic: a.TopicName,
		Key:   a.ID,
		Value: string(msgBytes),
	})
	if err != nil {
		log.Printf("Error sending message: %v", err)
		return
	}

	// Update the debate history
	a.mu.Lock()
	a.DebateHistory = append(a.DebateHistory, msg)
	a.LastMessageTime = time.Now()
	a.JoinedDebate = true
	a.mu.Unlock()
}

// processCommand processes a user command
func (a *Agent) processCommand(ctx context.Context, cmd string) {
	// Parse the command
	parts := strings.SplitN(cmd, " ", 2)
	if len(parts) == 0 {
		return
	}

	var feedback string

	// Handle help command
	if strings.ToLower(parts[0]) == "help" {
		helpText := `
Available Commands:
- style <style> - Change debate style (socratic, adversarial, collaborative, analytical, philosophical, humorous, devil's advocate, pragmatic)
- personality <description> - Add personality traits to your agent
- timeout <seconds> - Set response timeout in seconds
- new - Start a new debate topic
- help - Show this help message
Navigation:
- Tab - Switch focus between message view and input
- PgUp/PgDn - Scroll through message history
- Home/End - Jump to beginning/end of message history
`
		feedbackMsg := types.DebateMessage{
			AgentID:   "System",
			Content:   helpText,
			Timestamp: time.Now(),
		}

		// Send to the command feedback channel
		a.commandFeedback <- feedbackMsg
		return
	}

	a.mu.Lock()

	switch strings.ToLower(parts[0]) {
	case "style":
		if len(parts) < 2 {
			feedback = "Usage: style <debate_style>"
		} else {
			style := types.DebateStyle(strings.ToLower(parts[1]))
			a.Style = style
			feedback = fmt.Sprintf("Debate style set to: %s", style)
			log.Printf("Debate style set to: %s", style)
		}

	case "personality":
		if len(parts) < 2 {
			feedback = "Usage: personality <description>"
		} else {
			a.Personality = parts[1]
			feedback = fmt.Sprintf("Personality set to: %s", a.Personality)
			log.Printf("Personality set to: %s", a.Personality)
		}

	case "timeout":
		if len(parts) < 2 {
			feedback = "Usage: timeout <seconds>"
		} else {
			seconds := 60
			fmt.Sscanf(parts[1], "%d", &seconds)
			a.ResponseTimeout = time.Duration(seconds) * time.Second
			feedback = fmt.Sprintf("Response timeout set to: %s", a.ResponseTimeout)
			log.Printf("Response timeout set to: %s", a.ResponseTimeout)
		}

	case "new":
		feedback = "Starting a new debate topic..."
		a.mu.Unlock() // Unlock before starting new debate
		a.startNewDebate(ctx)
		return

	default:
		feedback = fmt.Sprintf("Unknown command: %s. Type 'help' to see available commands.", parts[0])
	}

	a.mu.Unlock()

	// Send feedback as a system message to the UI
	feedbackMsg := types.DebateMessage{
		AgentID:   "System",
		Content:   feedback,
		Timestamp: time.Now(),
	}

	// Send to the command feedback channel
	a.commandFeedback <- feedbackMsg
}

// startNewDebate starts a new debate
func (a *Agent) startNewDebate(ctx context.Context) {
	// Generate a new debate topic
	topic, err := a.generateDebateTopic(ctx)
	if err != nil {
		log.Printf("Error generating debate topic: %v", err)
		return
	}

	a.mu.Lock()
	a.DebateTopic = topic
	a.DebateHistory = []types.DebateMessage{}
	a.JoinedDebate = true
	a.mu.Unlock()

	// Send the first message
	content := fmt.Sprintf("Let's debate a new topic: %s", topic)
	a.sendResponse(ctx, content)
}

// generateDebateTopic generates a new debate topic
func (a *Agent) generateDebateTopic(ctx context.Context) (string, error) {
	// Prepare the prompt
	prompt := `Generate an interesting, thought-provoking debate topic. The topic should be:
1. Engaging and open-ended
2. Suitable for multiple perspectives
3. Not overly political or divisive
4. Intellectually stimulating

Respond with only the debate topic as a single sentence.`

	// Create the OpenAI request
	req := openai.ChatCompletionRequest{
		Model: openai.GPT3Dot5Turbo, // Using GPT-3.5 Turbo as requested
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: prompt,
			},
		},
		MaxTokens:   100,
		Temperature: 1.0,
	}

	// Call OpenAI
	resp, err := a.OpenAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

// formatDebateHistory formats the debate history for use in prompts
func formatDebateHistory(history []types.DebateMessage) string {
	var sb strings.Builder

	// Limit to last 10 messages to avoid token limits
	startIdx := 0
	if len(history) > 10 {
		startIdx = len(history) - 10
	}

	for i := startIdx; i < len(history); i++ {
		msg := history[i]
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n",
			msg.Timestamp.Format("15:04:05"),
			msg.AgentID,
			msg.Content,
		))
	}

	return sb.String()
}

// extractDebateTopic extracts a debate topic from a message
func extractDebateTopic(content string) string {
	// Look for common patterns like "Let's debate: X" or "New topic: X"
	prefixes := []string{
		"Let's debate a new topic: ",
		"Let's debate: ",
		"New topic: ",
		"Today's debate: ",
		"The topic is: ",
	}

	for _, prefix := range prefixes {
		if strings.Contains(content, prefix) {
			return strings.TrimSpace(strings.Split(content, prefix)[1])
		}
	}

	// If no pattern is found, just return the content
	return content
}
