package main

import (
	"log/slog"
)

func check(e error) {
	if e != nil {
		slog.Error("Unexpected Error", "error", e)
		panic(e)
	}
}

func loge(e error) {
	if e != nil {
		slog.Error("", "error", e)
	}
}

func logwe(e error) {
	if e != nil {
		slog.Warn("", "error", e)
	}
}

func logie(e error) {
	if e != nil {
		slog.Info("", "error", e)
	}
}

func logde(e error) {
	if e != nil {
		slog.Debug("", "error", e)
	}
}
