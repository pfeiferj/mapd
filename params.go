package main

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/pkg/errors"

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
	MAP_CURVATURES            = ParamPath("MapCurvatures", true)
	MAP_TARGET_VELOCITIES     = ParamPath("MapTargetVelocities", true)
	MAPD_LOG_LEVEL            = ParamPath("MapdLogLevel", true)
	MAPD_LOG_LEVEL_PERSIST    = ParamPath("MapdLogLevel", false)
	MAPD_PRETTY_LOG           = ParamPath("MapdPrettyLog", true)
	MAPD_PRETTY_LOG_PERSIST   = ParamPath("MapdPrettyLog", false)
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
	return false, errors.Wrap(err, "could not check param file stats")
}

func GetBasePath() string {
	exists, err := Exists("/data/media/0")
	logde(errors.Wrap(err, "could not check if media directory exists"))
	if exists {
		return "/data/media/0/osm"
	} else {
		return "media/"
	}
}

func EnsureParamDirectories() {
	err := os.MkdirAll(ParamsPath, 0o775)
	logde(errors.Wrap(err, "could not make params directory"))
	err = os.MkdirAll(MemParamsPath, 0o775)
	logde(errors.Wrap(err, "could not make memory params directory"))
}

func ResetParams() {
	empty_data := []uint8{}
	empty_object := []uint8{'{', '}'}
	empty_array := []uint8{'[', ']'}
	zero := []uint8{'0'}
	_ = PutParam(ROAD_NAME, empty_data)
	_ = PutParam(MAP_HAZARD, empty_object)
	_ = PutParam(NEXT_MAP_HAZARD, empty_object)
	_ = PutParam(MAP_SPEED_LIMIT, zero)
	_ = PutParam(MAP_ADVISORY_LIMIT, empty_object)
	_ = PutParam(NEXT_MAP_ADVISORY_LIMIT, empty_object)
	_ = PutParam(NEXT_MAP_SPEED_LIMIT, empty_object)
	_ = PutParam(LAST_GPS_POSITION, empty_object)
	_ = PutParam(DOWNLOAD_BOUNDS, empty_data)
	_ = PutParam(DOWNLOAD_LOCATIONS, empty_data)
	_ = PutParam(DOWNLOAD_PROGRESS, empty_data)
	_ = PutParam(MAP_CURVATURES, empty_array)
	_ = PutParam(MAP_TARGET_VELOCITIES, empty_array)
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
		return nil, errors.Wrap(err, "could not read params directory")
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
		return false, errors.Wrap(err, "could not read mem params directory")
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
		return errors.Wrap(err, "could not create temp param file")
	}
	tmpName := file.Name()
	defer os.Remove(tmpName)

	_, err = file.Write(data)
	if err != nil {
		return errors.Wrap(err, "could not write data to temp param file")
	}

	err = file.Sync()
	if err != nil {
		return errors.Wrap(err, "could not fsync temp param file")
	}

	fileLock := flock.New(filepath.Join(lock_dir, ".lock"))

	retries := 0
	for {
		locked, err := fileLock.TryLock()
		if err != nil {
			return errors.Wrap(err, "could not try locking param directory")
		}
		if locked {
			break
		}
		retries += 1
		if retries > 30 {
			// try to force the lock to be removed
			logie(os.Remove(filepath.Join(lock_dir, ".lock")))
		}
		if retries > 50 {
			return errors.New("could not obtain lock")
		}
		// if we didn't obtain the lock let's try again after a short delay
		time.Sleep(1 * time.Millisecond)
	}
	defer logwe(errors.Wrap(fileLock.Unlock(), "could not unlock params directory"))
	defer logde(errors.Wrap(os.Remove(filepath.Join(lock_dir, ".lock")), "could not remove params lock file"))

	err = os.Rename(tmpName, path)
	if err != nil {
		return errors.Wrap(err, "could not move temp param file to persistent location")
	}

	directory, err := os.Open(dir)
	if err != nil {
		return errors.Wrap(err, "could not open params directory")
	}

	err = directory.Sync()
	if err != nil {
		return errors.Wrap(err, "could not fsync params directory")
	}

	return nil
}

func RemoveParam(path string) error {
	dir := filepath.Dir(path)
	lock_dir := filepath.Dir(dir)
	fileLock := flock.New(filepath.Join(lock_dir, ".lock"))

	retries := 0
	for {
		locked, err := fileLock.TryLock()
		if err != nil {
			return errors.Wrap(err, "could not try locking params directory")
		}
		if locked {
			break
		}
		retries += 1
		if retries > 30 {
			// try to force the lock to be removed
			logde(os.Remove(filepath.Join(lock_dir, ".lock")))
		}
		if retries > 50 {
			return errors.New("could not obtain lock")
		}
		// if we didn't obtain the lock let's try again after a short delay
		time.Sleep(1 * time.Millisecond)
	}
	defer logwe(errors.Wrap(fileLock.Unlock(), "could not unlock params directory"))
	defer logde(errors.Wrap(os.Remove(filepath.Join(lock_dir, ".lock")), "could not remove params lock file"))

	os.Remove(path)

	directory, err := os.Open(dir)
	if err != nil {
		return errors.Wrap(err, "could not open params directory")
	}

	err = directory.Sync()
	if err != nil {
		return errors.Wrap(err, "could not fsync params directory")
	}

	return nil
}
