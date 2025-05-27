# AI Debate Agent

A Golang application that connects to a Kafka/Redpanda network and participates in debates using OpenAI's GPT-3.5 Turbo. Multiple agents can debate in the same topic, with each user able to customize their agent's behavior through the terminal interface.

## Features

- Connect to Kafka/Redpanda for message exchange
- Uses OpenAI GPT-3.5 Turbo to generate debate responses
- Decide when to respond based on debate context
- Automatically respond if no messages for a set timeout period
- Terminal UI for viewing debates and interacting with your agent
- Customizable debate styles and agent personalities
- Support for multiple (unlimited) agents debating in the same topic
- Automatic topic generation for new debates

## ToDo
- Replace debate chat completion with agentic AI that can use MCP server tools to reinforce arguments
- Create a fact checking agent with agentic AI that can use MCP server tools to verify arguments
- Create Python and Redpanda Connect ports of this demo to further demonstrate a distributed AI agents communicating with each other

## Requirements

- Go 1.16 or higher
- Kafka/Redpanda cluster
- OpenAI API key

## Configuration

Create a `.env` file based on the provided `.env.example`:

```
# Kafka/Redpanda Configuration
SEED_BROKERS=0.0.0.0:19092
TOPICS=debate
CONSUMER_GROUP=transactions-consumer
SASL_MECHANISM=SCRAM-SHA-256
SASL_USERNAME=USERNAME-HERE
SASL_PASSWORD=SECRET-PASSWORD-HERE
TLS_ENABLED=false

# OpenAI Configuration
OPENAI_API_KEY=YOUR-OPENAI-API-KEY-HERE
```

## Usage

1. Project setup:
   ```
   go mod init github.com/chandler767/Distributed-AI-Demo
   go mod tidy
   ```

2. Build the application:
   ```
   go build -o debate-agent ./cmd
   ```

3. Run the application:
   ```
   ./debate-agent
   ```

4. Multiple agents can be started in different terminals to participate in the same debate.

## Terminal Commands

While the agent is running, you can use the following commands:

- `style <style>` - Change debate style (socratic, adversarial, collaborative, analytical, philosophical, humorous, devil's advocate, pragmatic)
- `personality <description>` - Add personality traits to your agent
- `timeout <seconds>` - Set response timeout in seconds
- `new` - Start a new debate topic

## Navigation

- `Tab` - Switch focus between message view and input
- `PgUp/PgDn` - Scroll through message history
- `Home/End` - Jump to beginning/end of message history

## How It Works

1. Each agent connects to the specified Kafka topic with a unique consumer group
2. When started, agents look for existing debates in the topic
3. If a debate exists, agents join it; otherwise, the first agent creates a new debate topic
4. Agents use OpenAI to decide when to respond and what to say
5. If no agent has responded for the timeout period, an agent will generate a response
6. Users can customize their agent's behavior through terminal commands

## Multi-Agent Support

The application is designed to support multiple agents debating in the same topic:

- Each agent instance uses a unique consumer group to receive all messages
- Agents detect existing debates and join them automatically
- Debate topics are shared across all connected agents
- Each agent has its own personality and debate style

## Troubleshooting

- If messages aren't appearing in the terminal, check your connection details
- If multiple agents start new debates instead of joining existing ones, ensure they're using the same topic
- For any connection issues, verify your Redpanda cluster is running and accessible