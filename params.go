package main

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/gofrs/flock"
)

var ParamsPath string = "/data/params/d"
var MemParamsPath string = "/dev/shm/params/d"
var BasePath string = GetBasePath()

// Params
var ROAD_NAME = ParamPath("RoadName", true)
var MAP_SPEED_LIMIT = ParamPath("MapSpeedLimit", true)
var NEXT_MAP_SPEED_LIMIT = ParamPath("NextMapSpeedLimit", true)
var LAST_GPS_POSITION = ParamPath("LastGPSPosition", true)
var LAST_GPS_POSITION_PERSIST = ParamPath("LastGPSPosition", false)
var DOWNLOAD_BOUNDS = ParamPath("OSMDownloadBounds", true)
var DOWNLOAD_LOCATIONS = ParamPath("OSMDownloadLocations", true)

// exists returns whether the given file or directory exists
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func GetBasePath() string {
	exists, err := Exists("/data/media/0/realdata")
	loge(err)
	if exists {
		return "/data/media/0/realdata/"
	} else {
		return "media/"
	}
}

func EnsureParamDirectories() {
	err := os.MkdirAll(ParamsPath, 0775)
	loge(err)
	err = os.MkdirAll(MemParamsPath, 0775)
	loge(err)
}

func IsString(data []byte) bool {
	for _, b := range data {
		if (b < 32 || b > 126) && !(b == 9 || b == 13 || b == 10) {
			return false
		}
	}
	return true
}

func GetParams(isMem bool) ([]string, error) {
	basePath := ParamsPath
	if isMem {
		basePath = MemParamsPath
	}

	files, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}

	paramFiles := []string{}
	for _, file := range files {
		name := file.Name()
		if file.Type().IsRegular() && name[0] != '.' {
			paramFiles = append(paramFiles, name)
		}
	}
	sort.Strings(paramFiles)

	return paramFiles, nil
}

func HasMemParams() (bool, error) {
	files, err := os.ReadDir(BasePath)
	if err != nil {
		return false, err
	}
	return len(files) > 1, nil
}

func ParamPath(name string, isMem bool) string {
	var basePath string
	if isMem {
		basePath = MemParamsPath
	} else {
		basePath = ParamsPath
	}
	return filepath.Join(basePath, name)
}

func GetParam(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func PutParam(path string, data []byte) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".tmp_value_"+filepath.Base(path))
	if err != nil {
		return err
	}
	tmpName := file.Name()
	defer os.Remove(tmpName)

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	err = file.Sync()
	if err != nil {
		return err
	}

	fileLock := flock.New(filepath.Join(dir, ".lock"))

	err = fileLock.Lock()
	if err != nil {
		return err
	}
	defer fileLock.Unlock()

	err = os.Rename(tmpName, path)
	if err != nil {
		return err
	}

	directory, err := os.Open(dir)
	if err != nil {
		return err
	}

	err = directory.Sync()
	if err != nil {
		return err
	}

	return nil
}

func RemoveParam(path string) error {
	dir := filepath.Dir(path)
	fileLock := flock.New(filepath.Join(dir, ".lock"))

	err := fileLock.Lock()
	if err != nil {
		return err
	}
	defer fileLock.Unlock()

	os.Remove(path)

	directory, err := os.Open(dir)
	if err != nil {
		return err
	}

	err = directory.Sync()
	if err != nil {
		return err
	}

	return nil
}
