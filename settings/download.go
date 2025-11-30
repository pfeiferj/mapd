package settings

import (
	"archive/tar"
	"compress/gzip"
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
)

type LocationData struct {
	BoundingBox Bounds `json:"bounding_box"`
	FullName    string `json:"full_name"`
	Submenu     string `json:"submenu"`
}

type DownloadMenu map[string]map[string]LocationData

func GetDownloadMenu() (menu DownloadMenu) {
	if _, err := os.Stat("/data/openpilot/mapd_download_menu.json"); err == nil {
		recommended, err := os.ReadFile("/data/openpilot/mapd_download_menu.json")
		if err != nil {
			slog.Warn("failed to read custom download menu", "error", err)
		}
		err = json.Unmarshal(recommended, &menu)
		if err != nil {
			slog.Warn("failed to load custom download menu", "error", err)
			return
		}
	} else {
		err := json.Unmarshal(boundingBoxesJson, &menu)
		if err != nil {
			slog.Warn("failed to load download menu", "error", err)
			return
		}
	}
	return
}

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

type DownloadProgress struct {
	TotalFiles          int                                `json:"total_files"`
	DownloadedFiles     int                                `json:"downloaded_files"`
	Canceled            bool                               `json:"canceled"`
	Active              bool                               `json:"active"`
	LocationsToDownload []string                           `json:"locations_to_download"`
	LocationDetails     map[string]*DownloadLocationDetail `json:"location_details"`
}

type DownloadLocationDetail struct {
	TotalFiles      int `json:"location_total_files"`
	DownloadedFiles int `json:"location_downloaded_files"`
}

type download struct {
	progress     DownloadProgress
	progressChan chan DownloadProgress
	cancelChan   chan bool
}

func (p *DownloadProgress) addLocationDetails(path string) {
	p.LocationDetails[path] = &DownloadLocationDetail{
		TotalFiles: countFilesForBounds(getBoundsForPath(path)),
	}
}

func Download(paths string, progressChan chan DownloadProgress, cancelChan chan bool) {
	slog.Info("download", "paths", paths)
	pathsSplit := strings.Split(paths, ",")
	d := download{
		progress: DownloadProgress{
			LocationsToDownload: pathsSplit,
			TotalFiles:          countTotalFiles(pathsSplit),
			LocationDetails:     make(map[string]*DownloadLocationDetail),
			Active:              true,
		},
		progressChan: progressChan,
		cancelChan:   cancelChan,
	}

	for _, p := range pathsSplit {
		d.progress.addLocationDetails(p)
		location := getDataForPath(p)
		slog.Info("downloading nation", "nation", location.FullName)
		err, canceled := d.downloadBounds(location.BoundingBox, p)
		if err != nil {
			slog.Warn("failed to download nation", "error", err, "nation", location.FullName)
		}
		if canceled {
			d.progress.Canceled = true
			break
		}
	}
	d.progress.Active = false
	select { // nonblocking update of progress
	case d.progressChan <- d.progress:
	default:
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

func (d *download) downloadBounds(bounds Bounds, locationName string) (err error, cancel bool) {
	slog.Info("Downloading Bounds", "min_lat", bounds.MinLat, "min_lon", bounds.MinLon, "max_lat", bounds.MaxLat, "max_lon", bounds.MaxLon)

	// clip given bounds to file areas
	minLat, minLon, maxLat, maxLon := adjustedBounds(bounds)
	d.progress.LocationDetails[locationName].TotalFiles = countFilesForBounds(bounds)
	for i := minLat; i < maxLat; i += GROUP_AREA_BOX_DEGREES {
		for j := minLon; j < maxLon; j += GROUP_AREA_BOX_DEGREES {
			select { // nonblocking update of progress
			case d.progressChan <- d.progress:
			default:
			}
			select { // cancel if sent message
			case cancel := <-d.cancelChan:
				if cancel {
					return nil, true
				}
			default:
			}

			filename := fmt.Sprintf("offline/%d/%d.tar.gz", i, j)
			url := fmt.Sprintf("https://map-data.pfeifer.dev/%s", filename)
			outputName := filepath.Join(params.GetBaseOpPath(), "tmp", filename)
			err := os.MkdirAll(filepath.Dir(outputName), 0o775)
			if err != nil {
				slog.Error("failed to create offline maps output directory", "error", err)
			}
			err = DownloadFile(url, outputName)
			if err != nil {
				slog.Warn("failed to download file, continuing to next", "error", err, "url", url, "file", outputName)
				continue
			}
			file, err := os.Open(outputName)
			if err != nil {
				slog.Warn("failed to open downloaded file", "error", err, "file", outputName)
			}
			reader, err := gzip.NewReader(file)
			if err != nil {
				slog.Warn("failed to parse gzip downloaded file", "error", err, "file", outputName)
			}
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
						if err != nil {
							slog.Warn("could not create directory from downloaded gzip", "error", err, "file", outputName, "directory", target)
						}
					}

				// if it's a file create it
				case tar.TypeReg:
					f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
					if err != nil {
						slog.Warn("could not open file target from downloaded gzip", "error", err, "file", outputName, "targetFile", target)
					}

					_, err = io.Copy(f, tr)
					if err != nil {
						slog.Warn("could not write data to file target from downloaded gzip", "error", err, "file", outputName, "targetFile", target)
					}

					err = f.Sync()
					if err != nil {
						slog.Warn("could not fsync file target from downloaded gzip", "error", err, "file", outputName, "targetFile", target)
					}
					f.Close()
				}
			}
			err = reader.Close()
			if err != nil {
				slog.Warn("could not close gzip reader", "error", err)
			}
			err = file.Close()
			if err != nil {
				slog.Warn("could not close downloaded file", "error", err)
			}

			err = os.Remove(outputName)
			if err != nil {
				slog.Warn("could not delete downloaded gzip file", "error", err)
			}

			d.progress.DownloadedFiles++
			d.progress.LocationDetails[locationName].DownloadedFiles++
		}
	}
	err = os.RemoveAll(filepath.Join(params.GetBaseOpPath(), "tmp"))
	if err != nil {
		slog.Warn("could not remove temporary download directory", "error", err)
	}

	slog.Info("Finished Downloading Bounds", "min_lat", bounds.MinLat, "min_lon", bounds.MinLon, "max_lat", bounds.MaxLat, "max_lon", bounds.MaxLon)
	return nil, false
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
	menu := GetDownloadMenu()
	box := menu[parts[0]][parts[1]]
	if len(parts) > 2 {
		for i := range len(parts) - 2 {
			box = menu[box.Submenu][parts[i+2]]
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
