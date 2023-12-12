package main

import (
	"archive/tar"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

type LocationData struct {
	BoundingBox Bounds `json:"bounding_box"`
	FullName    string `json:"full_name"`
}

//go:embed nation_bounding_boxes.json
var nationBoundingBoxesJson []byte

var (
	NATION_BOXES = map[string]LocationData{}
	_            = json.Unmarshal(nationBoundingBoxesJson, &NATION_BOXES)
)

//go:embed us_states_bounding_boxes.json
var statesBoundingBoxesJson []byte

var (
	STATE_BOXES = map[string]LocationData{}
	_           = json.Unmarshal(statesBoundingBoxesJson, &STATE_BOXES)
)

func DownloadFile(url string, filepath string) (err error) {
	log.Info().Msgf("Downloading: %s\n", url)
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return errors.Wrap(err, "could not create file for download")
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "could not download the file data")
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("download received bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return errors.Wrap(err, "could not write download data to file")
	}
	err = out.Sync()
	if err != nil {
		return errors.Wrap(err, "could not fsync downloaded file")
	}

	return nil
}

type Bounds struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

type DownloadLocations struct {
	Nations []string `json:"nations"`
	States  []string `json:"states"`
}

type DownloadProgress struct {
	TotalFiles          int                                `json:"total_files"`
	DownloadedFiles     int                                `json:"downloaded_files"`
	LocationsToDownload []string                           `json:"locations_to_download"`
	LocationDetails     map[string]*DownloadLocationDetail `json:"location_details"`
}

type DownloadLocationDetail struct {
	TotalFiles      int `json:"location_total_files"`
	DownloadedFiles int `json:"location_downloaded_files"`
}

var progress DownloadProgress

func AddLocationDetailsToProgress(locationNames []string, locationType string) {
	for _, locationName := range locationNames {
		if _, ok := progress.LocationDetails[locationName]; !ok {
			progress.LocationDetails[locationName] = &DownloadLocationDetail{
				TotalFiles: countTotalFiles([]string{locationName}, locationType),
			}
		}
	}
}

func DownloadIfTriggered() {
	progress = DownloadProgress{
		LocationsToDownload: []string{},
		LocationDetails:     map[string]*DownloadLocationDetail{},
	}

	b, err := GetParam(DOWNLOAD_LOCATIONS)
	logwe(err)
	if err == nil && len(b) != 0 {
		var locations DownloadLocations
		err = json.Unmarshal(b, &locations)
		logwe(err)

		progress.LocationsToDownload = append(locations.Nations, locations.States...)
		progress.TotalFiles = countTotalFiles(locations.Nations, "nation") + countTotalFiles(locations.States, "state")

		AddLocationDetailsToProgress(locations.Nations, "nation")
		AddLocationDetailsToProgress(locations.States, "state")

		if err == nil {
			for _, location := range locations.Nations {
				lData, ok := NATION_BOXES[location]
				if ok {
					log.Info().Msgf("downloading nation: %s", NATION_BOXES[location].FullName)
					err = DownloadBounds(lData.BoundingBox, location)
					if err != nil {
						logie(err)
					}
				} else {
					log.Warn().Msgf("no bounding box data for nation code: %s", location)
				}
			}
			for _, location := range locations.States {
				lData, ok := STATE_BOXES[location]
				if ok {
					log.Info().Msgf("downloading state: %s", STATE_BOXES[location].FullName)
					err = DownloadBounds(lData.BoundingBox, location)
					if err != nil {
						logie(err)
					}
				} else {
					log.Warn().Msgf("no bounding box data for state code: %s", location)
				}
			}
		}
	}
	err = PutParam(DOWNLOAD_LOCATIONS, []byte{})
	logwe(err)

	b, err = GetParam(DOWNLOAD_BOUNDS)
	logwe(err)
	if err == nil && len(b) != 0 {
		var bounds Bounds
		err = json.Unmarshal(b, &bounds)
		logde(err)

		if err == nil {
			err = DownloadBounds(bounds, "CUSTOM")
			logie(err)
			if err == nil {
				err = PutParam(DOWNLOAD_BOUNDS, []byte{})
				logwe(err)
			}
		}
	}
}

func adjustedBounds(bounds Bounds) (int, int, int, int) {
	minLat := int(math.Floor(bounds.MinLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	minLon := int(math.Floor(bounds.MinLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	maxLat := int(math.Floor(bounds.MaxLat/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES
	maxLon := int(math.Floor(bounds.MaxLon/float64(GROUP_AREA_BOX_DEGREES))) * GROUP_AREA_BOX_DEGREES

	if bounds.MaxLat > float64(maxLat) {
		maxLat += GROUP_AREA_BOX_DEGREES
	}
	if bounds.MaxLon > float64(maxLon) {
		maxLon += GROUP_AREA_BOX_DEGREES
	}
	return minLat, minLon, maxLat, maxLon
}

func DownloadBounds(bounds Bounds, locationName string) (err error) {
	log.Info().Msgf("Downloading Bounds: %f, %f, %f, %f\n", bounds.MinLat, bounds.MinLon, bounds.MaxLat, bounds.MaxLon)

	// clip given bounds to file areas
	minLat, minLon, maxLat, maxLon := adjustedBounds(bounds)
	progress.LocationDetails[locationName].TotalFiles = countFilesForBounds(bounds)

	for i := minLat; i < maxLat; i += GROUP_AREA_BOX_DEGREES {
		for j := minLon; j < maxLon; j += GROUP_AREA_BOX_DEGREES {
			filename := fmt.Sprintf("offline/%d/%d.tar.gz", i, j)
			url := fmt.Sprintf("https://map-data.pfeifer.dev/%s", filename)
			outputName := filepath.Join(GetBaseOpPath(), "tmp", filename)
			err := os.MkdirAll(filepath.Dir(outputName), 0o775)
			logde(errors.Wrap(err, "failed to make output directory"))
			err = DownloadFile(url, outputName)
			if err != nil {
				logwe(errors.Wrap(err, "failed to download file, continuing to next"))
				continue
			}
			file, err := os.Open(outputName)
			logde(errors.Wrap(err, "failed to open downloaded file"))
			reader, err := gzip.NewReader(file)
			logde(errors.Wrap(err, "failed to read downloaded gzip"))
			tr := tar.NewReader(reader)
			for {
				header, err := tr.Next()
				if err != nil {
					break
				}

				// if the header is nil, just skip it (not sure how this happens)
				if header == nil {
					continue
				}
				// the target location where the dir/file should be created
				target := filepath.Join(GetBaseOpPath(), header.Name)
				// check the file type
				switch header.Typeflag {

				// if its a dir and it doesn't exist create it
				case tar.TypeDir:
					if _, err := os.Stat(target); err != nil {
						err := os.MkdirAll(target, 0o755)
						logde(errors.Wrap(err, "could not create directory from zip"))
					}

				// if it's a file create it
				case tar.TypeReg:
					f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
					logde(errors.Wrap(err, "could not open file target for unzipped file"))

					_, err = io.Copy(f, tr)
					logde(errors.Wrap(err, "could not write unzipped data to target file"))

					err = f.Sync()
					logde(errors.Wrap(err, "could not fsync unzipped target file"))
					f.Close()
				}
			}
			err = reader.Close()
			logde(errors.Wrap(err, "could not close gzip reader"))
			err = file.Close()
			logde(errors.Wrap(err, "could not close downloaded file"))

			err = os.Remove(outputName)
			logde(errors.Wrap(err, "could not delete downloaded gzip file"))

			progress.DownloadedFiles++
			progress.LocationDetails[locationName].DownloadedFiles++

			progressData, err := json.Marshal(progress)
			if err != nil {
				logde(errors.Wrap(err, "could not marshal download progress"))
			}

			err = PutParam(DOWNLOAD_PROGRESS, progressData)
			if err != nil {
				logwe(errors.Wrap(err, "could not write download progress"))
			}
		}
	}
	err = os.RemoveAll(filepath.Join(GetBaseOpPath(), "tmp"))
	logde(errors.Wrap(err, "could not remove temp download directory"))

	log.Info().Msgf("Finished Downloading Bounds: %f, %f, %f, %f\n", bounds.MinLat, bounds.MinLon, bounds.MaxLat, bounds.MaxLon)
	return nil
}

func countFilesForBounds(bounds Bounds) int {
	minLat, minLon, maxLat, maxLon := adjustedBounds(bounds)
	return ((maxLat - minLat) / GROUP_AREA_BOX_DEGREES) * ((maxLon - minLon) / GROUP_AREA_BOX_DEGREES)
}

func countTotalFiles(allLocations []string, locationType string) int {
	totalFiles := 0

	var boxes map[string]LocationData
	if locationType == "nation" {
		boxes = NATION_BOXES
	} else if locationType == "state" {
		boxes = STATE_BOXES
	}

	for _, location := range allLocations {
		if lData, ok := boxes[location]; ok {
			totalFiles += countFilesForBounds(lData.BoundingBox)
		}
	}

	return totalFiles
}
