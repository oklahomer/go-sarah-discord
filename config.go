package discord

import "github.com/bwmarrin/discordgo"

// Config contains configuration variables for the Discord Adapter.
type Config struct {
	// Token is the Discord bot token used for authentication.
	Token string `json:"token" yaml:"token"`

	// HelpCommand is the command string that triggers help.
	// When a user sends this exact string, the input is converted to sarah.HelpInput.
	HelpCommand string `json:"help_command" yaml:"help_command"`

	// AbortCommand is the command string that triggers context cancellation.
	// When a user sends this exact string, the input is converted to sarah.AbortInput.
	AbortCommand string `json:"abort_command" yaml:"abort_command"`

	// Intents declares the Gateway Intents the bot requires.
	Intents discordgo.Intent `json:"intents" yaml:"intents"`
}

// NewConfig creates and returns a new Config instance with default settings.
// Token is empty and must be set before use.
func NewConfig() *Config {
	return &Config{
		Token:        "",
		HelpCommand:  ".help",
		AbortCommand: ".abort",
		Intents:      discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentsMessageContent,
	}
}
