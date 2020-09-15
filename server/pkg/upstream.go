package pkg

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fsnotify/fsnotify"
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
	upstreamHome string
	tiupEnv      *environment.Environment
	single       *singleflight.Group
}

type UpdateUpstreamResult struct {
	Updated     bool
	Changedfile map[string][]byte
}

func NewUpstreamCache(upstreamHome string) (*UpstreamCache, error) {
	os.Setenv(localdata.EnvNameHome, upstreamHome)
	repoOpts := repository.Options{SkipVersionCheck: true, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	env, err := environment.InitEnv(repoOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &UpstreamCache{
		upstreamHome: upstreamHome,
		tiupEnv:      env,
		single:       &singleflight.Group{},
	}, nil
}

func (cache *UpstreamCache) UpdateUpstream() (updated UpdateUpstreamResult, err error) {
	result, err, _ := cache.single.Do("updateUpstream", cache.updateUpstream)
	if err != nil {
		return UpdateUpstreamResult{Updated: false}, errors.Trace(err)
	}
	updated = result.(UpdateUpstreamResult)

	log.Infof("---- updated: %v", updated.Updated)
	for filename := range updated.Changedfile {
		log.Infof("---file: %s\n", filename)
	}
	return updated, nil
}

func (cache *UpstreamCache) updateUpstream() (updated interface{}, err error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return false, errors.Trace(err)
	}

	defer watcher.Close()
	watcher.Events = make(chan fsnotify.Event, MAXFSEVENT) // prevent lost event, poor approach

	if _, err := os.Stat(cache.upstreamHome + "/manifests"); err != nil {
		if os.IsNotExist(err) {
			// FIXME: hardcoding filemode
			if err = os.Mkdir(cache.upstreamHome+"/manifests", 0751); err != nil {
				return false, errors.Trace(err)
			}
		}
	}

	if err = watcher.Add(cache.upstreamHome + "/manifests"); err != nil {
		return false, errors.Trace(err)
	}

	errCh := make(chan error)
	go func() {
		errCh <- cache.tiupEnv.V1Repository().UpdateComponentManifests()
	}()

	select {
	case err = <-errCh:
		if err != nil {
			return false, errors.Trace(err)
		}
	case <-time.After(UPSTREAM_TIMEOUT):
		return false, errors.New("update upstream timeout")
	}

	result := UpdateUpstreamResult{
		Changedfile: make(map[string][]byte),
	}

	for {
		select {
		case event := <-watcher.Events:
			data, err := ioutil.ReadFile(event.Name)
			if err != nil {
				return result, errors.Trace(err)
			}

			log.Infof("changed %s\n", filepath.Base(event.Name))
			result.Changedfile[filepath.Base(event.Name)] = data
			result.Updated = true
		default:
			return result, nil
		}
	}
}
