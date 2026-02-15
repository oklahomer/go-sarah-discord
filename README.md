# go-sarah-discord

[![CI](https://github.com/oklahomer/go-sarah-discord/actions/workflows/ci.yml/badge.svg)](https://github.com/oklahomer/go-sarah-discord/actions/workflows/ci.yml)
[![Coverage Status](https://coveralls.io/repos/github/oklahomer/go-sarah-discord/badge.svg?branch=main)](https://coveralls.io/github/oklahomer/go-sarah-discord?branch=main)

A [go-sarah](https://github.com/oklahomer/go-sarah) adapter for [Discord](https://discord.com/).

This adapter bridges go-sarah's bot framework with Discord, using [discordgo](https://github.com/bwmarrin/discordgo) for the underlying Discord API integration.

## Prerequisites

- Go 1.25+
- A Discord bot token (see [Creating a Bot Account](https://discordgo.readthedocs.io/en/latest/getting_started/01-getting_started/#creating-a-bot-account))

When creating your bot in the [Discord Developer Portal](https://discord.com/developers/applications), ensure the following are enabled under **Bot** settings:
- **Message Content Intent** (required to read message content)

## Installation

```bash
go get github.com/oklahomer/go-sarah-discord
```

## Quick Start

A complete working example is available in [\_example/main.go](./_example/main.go).

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/oklahomer/go-kasumi/logger"
	"github.com/oklahomer/go-sarah/v4"
	"github.com/oklahomer/go-sarah-discord"
)

func main() {
	// Configure the adapter.
	config := discord.NewConfig()
	config.Token = os.Getenv("DISCORD_TOKEN")

	// Create the adapter and bot.
	adapter, err := discord.NewAdapter(config)
	if err != nil {
		logger.Errorf("Failed to create adapter: %+v", err)
		return
	}
	storage := sarah.NewUserContextStorage(sarah.NewCacheConfig())
	bot := sarah.NewBot(adapter, sarah.BotWithStorage(storage))

	sarah.RegisterBot(bot)

	// Register a command.
	props := sarah.NewCommandPropsBuilder().
		BotType(discord.DISCORD).
		Identifier("hello").
		MatchPattern(regexp.MustCompile(`^\.hello`)).
		Func(func(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
			return discord.NewResponse(input, "Hello, World!")
		}).
		Instruction("Input .hello to receive a greeting.").
		MustBuild()
	sarah.RegisterCommandProps(props)

	// Run with graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	sarah.Run(ctx, sarah.NewConfig())
	<-ctx.Done()
}
```

Run with:

```bash
export DISCORD_TOKEN="your-bot-token"
go run .
```

## Configuration

`discord.Config` can be populated via `json.Unmarshal`, `yaml.Unmarshal`, or set manually.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Token` | `string` | `""` | Discord bot token (required) |
| `HelpCommand` | `string` | `".help"` | Message that triggers help listing |
| `AbortCommand` | `string` | `".abort"` | Message that cancels conversational context |
| `Intents` | `discordgo.Intent` | Guild + DM + MessageContent | Gateway intents for the bot |

## Architecture

```
Discord Server
    |
    v
discordgo.Session  <--- handles WebSocket, reconnection, rate limits
    |
    v
discord.Adapter    <--- converts Discord events to go-sarah primitives
    |
    v
go-sarah (Runner)  <--- manages bot lifecycle, command dispatch, workers
    |
    v
Your Commands      <--- business logic
```

- **discordgo** handles the low-level Discord API: WebSocket gateway connection, automatic reconnection, and rate limiting.
- **go-sarah-discord** (this package) acts as a bridge: it receives Discord message events and converts them into `sarah.Input`, and converts `sarah.Output` back to Discord API calls.
- **go-sarah** provides the framework: command matching, conversational context, scheduled tasks, and worker management.

## Advanced Usage

### Injecting a pre-configured session

If you need to configure the discordgo session directly (e.g., to set custom intents or add event handlers for non-message events), use `WithSession`:

```go
session, _ := discordgo.New("Bot " + token)
session.Identify.Intents = discordgo.IntentsAllWithoutPrivileged

adapter, _ := discord.NewAdapter(config, discord.WithSession(session))
```

### Conversational context

go-sarah supports multi-turn conversations. Use `discord.RespWithNext` to set a continuation function:

```go
func askName(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
	return discord.NewResponse(input, "What is your name?",
		discord.RespWithNext(receiveName))
}

func receiveName(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
	name := input.Message()
	return discord.NewResponse(input, "Hello, "+name+"!")
}
```

### Sending rich messages

Pass a `*discordgo.MessageSend` as the command response content for embeds, components, or other rich content:

```go
func richCommand(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
	return &sarah.CommandResponse{
		Content: &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "Rich Embed",
					Description: "This is a rich embed message.",
					Color:       0x00ff00,
				},
			},
		},
	}, nil
}
```
