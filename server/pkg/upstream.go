package pkg

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/pingcap/errors"
	"github.com/pingcap/tiup/pkg/environment"
	"github.com/pingcap/tiup/pkg/localdata"
	"github.com/pingcap/tiup/pkg/logger/log"
	"github.com/pingcap/tiup/pkg/repository"
	"golang.org/x/sync/singleflight"
)

const MAXFSEVENT = 512
const UPSTREAM_TIMEOUT = 60 * time.Second

type UpstreamCache struct {
	tiupEnv      *environment.Environment
	ManifestsDir string
	Single       *singleflight.Group
	MTimeCache   map[string]time.Time
}

func NewUpstreamCache(upstreamHome string) (*UpstreamCache, error) {
	os.Setenv(localdata.EnvNameHome, upstreamHome)
	repoOpts := repository.Options{SkipVersionCheck: true, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	env, err := environment.InitEnv(repoOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &UpstreamCache{
		ManifestsDir: upstreamHome + "/manifests",
		tiupEnv:      env,
		Single:       &singleflight.Group{},
		MTimeCache:   make(map[string]time.Time),
	}, nil
}

func (cache *UpstreamCache) UpdateUpstream() (updatedFiles map[string][]byte, err error) {
	updatedFiles = make(map[string][]byte)

	// make sure rootDir exists
	if _, err := os.Stat(cache.ManifestsDir); err != nil {
		if os.IsNotExist(err) {
			// HARDCODE: hardcoded file mode here
			if err = os.Mkdir(cache.ManifestsDir, 0751); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	// call tiup list to udpate manifests
	errCh := make(chan error)
	go func() {
		// UpdateComponentManifests could block for a very longtime
		// duration depends the underlying TCP stack, technically this won't cause memory leak
		// using channel here is to prevent function from blocking the main thread
		errCh <- cache.tiupEnv.V1Repository().UpdateComponentManifests()
	}()

	select {
	case err = <-errCh:
		if err != nil {
			return nil, errors.Trace(err)
		}

	case <-time.After(UPSTREAM_TIMEOUT):
		return nil, errors.New("get upstream manifests timeout")
	}

	// get updated manifests files
	err = filepath.Walk(cache.ManifestsDir, func(path string, info os.FileInfo, err error) error {
		if path == cache.ManifestsDir {
			return nil
		}

		if err != nil {
			return errors.Trace(err)
		}

		if cache.MTimeCache[info.Name()] != info.ModTime() { // file modified
			log.Debugf("ReadFile: %s", cache.ManifestsDir+"/"+info.Name())
			data, err := ioutil.ReadFile(cache.ManifestsDir + "/" + info.Name())
			if err != nil {
				return errors.Trace(err)
			}

			updatedFiles[info.Name()] = data
		}

		return nil
	})

	return updatedFiles, err
}

func (cache *UpstreamCache) UpdateCacheMTime() error {
	return filepath.Walk(cache.ManifestsDir, func(path string, info os.FileInfo, err error) error {
		if path == cache.ManifestsDir {
			return nil
		}

		if err != nil {
			return errors.Trace(err)
		}

		cache.MTimeCache[info.Name()] = info.ModTime()
		return nil
	})
}
