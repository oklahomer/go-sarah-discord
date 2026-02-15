package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/oklahomer/go-kasumi/logger"
	"github.com/oklahomer/go-sarah/v4"
)

const (
	// DISCORD is a designated sarah.BotType for Discord integration.
	DISCORD sarah.BotType = "discord"
)

// session is an internal interface that abstracts the discordgo.Session methods
// used by the Adapter. This allows mocking the session in tests.
// *discordgo.Session satisfies this interface.
type session interface {
	AddHandler(handler interface{}) func()
	Open() error
	Close() error
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

// ChannelID represents a Discord channel as sarah.OutputDestination.
type ChannelID string

var _ sarah.OutputDestination = ChannelID("")

// AdapterOption defines a function signature for Adapter's functional options.
type AdapterOption func(adapter *Adapter)

// WithSession creates an AdapterOption with the given *discordgo.Session.
// Use this to inject a pre-configured session.
// If this option is not given, NewAdapter creates a new session from Config.Token.
func WithSession(session *discordgo.Session) AdapterOption {
	return func(adapter *Adapter) {
		adapter.session = session
	}
}

// Adapter is a sarah.Adapter implementation for Discord.
type Adapter struct {
	config  *Config
	session session
}

var _ sarah.Adapter = (*Adapter)(nil)

// NewAdapter creates a new Adapter with the given Config and options.
func NewAdapter(config *Config, options ...AdapterOption) (*Adapter, error) {
	adapter := &Adapter{
		config: config,
	}

	for _, opt := range options {
		opt(adapter)
	}

	if adapter.session == nil {
		if config.Token == "" {
			return nil, ErrEmptyToken
		}

		s, err := discordgo.New("Bot " + config.Token)
		if err != nil {
			return nil, fmt.Errorf("failed to create Discord session: %w", err)
		}
		s.Identify.Intents = config.Intents
		adapter.session = s
	}

	return adapter, nil
}

// BotType returns a designated BotType for Discord integration.
func (a *Adapter) BotType() sarah.BotType {
	return DISCORD
}

// Run establishes a connection with Discord and blocks until the context is canceled.
func (a *Adapter) Run(ctx context.Context, enqueueInput func(sarah.Input) error, notifyErr func(error)) {
	a.session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		a.handleMessage(s, m, enqueueInput)
	})

	err := a.session.Open()
	if err != nil {
		notifyErr(sarah.NewBotNonContinuableError(fmt.Sprintf("failed to open Discord session: %s", err.Error())))
		return
	}

	// Block until the context is canceled.
	<-ctx.Done()

	if closeErr := a.session.Close(); closeErr != nil {
		logger.Errorf("Failed to close Discord session: %+v", closeErr)
	}
}

// handleMessage processes an incoming Discord message and routes it to enqueueInput.
func (a *Adapter) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate, enqueueInput func(sarah.Input) error) {
	input, err := MessageToInput(m)
	if err != nil {
		// MessageToInput returns ErrNoAuthor for system messages with no author.
		logger.Debugf("Skipping message: %+v", err)
		return
	}

	// Ignore messages from the bot itself.
	if s.State != nil && s.State.User != nil && m.Author.ID == s.State.User.ID {
		return
	}

	var enqueueErr error
	trimmed := strings.TrimSpace(input.Message())
	if a.config.HelpCommand != "" && trimmed == a.config.HelpCommand {
		enqueueErr = enqueueInput(sarah.NewHelpInput(input))
	} else if a.config.AbortCommand != "" && trimmed == a.config.AbortCommand {
		enqueueErr = enqueueInput(sarah.NewAbortInput(input))
	} else {
		enqueueErr = enqueueInput(input)
	}
	if enqueueErr != nil {
		logger.Errorf("Failed to enqueue input: %+v", enqueueErr)
	}
}

// SendMessage sends the given message to Discord.
func (a *Adapter) SendMessage(_ context.Context, output sarah.Output) {
	destination, ok := output.Destination().(ChannelID)
	if !ok {
		logger.Errorf("Destination is not instance of ChannelID. %#v.", output.Destination())
		return
	}

	channelID := string(destination)

	switch content := output.Content().(type) {
	case string:
		_, err := a.session.ChannelMessageSend(channelID, content)
		if err != nil {
			logger.Errorf("Failed to send message to %s: %+v", channelID, err)
		}

	case *discordgo.MessageSend:
		_, err := a.session.ChannelMessageSendComplex(channelID, content)
		if err != nil {
			logger.Errorf("Failed to send complex message to %s: %+v", channelID, err)
		}

	case *sarah.CommandHelps:
		lines := make([]string, 0, len(*content))
		for _, h := range *content {
			lines = append(lines, fmt.Sprintf("**%s**: %s", h.Identifier, h.Instruction))
		}
		text := strings.Join(lines, "\n")
		_, err := a.session.ChannelMessageSend(channelID, text)
		if err != nil {
			logger.Errorf("Failed to send help message to %s: %+v", channelID, err)
		}

	default:
		logger.Warnf("Unexpected output %#v", output)
	}
}

// Input is a sarah.Input implementation that represents a received Discord message.
type Input struct {
	Event     *discordgo.MessageCreate
	senderKey string
	text      string
	sentAt    time.Time
	channelID ChannelID
}

var _ sarah.Input = (*Input)(nil)

// SenderKey returns a unique key representing the sender in the channel.
func (i *Input) SenderKey() string {
	return i.senderKey
}

// Message returns the received text.
func (i *Input) Message() string {
	return i.text
}

// SentAt returns when the message was sent.
func (i *Input) SentAt() time.Time {
	return i.sentAt
}

// ReplyTo returns the Discord channel where the message was received.
func (i *Input) ReplyTo() sarah.OutputDestination {
	return i.channelID
}

// MessageToInput converts a *discordgo.MessageCreate event to *Input.
func MessageToInput(m *discordgo.MessageCreate) (*Input, error) {
	if m.Author == nil {
		return nil, ErrNoAuthor
	}

	return &Input{
		Event:     m,
		senderKey: fmt.Sprintf("%s_%s", m.ChannelID, m.Author.ID),
		text:      m.Content,
		sentAt:    m.Timestamp,
		channelID: ChannelID(m.ChannelID),
	}, nil
}

// NewResponse creates a *sarah.CommandResponse with the given message.
// Pass RespOption values to customize the response.
func NewResponse(input sarah.Input, message string, options ...RespOption) (*sarah.CommandResponse, error) {
	if _, ok := input.(*Input); !ok {
		return nil, fmt.Errorf("%T is not a *discord.Input", input)
	}

	stash := &respOptions{}
	for _, opt := range options {
		opt(stash)
	}

	return &sarah.CommandResponse{
		Content:     message,
		UserContext: stash.userContext,
	}, nil
}

// RespOption defines a function signature that NewResponse's functional options must satisfy.
type RespOption func(*respOptions)

type respOptions struct {
	userContext *sarah.UserContext
}

// RespWithNext sets a given function as part of the response's *sarah.UserContext.
// The next input from the same user is passed to this function.
func RespWithNext(fnc sarah.ContextualFunc) RespOption {
	return func(options *respOptions) {
		options.userContext = &sarah.UserContext{
			Next: fnc,
		}
	}
}

// RespWithNextSerializable sets the given argument as part of the response's *sarah.UserContext.
func RespWithNextSerializable(arg *sarah.SerializableArgument) RespOption {
	return func(options *respOptions) {
		options.userContext = &sarah.UserContext{
			Serializable: arg,
		}
	}
}
