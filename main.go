package main

import (
	"fmt"
	"time"
)


func main() {
	sub := getGpsSub()
	for {
		time.Sleep(LOOP_DELAY)
		location, success := sub.Read() 
		if !success {
			continue
		}
		fmt.Printf("location: %v, %v\n", location.Latitude(), location.Longitude())
	}

}
