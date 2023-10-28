package main

import (
	"log"
)

func check(e error) {
	if e != nil {
		log.Println(e)
		panic(e)
	}
}

func loge(e error) {
	if e != nil {
		log.Println(e)
	}
}
