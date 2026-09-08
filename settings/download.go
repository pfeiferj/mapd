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

// DownloadFile fetches url into destPath.
//
// It writes to destPath+".part" and only renames into place once the transfer has been verified,
// so an interrupted download (process killed, power cut, connection dropped) can never leave a
// TRUNCATED file sitting at the real path looking like a complete one. That failure mode is not
// hypothetical: a device that lost power mid-download was later unable to read its own map tiles
// ("could not unmarshal offline data: unexpected EOF") and silently re-downloaded ~286MB over
// cellular to repair itself.
//
// The transfer is also checked against Content-Length when the server provides one. This is
// defence-in-depth rather than the primary guard: Go's HTTP client already surfaces a body that
// ends short of a declared length as io.ErrUnexpectedEOF, so removing this check does not by itself
// let a truncated download through (verified by mutation). It costs nothing and covers the case
// where a future transport or a proxy is less strict.
func DownloadFile(url string, destPath string) (err error) {
	slog.Info("Downloading", "url", url)

	partPath := destPath + ".part"
	// Remove any leftover .part from a previous interrupted attempt before starting.
	_ = os.Remove(partPath)

	out, err := os.Create(partPath)
	if err != nil {
		return errors.Wrap(err, "could not create file for download")
	}
	// On any failure below, drop the partial file rather than leaving it to be mistaken for good
	// data later. Named return + defer so every early return is covered.
	defer func() {
		out.Close()
		if err != nil {
			_ = os.Remove(partPath)
		}
	}()

	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrap(err, "could not download the file data")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("download received bad status: %s", resp.Status)
	}

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return errors.Wrap(err, "could not write download data to file")
	}
	// ContentLength is -1 when the server doesn't declare one (chunked); only assert when known.
	if resp.ContentLength >= 0 && written != resp.ContentLength {
		return errors.Errorf("truncated download: got %d bytes, expected %d", written, resp.ContentLength)
	}
	if err = out.Sync(); err != nil {
		return errors.Wrap(err, "could not fsync downloaded file")
	}
	if err = out.Close(); err != nil {
		return errors.Wrap(err, "could not close downloaded file")
	}

	// Atomic publish: either the complete file appears at destPath, or nothing does.
	if err = os.Rename(partPath, destPath); err != nil {
		return errors.Wrap(err, "could not move downloaded file into place")
	}
	return nil
}

// ValidateArchive reads the whole gzip+tar stream and discards it, purely to prove the archive is
// intact BEFORE any live map tile is touched.
//
// This is the load-bearing check. gzip carries a CRC32 and length trailer that is only verified once
// the stream is read to the end, so a truncated archive is indistinguishable from a good one until
// something reads all of it. Extracting first and discovering the problem afterwards is exactly how
// half-written tiles reach disk.
func ValidateArchive(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "could not open archive for validation")
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return errors.Wrap(err, "could not parse archive gzip header")
	}
	defer reader.Close()

	tr := tar.NewReader(reader)
	entries := 0
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break // clean end of archive
		}
		if err != nil {
			return errors.Wrap(err, "archive is corrupt or truncated")
		}
		if header == nil {
			continue
		}
		// Read the entry body so gzip's CRC/length trailer is actually checked.
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return errors.Wrap(err, "archive entry is corrupt or truncated")
		}
		entries++
	}
	if entries == 0 {
		return errors.Errorf("archive contains no files")
	}

	// Drain whatever is left of the GZIP stream. This is not redundant: tar.Next returns io.EOF at
	// the tar end-of-archive marker, which sits BEFORE gzip's 8-byte CRC32+ISIZE trailer, so
	// stopping at the tar level never checks the trailer at all. A small tail truncation -- the
	// last few bytes lost as a connection dies -- otherwise validates clean. Reading to the end of
	// the gzip stream is what actually verifies the checksum and the uncompressed length.
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return errors.Wrap(err, "archive gzip stream is corrupt or truncated")
	}
	return nil
}

// safeJoin resolves a tar entry name under base, refusing anything that would escape it.
// Standard tar-slip guard: an entry named "../../etc/whatever" must never be written.
func safeJoin(base string, name string) (string, error) {
	target := filepath.Join(base, name)
	cleanBase := filepath.Clean(base) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanBase) {
		return "", errors.Errorf("archive entry escapes destination: %s", name)
	}
	return target, nil
}

// writeFileAtomic writes r to target via target+".part" then renames, so a crash mid-extract leaves
// a stray .part rather than a half-written map tile at a live path.
//
// It also fixes a subtler bug in the original extractor, which opened targets with
// os.O_CREATE|os.O_RDWR and NO os.O_TRUNC: writing a SHORTER file over a longer existing one left
// the tail of the old file attached, producing a corrupt hybrid that survived re-downloading.
// The RENAME is what fixes that -- it replaces the file wholesale, so length cannot carry over.
// O_TRUNC below is belt-and-braces for the .part itself.
func writeFileAtomic(target string, mode os.FileMode, r io.Reader) (err error) {
	partPath := target + ".part"
	_ = os.Remove(partPath)
	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return errors.Wrap(err, "could not open extract target")
	}
	defer func() {
		f.Close()
		if err != nil {
			_ = os.Remove(partPath)
		}
	}()
	if _, err = io.Copy(f, r); err != nil {
		return errors.Wrap(err, "could not write extract target")
	}
	if err = f.Sync(); err != nil {
		return errors.Wrap(err, "could not fsync extract target")
	}
	if err = f.Close(); err != nil {
		return errors.Wrap(err, "could not close extract target")
	}
	return errors.Wrap(os.Rename(partPath, target), "could not move extracted file into place")
}

// extractArchive unpacks a VALIDATED archive into the base path.
//
// Unlike the original inline extractor this returns an error instead of logging and pressing on:
// every one of those old warn-and-continue paths (open failed, gzip parse failed, copy failed) left
// a partially-written file at a LIVE tile path and still counted the tile as successfully
// downloaded. Each regular file is written atomically, so an interrupted extract leaves a stray
// .part rather than a corrupt tile.
func extractArchive(archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return errors.Wrap(err, "could not open archive")
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		return errors.Wrap(err, "could not parse archive gzip")
	}
	defer reader.Close()

	base := params.GetBaseOpPath()
	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "could not read archive entry")
		}
		if header == nil {
			continue
		}
		target, err := safeJoin(base, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if _, statErr := os.Stat(target); statErr != nil {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return errors.Wrap(err, "could not create directory from archive")
				}
			}
		case tar.TypeReg:
			// The parent dir may not exist if the archive omits explicit dir entries.
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.Wrap(err, "could not create parent directory for archive entry")
			}
			if err := writeFileAtomic(target, os.FileMode(header.Mode), tr); err != nil {
				return err
			}
		}
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

func (d *download) downloadBounds(bounds Bounds, locationName string) (err error, cancel bool) {
	slog.Info("Downloading Bounds", "min_lat", bounds.MinLat, "min_lon", bounds.MinLon, "max_lat", bounds.MaxLat, "max_lon", bounds.MaxLon)

	// clip given bounds to file areas
	minLat, minLon, maxLat, maxLon := adjustedBounds(bounds)
	d.progress.LocationDetails[locationName].TotalFiles = countFilesForBounds(bounds)
	for i := minLat; i < maxLat; i += GROUP_AREA_BOX_DEGREES {
		for j := minLon; j < maxLon; j += GROUP_AREA_BOX_DEGREES {
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

			// VALIDATE BEFORE EXTRACTING. The archive is read end-to-end (which is what actually
			// verifies gzip's CRC/length trailer) before a single live tile is touched. A truncated
			// or corrupt download is discarded here instead of being half-written over good map
			// data -- the failure that left this device unable to read its own tiles.
			if err = ValidateArchive(outputName); err != nil {
				slog.Warn("downloaded archive failed validation, discarding", "error", err, "url", url, "file", outputName)
				if rmErr := os.Remove(outputName); rmErr != nil {
					slog.Warn("could not delete invalid archive", "error", rmErr, "file", outputName)
				}
				continue
			}

			if err = extractArchive(outputName); err != nil {
				// Do NOT count this tile as downloaded -- it isn't.
				slog.Warn("failed to extract archive", "error", err, "file", outputName)
				if rmErr := os.Remove(outputName); rmErr != nil {
					slog.Warn("could not delete archive", "error", rmErr, "file", outputName)
				}
				continue
			}

			if err = os.Remove(outputName); err != nil {
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
		slog.Warn("ignoring invalid download path", "path", path)
		return LocationData{}
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
