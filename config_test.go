package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestNewConfig(t *testing.T) {
	config := NewConfig()

	if config.Token != "" {
		t.Errorf("Expected empty token, got %q", config.Token)
	}

	if config.HelpCommand != ".help" {
		t.Errorf("Expected HelpCommand to be %q, got %q", ".help", config.HelpCommand)
	}

	if config.AbortCommand != ".abort" {
		t.Errorf("Expected AbortCommand to be %q, got %q", ".abort", config.AbortCommand)
	}

	expectedIntents := discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent
	if config.Intents != expectedIntents {
		t.Errorf("Expected Intents to be %d, got %d", expectedIntents, config.Intents)
	}
}
