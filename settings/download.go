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

type archiveFile interface {
	io.Writer
	Sync() error
	Close() error
}

type archiveInstallOps struct {
	mkdirTemp func(string, string) (string, error)
	mkdirAll  func(string, os.FileMode) error
	openFile  func(string, int, os.FileMode) (archiveFile, error)
	rename    func(string, string) error
	removeAll func(string) error
	stat      func(string) (os.FileInfo, error)
}

var defaultArchiveInstallOps = archiveInstallOps{
	mkdirTemp: os.MkdirTemp,
	mkdirAll:  os.MkdirAll,
	openFile: func(name string, flag int, perm os.FileMode) (archiveFile, error) {
		return os.OpenFile(name, flag, perm)
	},
	rename:    os.Rename,
	removeAll: os.RemoveAll,
	stat:      os.Stat,
}

func cleanArchivePath(name string) (string, error) {
	clean := filepath.Clean(name)
	if name == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.Errorf("invalid archive path: %q", name)
	}
	return clean, nil
}

func writeArchiveFile(file archiveFile, reader io.Reader, expectedSize int64) error {
	written, err := io.Copy(file, reader)
	if err != nil {
		file.Close()
		return errors.Wrap(err, "could not write staged archive file")
	}
	if written != expectedSize {
		file.Close()
		return errors.Errorf("staged archive file size mismatch: wrote %d bytes, expected %d", written, expectedSize)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return errors.Wrap(err, "could not fsync staged archive file")
	}
	if err := file.Close(); err != nil {
		return errors.Wrap(err, "could not close staged archive file")
	}
	return nil
}

func extractArchiveToStage(archivePath string, stageRoot string, expectedRoot string, ops archiveInstallOps) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return errors.Wrap(err, "could not open downloaded file")
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return errors.Wrap(err, "could not parse gzip downloaded file")
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	seen := make(map[string]struct{})
	regularFiles := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "could not read downloaded tar archive")
		}

		// if the header is nil, just skip it (not sure how this happens)
		if header == nil {
			continue
		}

		name, err := cleanArchivePath(header.Name)
		if err != nil {
			return err
		}
		if name != expectedRoot && !strings.HasPrefix(name, expectedRoot+string(os.PathSeparator)) {
			return errors.Errorf("archive entry %q is outside expected root %q", header.Name, expectedRoot)
		}
		if _, exists := seen[name]; exists {
			return errors.Errorf("duplicate archive entry: %q", header.Name)
		}
		seen[name] = struct{}{}

		// the target location where the dir/file should be created
		target := filepath.Join(stageRoot, name)
		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := ops.stat(target); err != nil {
				if !os.IsNotExist(err) {
					return errors.Wrap(err, "could not inspect staged archive directory")
				}
				if err := ops.mkdirAll(target, os.FileMode(header.Mode).Perm()); err != nil {
					return errors.Wrap(err, "could not create staged archive directory")
				}
			}

		// if it's a file create it
		case tar.TypeReg, tar.TypeRegA:
			if err := ops.mkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.Wrap(err, "could not create staged archive parent directory")
			}
			stagedFile, err := ops.openFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode).Perm())
			if err != nil {
				return errors.Wrap(err, "could not open staged archive file")
			}
			if err := writeArchiveFile(stagedFile, tr, header.Size); err != nil {
				return err
			}
			regularFiles++
		default:
			return errors.Errorf("unsupported archive entry type %d for %q", header.Typeflag, header.Name)
		}
	}

	if _, err := io.Copy(io.Discard, reader); err != nil {
		return errors.Wrap(err, "could not validate gzip trailer")
	}
	if regularFiles == 0 {
		return errors.New("downloaded archive contains no regular files")
	}
	return nil
}

func commitArchiveGroup(stageRoot string, basePath string, expectedRoot string, ops archiveInstallOps) (bool, error) {
	stagedGroup := filepath.Join(stageRoot, expectedRoot)
	stagedInfo, err := ops.stat(stagedGroup)
	if err != nil {
		return false, errors.Wrap(err, "could not inspect staged archive group")
	}
	if !stagedInfo.IsDir() {
		return false, errors.Errorf("staged archive root is not a directory: %q", expectedRoot)
	}

	liveGroup := filepath.Join(basePath, expectedRoot)
	if err := ops.mkdirAll(filepath.Dir(liveGroup), 0o755); err != nil {
		return false, errors.Wrap(err, "could not create live archive parent directory")
	}

	backupGroup := filepath.Join(stageRoot, "backup")
	hadLiveGroup := false
	if _, err := ops.stat(liveGroup); err == nil {
		if err := ops.rename(liveGroup, backupGroup); err != nil {
			return false, errors.Wrap(err, "could not move live archive group to backup")
		}
		hadLiveGroup = true
	} else if !os.IsNotExist(err) {
		return false, errors.Wrap(err, "could not inspect live archive group")
	}

	if err := ops.rename(stagedGroup, liveGroup); err != nil {
		if hadLiveGroup {
			if restoreErr := ops.rename(backupGroup, liveGroup); restoreErr != nil {
				return true, errors.Wrapf(err, "could not install staged archive group and could not restore backup at %q: %v", backupGroup, restoreErr)
			}
		}
		return false, errors.Wrap(err, "could not install staged archive group")
	}
	return false, nil
}

func installArchiveWithOps(archivePath string, basePath string, expectedRoot string, ops archiveInstallOps) error {
	cleanExpectedRoot, err := cleanArchivePath(expectedRoot)
	if err != nil {
		return err
	}
	if cleanExpectedRoot != expectedRoot {
		return errors.Errorf("archive root is not canonical: %q", expectedRoot)
	}

	stageRoot, err := ops.mkdirTemp(basePath, ".mapd-install-")
	if err != nil {
		return errors.Wrap(err, "could not create archive staging directory")
	}
	preserveStage := false
	defer func() {
		if preserveStage {
			return
		}
		if err := ops.removeAll(stageRoot); err != nil {
			slog.Warn("could not remove archive staging directory", "error", err, "directory", stageRoot)
		}
	}()

	if err := extractArchiveToStage(archivePath, stageRoot, expectedRoot, ops); err != nil {
		return err
	}
	preserveStage, err = commitArchiveGroup(stageRoot, basePath, expectedRoot, ops)
	return err
}

func installArchive(archivePath string, basePath string, expectedRoot string) error {
	return installArchiveWithOps(archivePath, basePath, expectedRoot, defaultArchiveInstallOps)
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
			installErr := installArchive(outputName, params.GetBaseOpPath(), strings.TrimSuffix(filename, ".tar.gz"))
			if installErr != nil {
				slog.Warn("failed to install downloaded archive", "error", installErr, "file", outputName)
			}
			if err := os.Remove(outputName); err != nil {
				slog.Warn("could not delete downloaded gzip file", "error", err)
			}
			if installErr != nil {
				continue
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
