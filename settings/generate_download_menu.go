package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/clip"
	"github.com/paulmach/orb/geojson"
)

// GenerateDownloadMenu adds archive rows using local Natural Earth country and state GeoJSON files.
func GenerateDownloadMenu(menuFile, countriesFile, statesFile, outputFile string) error {
	data, err := os.ReadFile(menuFile)
	if err != nil {
		return err
	}
	var menu DownloadMenu
	if err := json.Unmarshal(data, &menu); err != nil {
		return err
	}
	if menu == nil {
		return fmt.Errorf("%s: expected menu object", menuFile)
	}

	regions := make(map[string][]orb.Ring)
	for index, filename := range []string{countriesFile, statesFile} {
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		// Validate nullable objects before the GeoJSON decoder dereferences geometry.
		var collection struct {
			Features []*struct {
				Geometry *struct{} `json:"geometry"`
			} `json:"features"`
		}
		if err := json.Unmarshal(data, &collection); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		for featureIndex, feature := range collection.Features {
			if feature == nil {
				return fmt.Errorf("%s: feature %d is null", filename, featureIndex)
			}
			if feature.Geometry == nil {
				return fmt.Errorf("%s: feature %d has no geometry", filename, featureIndex)
			}
		}
		features, err := geojson.UnmarshalFeatureCollection(data)
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		for _, feature := range features.Features {
			code, _ := feature.Properties["ISO_A2_EH"].(string)
			path := "nation." + code
			if index == 1 {
				code, _ = feature.Properties["iso_3166_2"].(string)
				if !strings.HasPrefix(code, "US-") {
					continue
				}
				path = "us_state." + strings.TrimPrefix(code, "US-")
			}
			var polygons orb.MultiPolygon
			switch geometry := feature.Geometry.(type) {
			case orb.Polygon:
				polygons = orb.MultiPolygon{geometry}
			case orb.MultiPolygon:
				polygons = geometry
			default:
				return fmt.Errorf("%s: expected polygon geometry for %s", filename, path)
			}
			for _, polygon := range polygons {
				if len(polygon) > 0 {
					// Keep outer rings, including islands; holes conservatively retain extra archives.
					regions[path] = append(regions[path], polygon[0])
				}
			}
		}
	}
	for _, code := range []string{"PR", "VI", "MP", "AS"} {
		regions["us_state."+code] = regions["nation."+code]
	}
	regions["us_state.GM"] = regions["nation.GU"] // Existing menu key for Guam.

	for group, locations := range menu {
		for code, location := range locations {
			location.DownloadRows = nil
			// These source boundaries omit territory covered by the existing menu entries.
			if group == "nation" && (code == "MA" || code == "SO" || code == "UA") {
				locations[code] = location
				continue
			}
			fullCount := location.countFiles()
			rings := regions[group+"."+code]
			for _, row := range location.downloadRows() {
				for longitude := row[1]; longitude < row[2]; longitude += GROUP_AREA_BOX_DEGREES {
					cell := orb.Bound{
						Min: orb.Point{float64(longitude), float64(row[0])},
						Max: orb.Point{float64(longitude + GROUP_AREA_BOX_DEGREES), float64(row[0] + GROUP_AREA_BOX_DEGREES)},
					}
					cell = cell.Pad(0.05) // allow for coarse coastlines near archive edges
					for _, ring := range rings {
						if !cell.Intersects(ring.Bound()) || len(clip.Ring(cell, ring.Clone())) == 0 {
							continue
						}
						last := len(location.DownloadRows) - 1
						if last >= 0 && location.DownloadRows[last][0] == row[0] && location.DownloadRows[last][2] == longitude {
							location.DownloadRows[last][2] += GROUP_AREA_BOX_DEGREES
						} else {
							location.DownloadRows = append(location.DownloadRows, [3]int{row[0], longitude, longitude + GROUP_AREA_BOX_DEGREES})
						}
						break
					}
				}
			}
			if location.countFiles() == fullCount {
				location.DownloadRows = nil
			}
			locations[code] = location
		}
	}
	data, err = json.MarshalIndent(menu, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputFile, append(data, '\n'), 0o644)
}
