package pkg

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fsnotify/fsnotify"
	"github.com/pingcap/errors"
	"github.com/pingcap/tiup/pkg/environment"
	"github.com/pingcap/tiup/pkg/localdata"
	"github.com/pingcap/tiup/pkg/repository"
	"golang.org/x/sync/singleflight"
)

const MAXFSEVENT = 64

type UpstreamCache struct {
	upstreamHome string
	tiupEnv      *environment.Environment
	single       *singleflight.Group
}

type UpdateUpstreamResult struct {
	Updated     bool
	Changedfile map[string][]byte
}

func NewUpstreamCache(upstreamHome string) *UpstreamCache {
	os.Setenv(localdata.EnvNameHome, upstreamHome)
	repoOpts := repository.Options{SkipVersionCheck: true, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	env, err := environment.InitEnv(repoOpts)
	if err != nil {
		return nil
	}

	return &UpstreamCache{
		upstreamHome: upstreamHome,
		tiupEnv:      env,
		single:       &singleflight.Group{},
	}
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

	err = watcher.Add(cache.upstreamHome + "/manifests")
	if err != nil {
		return false, errors.Trace(err)
	}

	err = cache.tiupEnv.V1Repository().UpdateComponentManifests()
	if err != nil {
		return false, errors.Trace(err)
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

		// time.Sleep(100 * time.Millisecond)
	}
}
