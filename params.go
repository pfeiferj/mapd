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
	LAST_GPS_POSITION_PERSIST = ParamPath("LastGPSPosition", false)
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
