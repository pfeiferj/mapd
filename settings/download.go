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
	BoundingBox  Bounds       `json:"bounding_box"`
	FullName     string       `json:"full_name"`
	Submenu      string       `json:"submenu,omitempty"`
	DownloadRows DownloadRows `json:"download_rows,omitempty"`
}

type DownloadRows [][3]int

func (rows *DownloadRows) UnmarshalJSON(data []byte) error {
	// Pointers distinguish a missing/null coordinate from the valid coordinate zero.
	var decoded [][]*int
	*rows = nil
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil // Invalid optional selections retain the bounding-box fallback.
	}
	for _, row := range decoded {
		if len(row) != 3 || row[0] == nil || row[1] == nil || row[2] == nil {
			*rows = nil
			return nil
		}
		*rows = append(*rows, [3]int{*row[0], *row[1], *row[2]})
	}
	return nil
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

func Download(paths string, progressChan chan DownloadProgress, cancelChan chan bool) {
	slog.Info("download", "paths", paths)
	pathsSplit := strings.Split(paths, ",")
	menu := GetDownloadMenu()
	locations := make([]LocationData, len(pathsSplit))
	d := download{
		progress: DownloadProgress{
			LocationsToDownload: pathsSplit,
			LocationDetails:     make(map[string]*DownloadLocationDetail),
			Active:              true,
		},
		progressChan: progressChan,
		cancelChan:   cancelChan,
	}

	for i, path := range pathsSplit {
		locations[i] = menu.getDataForPath(path)
		total := locations[i].countFiles()
		d.progress.TotalFiles += total
		d.progress.LocationDetails[path] = &DownloadLocationDetail{TotalFiles: total}
	}

	for i, p := range pathsSplit {
		location := locations[i]
		d.progress.LocationDetails[p].DownloadedFiles = 0
		slog.Info("downloading nation", "nation", location.FullName)
		err, canceled := d.downloadLocation(location, p)
		if err != nil {
			slog.Warn("failed to download nation", "error", err, "nation", location.FullName)
		}
		if canceled {
			d.progress.Canceled = true
			break
		}
	}
	d.progress.Active = false
	d.publishProgress()
}

func (d *download) publishProgress() {
	progress := d.progress
	progress.LocationsToDownload = append([]string(nil), d.progress.LocationsToDownload...)
	progress.LocationDetails = make(map[string]*DownloadLocationDetail, len(d.progress.LocationDetails))
	for path, detail := range d.progress.LocationDetails {
		locationDetail := *detail
		progress.LocationDetails[path] = &locationDetail
	}

	select { // discard queued progress
	case <-d.progressChan:
	default:
	}
	select {
	case d.progressChan <- progress:
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

func (d *download) downloadLocation(location LocationData, locationName string) (err error, cancel bool) {
	bounds := location.BoundingBox
	slog.Info("Downloading Bounds", "min_lat", bounds.MinLat, "min_lon", bounds.MinLon, "max_lat", bounds.MaxLat, "max_lon", bounds.MaxLon)

	for _, row := range location.downloadRows() {
		i := row[0]
		for j := row[1]; j < row[2]; j += GROUP_AREA_BOX_DEGREES {
			d.publishProgress()
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

// Each row is [latitude, first longitude, exclusive last longitude] on the archive grid.
func (location LocationData) downloadRows() [][3]int {
	minLat, minLon, maxLat, maxLon := adjustedBounds(location.BoundingBox)
	valid := len(location.DownloadRows) > 0
	for index, row := range location.DownloadRows {
		if row[0] < minLat || row[0] >= maxLat || row[1] < minLon || row[2] > maxLon || row[1] >= row[2] ||
			row[0]%GROUP_AREA_BOX_DEGREES != 0 || row[1]%GROUP_AREA_BOX_DEGREES != 0 || row[2]%GROUP_AREA_BOX_DEGREES != 0 {
			valid = false
			break
		}
		if index > 0 {
			previous := location.DownloadRows[index-1]
			if row[0] < previous[0] || (row[0] == previous[0] && row[1] < previous[2]) {
				valid = false
				break
			}
		}
	}
	if valid {
		return location.DownloadRows
	}
	var rows [][3]int
	for latitude := minLat; latitude < maxLat; latitude += GROUP_AREA_BOX_DEGREES {
		rows = append(rows, [3]int{latitude, minLon, maxLon})
	}
	return rows
}

func (location LocationData) countFiles() int {
	total := 0
	for _, row := range location.downloadRows() {
		total += (row[2] - row[1]) / GROUP_AREA_BOX_DEGREES
	}
	return total
}

func (menu DownloadMenu) getDataForPath(path string) LocationData {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		slog.Warn("ignoring invalid download path", "path", path)
		return LocationData{}
	}
	box := menu[parts[0]][parts[1]]
	if len(parts) > 2 {
		for i := range len(parts) - 2 {
			box = menu[box.Submenu][parts[i+2]]
		}
	}
	return box
}
