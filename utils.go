package main

import (
	"github.com/rs/zerolog/log"
)

func check(e error) {
	if e != nil {
		log.Error().Stack().Err(e).Msg("")
		panic(e)
	}
}

func loge(e error) {
	if e != nil {
		log.Error().Stack().Err(e).Msg("")
	}
}

func logwe(e error) {
	if e != nil {
		log.Warn().Stack().Err(e).Msg("")
	}
}

func logie(e error) {
	if e != nil {
		log.Info().Stack().Err(e).Msg("")
	}
}

func logde(e error) {
	if e != nil {
		log.Debug().Stack().Err(e).Msg("")
	}
}
