package settings

import (
	"archive/tar"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"pfeifer.dev/mapd/params"
	"pfeifer.dev/mapd/utils"
)

type LocationData struct {
	BoundingBox Bounds `json:"bounding_box"`
	FullName    string `json:"full_name"`
	Submenu     string `json:"submenu"`
}

//go:embed download_menu.json
var boundingBoxesJson []byte

var (
	BOUNDING_BOXES = map[string]map[string]LocationData{}
	_              = json.Unmarshal(boundingBoxesJson, &BOUNDING_BOXES)
)

func DownloadFile(url string, filepath string) (err error) {
	slog.Info("Downloading", "url", url)
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

func AddLocationDetailsToProgress(path string) {
	progress.LocationDetails[path] = &DownloadLocationDetail{
		TotalFiles: countFilesForBounds(getBoundsForPath(path)),
	}
}

func Download(paths string) {
	slog.Info("download", "paths", paths)
	progress = DownloadProgress{
		LocationsToDownload: []string{},
		LocationDetails:     map[string]*DownloadLocationDetail{},
	}

	pathsSplit := strings.Split(paths, ",")
	progress.LocationsToDownload = pathsSplit
	progress.TotalFiles = countTotalFiles(pathsSplit)
	for _, p := range pathsSplit {
		AddLocationDetailsToProgress(p)
		location := getDataForPath(p)
		slog.Info("downloading nation", "nation", location.FullName)
		err := DownloadBounds(location.BoundingBox, p)
		if err != nil {
			utils.Logie(err)
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
	slog.Info("Downloading Bounds", "min_lat", bounds.MinLat, "min_lon", bounds.MinLon, "max_lat", bounds.MaxLat, "max_lon", bounds.MaxLon)

	// clip given bounds to file areas
	minLat, minLon, maxLat, maxLon := adjustedBounds(bounds)
	progress.LocationDetails[locationName].TotalFiles = countFilesForBounds(bounds)

	for i := minLat; i < maxLat; i += GROUP_AREA_BOX_DEGREES {
		for j := minLon; j < maxLon; j += GROUP_AREA_BOX_DEGREES {
			filename := fmt.Sprintf("offline/%d/%d.tar.gz", i, j)
			url := fmt.Sprintf("https://map-data.pfeifer.dev/%s", filename)
			outputName := filepath.Join(params.GetBaseOpPath(), "tmp", filename)
			err := os.MkdirAll(filepath.Dir(outputName), 0o775)
			utils.Logde(errors.Wrap(err, "failed to make output directory"))
			err = DownloadFile(url, outputName)
			if err != nil {
				utils.Logwe(errors.Wrap(err, "failed to download file, continuing to next"))
				continue
			}
			file, err := os.Open(outputName)
			utils.Logde(errors.Wrap(err, "failed to open downloaded file"))
			reader, err := gzip.NewReader(file)
			utils.Logde(errors.Wrap(err, "failed to read downloaded gzip"))
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
				target := filepath.Join(params.GetBaseOpPath(), header.Name)
				// check the file type
				switch header.Typeflag {

				// if its a dir and it doesn't exist create it
				case tar.TypeDir:
					if _, err := os.Stat(target); err != nil {
						err := os.MkdirAll(target, 0o755)
						utils.Logde(errors.Wrap(err, "could not create directory from zip"))
					}

				// if it's a file create it
				case tar.TypeReg:
					f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
					utils.Logde(errors.Wrap(err, "could not open file target for unzipped file"))

					_, err = io.Copy(f, tr)
					utils.Logde(errors.Wrap(err, "could not write unzipped data to target file"))

					err = f.Sync()
					utils.Logde(errors.Wrap(err, "could not fsync unzipped target file"))
					f.Close()
				}
			}
			err = reader.Close()
			utils.Logde(errors.Wrap(err, "could not close gzip reader"))
			err = file.Close()
			utils.Logde(errors.Wrap(err, "could not close downloaded file"))

			err = os.Remove(outputName)
			utils.Logde(errors.Wrap(err, "could not delete downloaded gzip file"))

			progress.DownloadedFiles++
			progress.LocationDetails[locationName].DownloadedFiles++
		}
	}
	err = os.RemoveAll(filepath.Join(params.GetBaseOpPath(), "tmp"))
	utils.Logde(errors.Wrap(err, "could not remove temp download directory"))

	slog.Info("Finished Downloading Bounds", "min_lat", bounds.MinLat, "min_lon", bounds.MinLon, "max_lat", bounds.MaxLat, "max_lon", bounds.MaxLon)
	return nil
}

func countFilesForBounds(bounds Bounds) int {
	minLat, minLon, maxLat, maxLon := adjustedBounds(bounds)
	return ((maxLat - minLat) / GROUP_AREA_BOX_DEGREES) * ((maxLon - minLon) / GROUP_AREA_BOX_DEGREES)
}

func getDataForPath(path string) LocationData {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		panic("invalid download path")
	}
	box := BOUNDING_BOXES[parts[0]][parts[1]]
	if len(parts) > 2 {
		for i := range len(parts) - 2 {
			box = BOUNDING_BOXES[box.Submenu][parts[i+2]]
		}
	}
	return box
}

func getBoundsForPath(path string) Bounds {
	return getDataForPath(path).BoundingBox
}

func countTotalFiles(paths []string) int {
	totalFiles := 0

	for _, p := range paths {
		totalFiles += countFilesForBounds(getBoundsForPath(p))
	}

	return totalFiles
}
