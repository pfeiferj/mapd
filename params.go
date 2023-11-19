package main

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/gofrs/flock"
)

var (
	ParamsPath    string = "/data/params/d"
	MemParamsPath string = "/dev/shm/params/d"
	BasePath      string = GetBasePath()
)

// Params
var (
	ROAD_NAME                 = ParamPath("RoadName", true)
	MAP_HAZARD                = ParamPath("MapHazard", true)
	NEXT_MAP_HAZARD           = ParamPath("NextMapHazard", true)
	MAP_SPEED_LIMIT           = ParamPath("MapSpeedLimit", true)
	MAP_ADVISORY_LIMIT        = ParamPath("MapAdvisoryLimit", true)
	NEXT_MAP_ADVISORY_LIMIT   = ParamPath("NextMapAdvisoryLimit", true)
	NEXT_MAP_SPEED_LIMIT      = ParamPath("NextMapSpeedLimit", true)
	LAST_GPS_POSITION         = ParamPath("LastGPSPosition", true)
	LAST_GPS_POSITION_PERSIST = ParamPath("LastGPSPosition", false)
	DOWNLOAD_BOUNDS           = ParamPath("OSMDownloadBounds", true)
	DOWNLOAD_LOCATIONS        = ParamPath("OSMDownloadLocations", true)
	DOWNLOAD_PROGRESS         = ParamPath("OSMDownloadProgress", false)
	MAP_CURVATURES            = ParamPath("MapCurvatures", false)
	MAP_TARGET_VELOCITIES     = ParamPath("MapTargetVelocities", false)
)

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
	exists, err := Exists("/data/media/0")
	loge(err)
	if exists {
		return "/data/media/0/osm"
	} else {
		return "media/"
	}
}

func EnsureParamDirectories() {
	err := os.MkdirAll(ParamsPath, 0o775)
	loge(err)
	err = os.MkdirAll(MemParamsPath, 0o775)
	loge(err)
}

func EnsureParamsExist() {
	data := []uint8{}
	_ = PutParam(ROAD_NAME, data)
	_ = PutParam(MAP_HAZARD, data)
	_ = PutParam(NEXT_MAP_HAZARD, data)
	_ = PutParam(MAP_SPEED_LIMIT, data)
	_ = PutParam(MAP_ADVISORY_LIMIT, data)
	_ = PutParam(NEXT_MAP_ADVISORY_LIMIT, data)
	_ = PutParam(NEXT_MAP_SPEED_LIMIT, data)
	_ = PutParam(LAST_GPS_POSITION, data)
	_ = PutParam(DOWNLOAD_BOUNDS, data)
	_ = PutParam(DOWNLOAD_LOCATIONS, data)
	_ = PutParam(DOWNLOAD_PROGRESS, data)
	_ = PutParam(MAP_CURVATURES, data)
	_ = PutParam(MAP_TARGET_VELOCITIES, data)
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
	lock_dir := filepath.Dir(dir)
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

	fileLock := flock.New(filepath.Join(lock_dir, ".lock"))

	err = fileLock.Lock()
	if err != nil {
		return err
	}
	defer loge(fileLock.Unlock())

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
	lock_dir := filepath.Dir(dir)
	fileLock := flock.New(filepath.Join(lock_dir, ".lock"))

	err := fileLock.Lock()
	if err != nil {
		return err
	}
	defer loge(fileLock.Unlock())

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
