package discord

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/oklahomer/go-sarah/v4"
)

// mockSession implements the session interface for testing.
type mockSession struct {
	addHandlerFunc         func(handler interface{}) func()
	openFunc               func() error
	closeFunc              func() error
	channelMessageSendFunc func(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	channelMessageSendComplexFunc func(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

func (m *mockSession) AddHandler(handler interface{}) func() {
	if m.addHandlerFunc != nil {
		return m.addHandlerFunc(handler)
	}
	return func() {}
}

func (m *mockSession) Open() error {
	if m.openFunc != nil {
		return m.openFunc()
	}
	return nil
}

func (m *mockSession) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockSession) ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.channelMessageSendFunc != nil {
		return m.channelMessageSendFunc(channelID, content, options...)
	}
	return &discordgo.Message{}, nil
}

func (m *mockSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	if m.channelMessageSendComplexFunc != nil {
		return m.channelMessageSendComplexFunc(channelID, data, options...)
	}
	return &discordgo.Message{}, nil
}

func TestBotTypeValue(t *testing.T) {
	if DISCORD != sarah.BotType("discord") {
		t.Errorf("Expected DISCORD to be %q, got %q", "discord", DISCORD)
	}
}

func TestNewAdapter(t *testing.T) {
	t.Run("with token", func(t *testing.T) {
		config := NewConfig()
		config.Token = "test-token"

		adapter, err := NewAdapter(config)
		if err != nil {
			t.Fatalf("Unexpected error: %+v", err)
		}

		if adapter == nil {
			t.Fatal("Expected non-nil adapter")
		}

		if adapter.config != config {
			t.Error("Config not set correctly")
		}

		if adapter.session == nil {
			t.Error("Expected session to be created")
		}
	})

	t.Run("without token and without session", func(t *testing.T) {
		config := NewConfig()

		_, err := NewAdapter(config)
		if err == nil {
			t.Fatal("Expected an error when no token and no session is given")
		}

		if err != ErrEmptyToken {
			t.Errorf("Expected ErrEmptyToken, got %+v", err)
		}
	})

	t.Run("with injected session", func(t *testing.T) {
		config := NewConfig()
		session := &discordgo.Session{}

		adapter, err := NewAdapter(config, WithSession(session))
		if err != nil {
			t.Fatalf("Unexpected error: %+v", err)
		}

		if adapter.session != session {
			t.Error("Expected injected session to be used")
		}
	})
}

func TestAdapter_BotType(t *testing.T) {
	adapter := &Adapter{config: NewConfig()}

	if adapter.BotType() != DISCORD {
		t.Errorf("Expected BotType to be %q, got %q", DISCORD, adapter.BotType())
	}
}

func TestAdapter_Run(t *testing.T) {
	t.Run("Open fails", func(t *testing.T) {
		mock := &mockSession{
			openFunc: func() error {
				return fmt.Errorf("connection refused")
			},
		}

		adapter := &Adapter{
			config:  NewConfig(),
			session: mock,
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var notifiedErr error
		notifyErr := func(err error) {
			notifiedErr = err
		}

		adapter.Run(ctx, func(input sarah.Input) error { return nil }, notifyErr)

		if notifiedErr == nil {
			t.Fatal("Expected notifyErr to be called when Open fails")
		}

		errStr := notifiedErr.Error()
		if !strings.Contains(errStr, "connection refused") {
			t.Errorf("Expected error to contain 'connection refused', got %q", errStr)
		}
	})

	t.Run("context canceled calls Close", func(t *testing.T) {
		var closeCalled bool
		mock := &mockSession{
			closeFunc: func() error {
				closeCalled = true
				return nil
			},
		}

		adapter := &Adapter{
			config:  NewConfig(),
			session: mock,
		}

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			adapter.Run(ctx, func(input sarah.Input) error { return nil }, func(err error) {})
			close(done)
		}()

		// Cancel context to unblock Run
		cancel()
		<-done

		if !closeCalled {
			t.Error("Expected Close to be called after context cancellation")
		}
	})

	t.Run("Close error is handled gracefully", func(t *testing.T) {
		mock := &mockSession{
			closeFunc: func() error {
				return fmt.Errorf("close failed")
			},
		}

		adapter := &Adapter{
			config:  NewConfig(),
			session: mock,
		}

		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			adapter.Run(ctx, func(input sarah.Input) error { return nil }, func(err error) {})
			close(done)
		}()

		cancel()
		<-done

		// Should not panic -- the error is logged internally
	})

	t.Run("AddHandler is called", func(t *testing.T) {
		var handlerRegistered bool
		mock := &mockSession{
			addHandlerFunc: func(handler interface{}) func() {
				handlerRegistered = true
				return func() {}
			},
			openFunc: func() error {
				return fmt.Errorf("stop here")
			},
		}

		adapter := &Adapter{
			config:  NewConfig(),
			session: mock,
		}

		ctx := context.Background()
		adapter.Run(ctx, func(input sarah.Input) error { return nil }, func(err error) {})

		if !handlerRegistered {
			t.Error("Expected AddHandler to be called")
		}
	})
}

func TestAdapter_handleMessage(t *testing.T) {
	botUserID := "bot-user-123"

	sessionWithState := &discordgo.Session{
		State: discordgo.NewState(),
	}
	sessionWithState.State.User = &discordgo.User{ID: botUserID}

	t.Run("regular message is enqueued as Input", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   "hello",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received == nil {
			t.Fatal("Expected input to be enqueued")
		}

		if _, ok := received.(*Input); !ok {
			t.Errorf("Expected *Input, got %T", received)
		}

		if received.Message() != "hello" {
			t.Errorf("Expected message %q, got %q", "hello", received.Message())
		}
	})

	t.Run("help command is wrapped as HelpInput", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   ".help",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received == nil {
			t.Fatal("Expected input to be enqueued")
		}

		if _, ok := received.(*sarah.HelpInput); !ok {
			t.Errorf("Expected *sarah.HelpInput, got %T", received)
		}
	})

	t.Run("abort command is wrapped as AbortInput", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   ".abort",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received == nil {
			t.Fatal("Expected input to be enqueued")
		}

		if _, ok := received.(*sarah.AbortInput); !ok {
			t.Errorf("Expected *sarah.AbortInput, got %T", received)
		}
	})

	t.Run("bot's own message is ignored", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   "hello from bot",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: botUserID}, // Same as bot user
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received != nil {
			t.Error("Bot's own message should be ignored")
		}
	})

	t.Run("help command with whitespace is still recognized", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   "  .help  ",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received == nil {
			t.Fatal("Expected input to be enqueued")
		}

		if _, ok := received.(*sarah.HelpInput); !ok {
			t.Errorf("Expected *sarah.HelpInput, got %T", received)
		}
	})

	t.Run("empty help command disables help detection", func(t *testing.T) {
		config := NewConfig()
		config.HelpCommand = ""
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   ".help",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received == nil {
			t.Fatal("Expected input to be enqueued")
		}

		// When HelpCommand is empty, ".help" should be treated as regular input
		if _, ok := received.(*Input); !ok {
			t.Errorf("Expected *Input (regular), got %T", received)
		}
	})

	t.Run("session without state does not panic", func(t *testing.T) {
		config := NewConfig()
		sessionNoState := &discordgo.Session{}
		adapter := &Adapter{config: config, session: sessionNoState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   "hello",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		adapter.handleMessage(sessionNoState, m, enqueue)

		if received == nil {
			t.Fatal("Expected input to be enqueued")
		}
	})

	t.Run("nil author is ignored", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		var received sarah.Input
		enqueue := func(input sarah.Input) error {
			received = input
			return nil
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   "hello",
				Timestamp: time.Now(),
				Author:    nil,
			},
		}

		adapter.handleMessage(sessionWithState, m, enqueue)

		if received != nil {
			t.Error("Message with nil Author should be ignored")
		}
	})

	t.Run("enqueue error is handled gracefully", func(t *testing.T) {
		config := NewConfig()
		adapter := &Adapter{config: config, session: sessionWithState}

		enqueue := func(input sarah.Input) error {
			return fmt.Errorf("queue full")
		}

		m := &discordgo.MessageCreate{
			Message: &discordgo.Message{
				ChannelID: "ch-1",
				Content:   "hello",
				Timestamp: time.Now(),
				Author:    &discordgo.User{ID: "user-1"},
			},
		}

		// Should not panic when enqueue returns an error
		adapter.handleMessage(sessionWithState, m, enqueue)
	})
}

func TestAdapter_SendMessage(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		var gotChannelID, gotContent string
		mock := &mockSession{
			channelMessageSendFunc: func(channelID, content string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				gotChannelID = channelID
				gotContent = content
				return &discordgo.Message{}, nil
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		output := sarah.NewOutputMessage(ChannelID("ch-1"), "hello world")
		adapter.SendMessage(context.Background(), output)

		if gotChannelID != "ch-1" {
			t.Errorf("Expected channelID %q, got %q", "ch-1", gotChannelID)
		}
		if gotContent != "hello world" {
			t.Errorf("Expected content %q, got %q", "hello world", gotContent)
		}
	})

	t.Run("string content with send error", func(t *testing.T) {
		mock := &mockSession{
			channelMessageSendFunc: func(channelID, content string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				return nil, fmt.Errorf("send failed")
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		output := sarah.NewOutputMessage(ChannelID("ch-1"), "hello")
		// Should not panic, just log the error
		adapter.SendMessage(context.Background(), output)
	})

	t.Run("MessageSend content", func(t *testing.T) {
		var gotChannelID string
		var gotData *discordgo.MessageSend
		mock := &mockSession{
			channelMessageSendComplexFunc: func(channelID string, data *discordgo.MessageSend, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				gotChannelID = channelID
				gotData = data
				return &discordgo.Message{}, nil
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		msg := &discordgo.MessageSend{Content: "complex msg"}
		output := sarah.NewOutputMessage(ChannelID("ch-2"), msg)
		adapter.SendMessage(context.Background(), output)

		if gotChannelID != "ch-2" {
			t.Errorf("Expected channelID %q, got %q", "ch-2", gotChannelID)
		}
		if gotData == nil || gotData.Content != "complex msg" {
			t.Error("Expected MessageSend to be passed through")
		}
	})

	t.Run("MessageSend content with send error", func(t *testing.T) {
		mock := &mockSession{
			channelMessageSendComplexFunc: func(channelID string, data *discordgo.MessageSend, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				return nil, fmt.Errorf("send failed")
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		msg := &discordgo.MessageSend{Content: "complex msg"}
		output := sarah.NewOutputMessage(ChannelID("ch-2"), msg)
		// Should not panic, just log the error
		adapter.SendMessage(context.Background(), output)
	})

	t.Run("CommandHelps content", func(t *testing.T) {
		var gotContent string
		mock := &mockSession{
			channelMessageSendFunc: func(channelID, content string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				gotContent = content
				return &discordgo.Message{}, nil
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		helps := &sarah.CommandHelps{
			{Identifier: "echo", Instruction: "Input .echo to echo back"},
			{Identifier: "hello", Instruction: "Input .hello to greet"},
		}
		output := sarah.NewOutputMessage(ChannelID("ch-3"), helps)
		adapter.SendMessage(context.Background(), output)

		if !strings.Contains(gotContent, "**echo**: Input .echo to echo back") {
			t.Errorf("Expected help text to contain echo, got %q", gotContent)
		}
		if !strings.Contains(gotContent, "**hello**: Input .hello to greet") {
			t.Errorf("Expected help text to contain hello, got %q", gotContent)
		}
	})

	t.Run("CommandHelps content with send error", func(t *testing.T) {
		mock := &mockSession{
			channelMessageSendFunc: func(channelID, content string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				return nil, fmt.Errorf("send failed")
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		helps := &sarah.CommandHelps{
			{Identifier: "echo", Instruction: "echo help"},
		}
		output := sarah.NewOutputMessage(ChannelID("ch-3"), helps)
		// Should not panic, just log the error
		adapter.SendMessage(context.Background(), output)
	})

	t.Run("invalid destination type", func(t *testing.T) {
		mock := &mockSession{
			channelMessageSendFunc: func(channelID, content string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				t.Error("ChannelMessageSend should not be called for invalid destination")
				return nil, nil
			},
			channelMessageSendComplexFunc: func(channelID string, data *discordgo.MessageSend, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				t.Error("ChannelMessageSendComplex should not be called for invalid destination")
				return nil, nil
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		output := sarah.NewOutputMessage("not-a-channel-id", "hello")
		adapter.SendMessage(context.Background(), output)
	})

	t.Run("unexpected content type", func(t *testing.T) {
		mock := &mockSession{
			channelMessageSendFunc: func(channelID, content string, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				t.Error("ChannelMessageSend should not be called for unexpected content")
				return nil, nil
			},
			channelMessageSendComplexFunc: func(channelID string, data *discordgo.MessageSend, opts ...discordgo.RequestOption) (*discordgo.Message, error) {
				t.Error("ChannelMessageSendComplex should not be called for unexpected content")
				return nil, nil
			},
		}
		adapter := &Adapter{config: NewConfig(), session: mock}

		output := sarah.NewOutputMessage(ChannelID("ch-1"), 12345) // int is unexpected
		adapter.SendMessage(context.Background(), output)
	})
}

func TestMessageToInput_NilAuthor(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "channel-123",
			Content:   "hello",
			Timestamp: time.Now(),
			Author:    nil,
		},
	}

	_, err := MessageToInput(m)
	if err == nil {
		t.Fatal("Expected error for nil Author")
	}

	if err != ErrNoAuthor {
		t.Errorf("Expected ErrNoAuthor, got %+v", err)
	}
}

func TestMessageToInput(t *testing.T) {
	now := time.Now()
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "channel-123",
			Content:   "hello world",
			Timestamp: now,
			Author: &discordgo.User{
				ID:       "user-456",
				Username: "testuser",
			},
		},
	}

	input, err := MessageToInput(m)
	if err != nil {
		t.Fatalf("Unexpected error: %+v", err)
	}

	t.Run("SenderKey", func(t *testing.T) {
		expected := "channel-123_user-456"
		if input.SenderKey() != expected {
			t.Errorf("Expected SenderKey %q, got %q", expected, input.SenderKey())
		}
	})

	t.Run("Message", func(t *testing.T) {
		if input.Message() != "hello world" {
			t.Errorf("Expected Message %q, got %q", "hello world", input.Message())
		}
	})

	t.Run("SentAt", func(t *testing.T) {
		if !input.SentAt().Equal(now) {
			t.Errorf("Expected SentAt %v, got %v", now, input.SentAt())
		}
	})

	t.Run("ReplyTo", func(t *testing.T) {
		dest, ok := input.ReplyTo().(ChannelID)
		if !ok {
			t.Fatal("ReplyTo should return ChannelID")
		}
		if string(dest) != "channel-123" {
			t.Errorf("Expected ReplyTo %q, got %q", "channel-123", string(dest))
		}
	})

	t.Run("Event preserved", func(t *testing.T) {
		if input.Event != m {
			t.Error("Original event should be preserved in Input")
		}
	})
}

func TestInput_SarahInputInterface(t *testing.T) {
	var sarahInput sarah.Input = &Input{
		senderKey: "key",
		text:      "text",
		sentAt:    time.Now(),
		channelID: ChannelID("ch"),
	}

	if sarahInput.SenderKey() != "key" {
		t.Errorf("Expected SenderKey %q, got %q", "key", sarahInput.SenderKey())
	}

	if sarahInput.Message() != "text" {
		t.Errorf("Expected Message %q, got %q", "text", sarahInput.Message())
	}
}

func TestNewResponse(t *testing.T) {
	t.Run("simple response", func(t *testing.T) {
		input := &Input{
			senderKey: "ch_user",
			text:      ".echo hello",
			sentAt:    time.Now(),
			channelID: ChannelID("ch"),
		}

		resp, err := NewResponse(input, "hello")
		if err != nil {
			t.Fatalf("Unexpected error: %+v", err)
		}

		if resp.Content != "hello" {
			t.Errorf("Expected content %q, got %v", "hello", resp.Content)
		}

		if resp.UserContext != nil {
			t.Error("Expected nil UserContext for simple response")
		}
	})

	t.Run("response with next", func(t *testing.T) {
		input := &Input{
			senderKey: "ch_user",
			text:      ".start",
			sentAt:    time.Now(),
			channelID: ChannelID("ch"),
		}

		nextFunc := func(ctx context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
			return &sarah.CommandResponse{Content: "next step"}, nil
		}

		resp, err := NewResponse(input, "step 1", RespWithNext(nextFunc))
		if err != nil {
			t.Fatalf("Unexpected error: %+v", err)
		}

		if resp.UserContext == nil {
			t.Fatal("Expected non-nil UserContext")
		}

		if resp.UserContext.Next == nil {
			t.Error("Expected non-nil UserContext.Next")
		}
	})

	t.Run("response with serializable next", func(t *testing.T) {
		input := &Input{
			senderKey: "ch_user",
			text:      ".start",
			sentAt:    time.Now(),
			channelID: ChannelID("ch"),
		}

		arg := &sarah.SerializableArgument{
			FuncIdentifier: "myFunc",
			Argument:       "arg",
		}

		resp, err := NewResponse(input, "step 1", RespWithNextSerializable(arg))
		if err != nil {
			t.Fatalf("Unexpected error: %+v", err)
		}

		if resp.UserContext == nil {
			t.Fatal("Expected non-nil UserContext")
		}

		if resp.UserContext.Serializable == nil {
			t.Error("Expected non-nil UserContext.Serializable")
		}

		if resp.UserContext.Serializable.FuncIdentifier != "myFunc" {
			t.Errorf("Expected FuncIdentifier %q, got %q", "myFunc", resp.UserContext.Serializable.FuncIdentifier)
		}
	})

	t.Run("non-discord input returns error", func(t *testing.T) {
		discordInput := &Input{
			senderKey: "ch_user",
			text:      ".help",
			sentAt:    time.Now(),
			channelID: ChannelID("ch"),
		}
		helpInput := sarah.NewHelpInput(discordInput)

		_, err := NewResponse(helpInput, "should fail")
		if err == nil {
			t.Fatal("Expected an error for non-discord Input")
		}
	})
}

func TestWithSession(t *testing.T) {
	session := &discordgo.Session{}
	adapter := &Adapter{}

	opt := WithSession(session)
	opt(adapter)

	if adapter.session != session {
		t.Error("WithSession should set the session on the adapter")
	}
}

func TestChannelID_OutputDestination(t *testing.T) {
	var dest sarah.OutputDestination = ChannelID("test")
	_ = dest

	chID := ChannelID("test-channel")
	if string(chID) != "test-channel" {
		t.Errorf("Expected %q, got %q", "test-channel", string(chID))
	}
}
