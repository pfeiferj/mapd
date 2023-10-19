package main

import (
	"fmt"
	"github.com/serjvanilla/go-overpass"
	"math"
)

func AsyncFetchRoadsAroundLocation(resChannel chan overpass.Result, errChannel chan error, lat float64, lon float64, radius float64) {
	res, err := FetchRoadsAroundLocation(lat, lon, radius)
	if err != nil {
		errChannel <- err
		return
	}
	resChannel <- res
}

func FetchRoadsAroundLocation(lat float64, lon float64, radius float64) (overpass.Result, error) {
	client := overpass.New()
	bbox_angle := (radius / R) * (180 / math.Pi)
	bbox_str := fmt.Sprintf("%f,%f,%f,%f", lat-bbox_angle, lon-bbox_angle, lat+bbox_angle, lon+bbox_angle)
	query_fmt := "[out:json];way(%s)\n" +
		"[highway]\n" +
		"[highway!~\"^(footway|path|corridor|bridleway|steps|cycleway|construction|bus_guideway|escape|service|track)$\"];\n" +
		"(._;>;);\n" +
		"out;\n" +
		"is_in(%f,%f);area._[admin_level~\"[24]\"];\n" +
		"convert area ::id = id(), admin_level = t['admin_level'],\n" +
		"name = t['name'], \"ISO3166-1:alpha2\" = t['ISO3166-1:alpha2'];out;"
	query := fmt.Sprintf(query_fmt, bbox_str, lat, lon)
	result, err := client.Query(query)
	return result, err
}
