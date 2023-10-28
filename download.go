package main

import (
	"bufio"
	"fmt"
	"io"
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

func TriggeredDownloadRegion() (err error) {
	b, err := GetParam(DOWNLOAD_REGION)
	if err != nil {
		return err
	}
	region := string(b)
	if len(region) > 0 {
		err = DownloadRegion(region)
		if err != nil {
			return err
		}
		err = PutParam(DOWNLOAD_REGION, []byte{})
		loge(err)
	}
	return nil
}

func DownloadRegion(region string) (err error) {
	fmt.Printf("Downloading Region: %s\n", region)
	file, err := os.Open(fmt.Sprintf("%s/%s-files.txt", GetBaseOpPath(), region))
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		filename := scanner.Text()
		url := fmt.Sprintf("https://map-data.pfeifer.dev/%s", filename)
		outputName := filepath.Join(GetBaseOpPath(), filename)
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
	err = os.RemoveAll(region)
	loge(err)

	if err := scanner.Err(); err != nil {
		return err
	}

	fmt.Printf("Finished Downloading Region: %s\n", region)
	return nil
}
