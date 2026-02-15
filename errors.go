package discord

import "errors"

// ErrEmptyToken indicates that no token was provided and no session was injected via WithSession.
var ErrEmptyToken = errors.New("token must be set or a session must be provided via WithSession")

// ErrNoAuthor indicates that the given message has no author.
var ErrNoAuthor = errors.New("message has no author")
