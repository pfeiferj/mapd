package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

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

	return nil
}

type Bounds struct {
	MinLat float64 `json:"min_lat"`
	MinLon float64 `json:"min_lon"`
	MaxLat float64 `json:"max_lat"`
	MaxLon float64 `json:"max_lon"`
}

func DownloadIfTriggered() (err error) {
	b, err := GetParam(DOWNLOAD_BOUNDS)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return
	}

	var bounds Bounds
	err = json.Unmarshal(b, &bounds)
	if err != nil {
		return err
	}

	err = DownloadBounds(bounds)
	if err != nil {
		return err
	}
	err = PutParam(DOWNLOAD_BOUNDS, []byte{})
	loge(err)
	return nil
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
			cmd := exec.Command("tar", "-xf", outputName)
			cmd.Dir = filepath.Dir(GetBaseOpPath())
			err = cmd.Run()
			loge(err)
			os.Remove(outputName)
		}
	}
	err = os.RemoveAll(filepath.Join(GetBaseOpPath(), "tmp"))
	loge(err)

	fmt.Printf("Finished Downloading Bounds: %f, %f, %f, %f\n", bounds.MinLat, bounds.MinLon, bounds.MaxLat, bounds.MaxLon)
	return nil
}
