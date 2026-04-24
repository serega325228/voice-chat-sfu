package main

import (
	"log/slog"
	"voice-chat-sfu/internal/app"
)

func main() {
	a, err := app.New()
	if err != nil {
		slog.Error("failed to initialize application", "err", err)
		return
	}

	if err := a.Run(); err != nil {
		slog.Error("application error", "err", err)
	}
}
