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
		slog.Error("Error Occurred", "error", e)
	}
}

func logwe(e error) {
	if e != nil {
		slog.Warn("Error Occurred", "error", e)
	}
}

func logie(e error) {
	if e != nil {
		slog.Info("Error Occurred", "error", e)
	}
}

func logde(e error) {
	if e != nil {
		slog.Debug("Error Occurred", "error", e)
	}
}
