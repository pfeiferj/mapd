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
)

type LocationData struct {
	BoundingBox Bounds `json:"bounding_box"`
	FullName    string `json:"full_name"`
}

//go:embed nation_bounding_boxes.json
var nationBoundingBoxesJson []byte
var NATION_BOXES = map[string]LocationData{}
var _ = json.Unmarshal(nationBoundingBoxesJson, &NATION_BOXES)

//go:embed us_states_bounding_boxes.json
var statesBoundingBoxesJson []byte
var STATE_BOXES = map[string]LocationData{}
var _ = json.Unmarshal(statesBoundingBoxesJson, &STATE_BOXES)

func DownloadFile(url string, filepath string) (err error) {
	fmt.Printf("Downloading: %s\n", url)
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}
	err = out.Sync()
	if err != nil {
		return err
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
	TotalFiles      int     `json:"total_files"`
	DownloadedFiles int     `json:"downloaded_files"`
	Percentage      float64 `json:"percentage"`
}

func DownloadIfTriggered() {
	b, err := GetParam(DOWNLOAD_LOCATIONS)
	loge(err)
	if err == nil && len(b) != 0 {
		var locations DownloadLocations
		err = json.Unmarshal(b, &locations)
		loge(err)

		if err == nil {
			for _, location := range locations.Nations {
				fmt.Println(location)
				lData, ok := NATION_BOXES[location]
				fmt.Println(ok)
				if ok {
					err = DownloadBounds(lData.BoundingBox)
					if err != nil {
						loge(err)
					}
				}
			}
			for _, location := range locations.States {
				lData, ok := STATE_BOXES[location]
				if ok {
					err = DownloadBounds(lData.BoundingBox)
					if err != nil {
						loge(err)
					}
				}
			}
		}
	}
	err = PutParam(DOWNLOAD_LOCATIONS, []byte{})
	loge(err)

	b, err = GetParam(DOWNLOAD_BOUNDS)
	loge(err)
	if err == nil && len(b) != 0 {
		var bounds Bounds
		err = json.Unmarshal(b, &bounds)
		loge(err)

		if err == nil {
			err = DownloadBounds(bounds)
			loge(err)
			if err == nil {
				err = PutParam(DOWNLOAD_BOUNDS, []byte{})
				loge(err)
			}
		}
	}
}

func DownloadBounds(bounds Bounds) (err error) {
	fmt.Printf("Downloading Bounds: %f, %f, %f, %f\n", bounds.MinLat, bounds.MinLon, bounds.MaxLat, bounds.MaxLon)

	// clip given bounds to file areas
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
	totalFiles := ((maxLat - minLat) / GROUP_AREA_BOX_DEGREES) * ((maxLon - minLon) / GROUP_AREA_BOX_DEGREES)
	progress := &DownloadProgress{
		TotalFiles: totalFiles,
	}

	for i := minLat; i < maxLat; i += GROUP_AREA_BOX_DEGREES {
		for j := minLon; j < maxLon; j += GROUP_AREA_BOX_DEGREES {
			filename := fmt.Sprintf("offline/%d/%d.tar.gz", i, j)
			url := fmt.Sprintf("https://map-data.pfeifer.dev/%s", filename)
			outputName := filepath.Join(GetBaseOpPath(), "tmp", filename)
			err := os.MkdirAll(filepath.Dir(outputName), 0775)
			loge(err)
			err = DownloadFile(url, outputName)
			if err != nil {
				fmt.Print(err)
				continue
			}
			file, err := os.Open(outputName)
			loge(err)
			reader, err := gzip.NewReader(file)
			loge(err)
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
						err := os.MkdirAll(target, 0755)
						loge(err)
					}

				// if it's a file create it
				case tar.TypeReg:
					f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
					loge(err)

					_, err = io.Copy(f, tr)
					loge(err)

					err = f.Sync()
					loge(err)
					f.Close()
				}
			}
			err = reader.Close()
			loge(err)
			err = file.Close()
			loge(err)

			err = os.Remove(outputName)
			loge(err)

			progress.DownloadedFiles++
			progress.Percentage = (float64(progress.DownloadedFiles) / float64(progress.TotalFiles)) * 100

			progressData, err := json.Marshal(progress)
			if err != nil {
				loge(err)
			}

			progressFile := ParamPath("DownloadProgress", true)
			err = PutParam(progressFile, progressData)
			if err != nil {
				loge(err)
			}
		}
	}
	err = os.RemoveAll(filepath.Join(GetBaseOpPath(), "tmp"))
	loge(err)

	fmt.Printf("Finished Downloading Bounds: %f, %f, %f, %f\n", bounds.MinLat, bounds.MinLon, bounds.MaxLat, bounds.MaxLon)
	return nil
}
