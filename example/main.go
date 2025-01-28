package main

import (
	"log/slog"
	"os"

	"github.com/go-stdx/trash"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug.Level()})))
	_, err := trash.Put("./xxx")
	if err != nil {
		panic(err)
	}
}
