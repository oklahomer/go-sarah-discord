// This is an example bot that demonstrates how to use go-sarah-discord.
// It registers three commands: echo, hello, and description.
//
// Usage:
//
//	export DISCORD_TOKEN="your-bot-token"
//	go run .
//
// Then, in a Discord channel where the bot is present, type:
//
//	.echo Hello, World!
//	.hello
//	.description
//	.help
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/oklahomer/go-kasumi/logger"
	"github.com/oklahomer/go-sarah/v4"

	"github.com/oklahomer/go-sarah-discord"
)

func main() {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "DISCORD_TOKEN environment variable is required")
		os.Exit(1)
	}

	// Set up the Discord adapter configuration.
	config := discord.NewConfig()
	config.Token = token

	// Create the adapter.
	adapter, err := discord.NewAdapter(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create adapter: %s\n", err)
		os.Exit(1)
	}

	// Create a Bot with the adapter and an in-memory user context storage
	// for conversational state management.
	storage := sarah.NewUserContextStorage(sarah.NewCacheConfig())
	bot := sarah.NewBot(adapter, sarah.BotWithStorage(storage))

	// Register the bot with go-sarah.
	sarah.RegisterBot(bot)

	// Register example commands.
	registerEchoCommand()
	registerHelloCommand()
	registerDescriptionCommand()

	// Set up a context that cancels on SIGINT or SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start go-sarah's lifecycle management.
	err = sarah.Run(ctx, sarah.NewConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to run: %s\n", err)
		os.Exit(1)
	}

	logger.Infof("Bot is running. Press Ctrl+C to stop.")

	// Block until shutdown signal.
	<-ctx.Done()

	logger.Infof("Shutting down...")
}

var echoPattern = regexp.MustCompile(`^\.echo`)

func registerEchoCommand() {
	props := sarah.NewCommandPropsBuilder().
		BotType(discord.DISCORD).
		Identifier("echo").
		MatchPattern(echoPattern).
		Func(func(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
			msg := sarah.StripMessage(echoPattern, input.Message())
			if msg == "" {
				msg = "Usage: .echo <message>"
			}
			return discord.NewResponse(input, msg)
		}).
		Instruction("Input .echo <message> to have the bot echo your message back.").
		MustBuild()

	sarah.RegisterCommandProps(props)
}

func registerHelloCommand() {
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
}

var descriptionPattern = regexp.MustCompile(`^\.description`)

func registerDescriptionCommand() {
	props := sarah.NewCommandPropsBuilder().
		BotType(discord.DISCORD).
		Identifier("description").
		MatchPattern(descriptionPattern).
		Func(func(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
			return discord.NewResponse(input, &discordgo.MessageSend{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "go-sarah-discord",
						Description: "A Discord adapter for the go-sarah bot framework.",
						Color:       0x5865F2, // Discord blurple
						Fields: []*discordgo.MessageEmbedField{
							{Name: "Echo", Value: "`.echo <message>` — echoes your message back", Inline: false},
							{Name: "Hello", Value: "`.hello` — receive a greeting", Inline: false},
							{Name: "Description", Value: "`.description` — display this embed", Inline: false},
						},
						Footer: &discordgo.MessageEmbedFooter{
							Text: "Powered by go-sarah",
						},
					},
				},
			})
		}).
		Instruction("Input .description to display a rich embed message.").
		MustBuild()

	sarah.RegisterCommandProps(props)
}
