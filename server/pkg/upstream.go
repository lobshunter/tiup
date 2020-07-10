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
	"github.com/pingcap/tiup/pkg/repository"
	"golang.org/x/sync/singleflight"
)

const MAXFSEVENT = 64
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

	Debug("---- updated: %v", updated.Updated)
	for filename := range updated.Changedfile {
		Debug("---file: %s\n", filename)
	}
	return updated, nil
}

func (cache *UpstreamCache) updateUpstream() (updated interface{}, err error) {
	watcher, err := fsnotify.NewWatcher()
	watcher.Events = make(chan fsnotify.Event, MAXFSEVENT) // prevent lost event, poor approach
	if err != nil {
		return false, errors.Trace(err)
	}

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
		// FIXME:update manifest could stuck forever, may cause memory & goroutine leak
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
			println("------- capacity of channel", cap(watcher.Events))
			data, err := ioutil.ReadFile(event.Name)
			if err != nil {
				return result, errors.Trace(err)
			}

			Debug("changed %s\n", filepath.Base(event.Name))
			result.Changedfile[filepath.Base(event.Name)] = data
			result.Updated = true
		default:
			return result, nil
		}
	}
}
