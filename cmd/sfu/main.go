package main

import (
	"log/slog"
	"voice-chat-sfu/internal/app"
)

func main() {
	a := app.New()

	if err := a.Run(); err != nil {
		slog.Error("application error", "err", err)
	}
}
