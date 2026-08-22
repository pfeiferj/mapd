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
	ArchiveRanges []ArchiveRange `json:"archive_ranges,omitempty"`
	BoundingBox   Bounds         `json:"bounding_box"`
	FullName      string         `json:"full_name"`
	Submenu       string         `json:"submenu"`
}

// ArchiveRange is [minimum latitude, inclusive minimum longitude, exclusive maximum longitude] for a 2-degree latitude band.
type ArchiveRange [3]int

func (r ArchiveRange) coordinates() (latitude, minLongitude, maxLongitude int) {
	return r[0], r[1], r[2]
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

func (p *DownloadProgress) addLocationDetails(path string, location LocationData) {
	p.LocationDetails[path] = &DownloadLocationDetail{
		TotalFiles: countFilesForLocation(location),
	}
}

func Download(paths string, progressChan chan DownloadProgress, cancelChan chan bool) {
	slog.Info("download", "paths", paths)
	pathsSplit := strings.Split(paths, ",")
	menu := GetDownloadMenu()
	d := download{
		progress: DownloadProgress{
			LocationsToDownload: pathsSplit,
			TotalFiles:          countTotalFiles(menu, pathsSplit),
			LocationDetails:     make(map[string]*DownloadLocationDetail),
			Active:              true,
		},
		progressChan: progressChan,
		cancelChan:   cancelChan,
	}

	for _, p := range pathsSplit {
		location := getDataForPath(menu, p)
		d.progress.addLocationDetails(p, location)
		slog.Info("downloading location", "location", location.FullName)
		err, canceled := d.downloadLocation(location, p)
		if err != nil {
			slog.Warn("failed to download location", "error", err, "location", location.FullName)
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

func archiveRangesForLocation(location LocationData) []ArchiveRange {
	if len(location.ArchiveRanges) > 0 {
		return location.ArchiveRanges
	}

	minLat, minLon, maxLat, maxLon := adjustedBounds(location.BoundingBox)
	var archiveRanges []ArchiveRange
	for lat := minLat; lat < maxLat; lat += GROUP_AREA_BOX_DEGREES {
		archiveRanges = append(archiveRanges, ArchiveRange{lat, minLon, maxLon})
	}
	return archiveRanges
}

func (d *download) downloadLocation(location LocationData, locationName string) (err error, cancel bool) {
	slog.Info("Downloading Location", "location", locationName)

	for _, archiveRange := range archiveRangesForLocation(location) {
		latitude, minLongitude, maxLongitude := archiveRange.coordinates()
		for longitude := minLongitude; longitude < maxLongitude; longitude += GROUP_AREA_BOX_DEGREES {
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

			filename := fmt.Sprintf("offline/%d/%d.tar.gz", latitude, longitude)
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

	slog.Info("Finished Downloading Location", "location", locationName)
	return nil, false
}

func countFilesForLocation(location LocationData) int {
	totalFiles := 0
	for _, archiveRange := range archiveRangesForLocation(location) {
		_, minLongitude, maxLongitude := archiveRange.coordinates()
		for longitude := minLongitude; longitude < maxLongitude; longitude += GROUP_AREA_BOX_DEGREES {
			totalFiles++
		}
	}
	return totalFiles
}

func getDataForPath(menu DownloadMenu, path string) LocationData {
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

func countTotalFiles(menu DownloadMenu, paths []string) int {
	totalFiles := 0

	for _, p := range paths {
		totalFiles += countFilesForLocation(getDataForPath(menu, p))
	}

	return totalFiles
}
