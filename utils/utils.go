package utils

import (
	"log/slog"
)

func Check(e error) {
	if e != nil {
		slog.Error("Unexpected Error", "error", e)
		panic(e)
	}
}

func Loge(e error) {
	if e != nil {
		slog.Error("", "error", e)
	}
}

func Logwe(e error) {
	if e != nil {
		slog.Warn("", "error", e)
	}
}

func Logie(e error) {
	if e != nil {
		slog.Info("", "error", e)
	}
}

func Logde(e error) {
	if e != nil {
		slog.Debug("", "error", e)
	}
}
