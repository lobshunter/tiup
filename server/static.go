// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	cjson "github.com/gibson042/canonicaljson-go"
	"github.com/google/uuid"
	"github.com/pingcap/errors"
	"github.com/pingcap/tiup/pkg/logger/log"
	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
	"github.com/pingcap/tiup/server/pkg"
)

// staticServer start a static web server
func (s *server) staticServer(local string, upstream string) http.Handler {
	fs := http.Dir(local)
	fsh := http.FileServer(fs)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HARDCODE: hardcoding here, only call updateUpstream when downloading timestamp.json
		if r.URL.Path == "timestamp.json" {
			log.Infof("----getting timestamp----")

			_, err, _ := s.upstreamCache.Single.Do("merge", func() (interface{}, error) {
				return nil, s.mergeUpstream()
			})

			if err != nil {
				log.Errorf("mergeUpstream error: %v", err)
			}
		}

		if f, err := fs.Open(path.Clean(r.URL.Path)); err == nil {
			f.Close()
		} else if os.IsNotExist(err) && upstream != "" {
			if err := proxyUpstream(w, r, upstream); err != nil {
				log.Errorf("Proxy upstream: %s", err.Error())
				fsh.ServeHTTP(w, r)
			}
			log.Errorf("Handle file: %s", err.Error())
			return
		}
		fsh.ServeHTTP(w, r)
	})
}

// mergeUpstream merge upstream manifests update to local manifests
func (s *server) mergeUpstream() (err error) {
	log.Infof("call mergeUpstream")

	defer func() {
		if err != nil {
			log.Errorf("mergeUpstream error stack: { %s\n}", errors.ErrorStack(err))
		}
	}()

	updatedFiles, err := s.upstreamCache.UpdateUpstream()
	if err != nil {
		return errors.Trace(err)
	}

	if len(updatedFiles) > 0 {
		log.Infof("----upstream updated----")
		uuid := uuid.New().String()
		err = s.sm.Begin(uuid)
		if err != nil {
			return errors.Trace(err)
		}

		txn := s.sm.Load(uuid)
		defer func() {
			if err != nil {
				txn.Rollback()
			}
		}()

		md := model.New(txn, s.keys)

		// merge snapshot
		var remoteSnapshot, localSnapshot model.SnapshotManifest
		if err = cjson.Unmarshal(updatedFiles[v1manifest.ManifestFilenameSnapshot], &remoteSnapshot); err != nil {
			return errors.Trace(err)
		}

		if err = txn.ReadManifest(v1manifest.ManifestFilenameSnapshot, &localSnapshot); err != nil {
			return errors.Trace(err)
		}

		if err = pkg.MergeSnapshot(&remoteSnapshot, &localSnapshot, s.keys[v1manifest.ManifestTypeSnapshot]); err != nil {
			return errors.Trace(err)
		}

		// incase root manifest rotates
		_, err = txn.Stat("root.json")
		if err != nil {
			if os.IsNotExist(err) {
				// not exist means server use root.json of upstream
				// so it's needed to use metadata of upstream
				localSnapshot.Signed.Meta["/root.json"] = remoteSnapshot.Signed.Meta["/root.json"]
			} else {
				return errors.Trace(err)
			}
		}

		// merge index
		// sometimes index is not updated along with snapshot and timestamp
		if indexData, needMergeIndex := updatedFiles[v1manifest.ManifestFilenameIndex]; needMergeIndex {
			var localIndex, remoteIndex model.IndexManifest
			indexVersion := localSnapshot.Signed.Meta["/index.json"].Version

			if err = cjson.Unmarshal(indexData, &remoteIndex); err != nil {
				return errors.Trace(err)
			}

			err = txn.ReadLocalManifest(fmt.Sprintf("%d.index.json", indexVersion), &localIndex)
			if os.IsNotExist(err) {
				err = pkg.MergeIndex(indexVersion+1, &remoteIndex, &remoteIndex, s.keys[v1manifest.ManifestTypeIndex])
				if err != nil {
					return errors.Trace(err)
				}
				// code can only reach here at startup, otherwise is a fatal error
				// no local index, no need to merge
			} else if err != nil {
				return errors.Trace(err)
			} else {
				err = pkg.MergeIndex(indexVersion+1, &localIndex, &remoteIndex, s.keys[v1manifest.ManifestTypeIndex])
				if err != nil {
					return errors.Trace(err)
				}
			}

			if err = txn.WriteManifest(fmt.Sprintf("%d.index.json", indexVersion+1), &remoteIndex); err != nil {
				return errors.Trace(err)
			}

			// update index stat in snapshot too
			indexStat, err := txn.Stat(fmt.Sprintf("%d.index.json", indexVersion+1))
			if err != nil {
				return errors.Trace(err)
			}
			localSnapshot.Signed.Meta["/"+v1manifest.ManifestFilenameIndex] = v1manifest.FileVersion{Version: indexVersion + 1, Length: uint(indexStat.Size())}
		}

		// merge component manifest
		delete(updatedFiles, v1manifest.ManifestFilenameTimestamp)
		delete(updatedFiles, v1manifest.ManifestFilenameSnapshot)
		delete(updatedFiles, v1manifest.ManifestFilenameIndex)
		for fileName, content := range updatedFiles {
			if strings.HasSuffix(fileName, v1manifest.ManifestFilenameRoot) {
				// root file should be handled seperatly
				continue
			}

			var remoteComp, localComp model.ComponentManifest
			compVersion := localSnapshot.Signed.Meta["/"+fileName].Version

			if err = cjson.Unmarshal(content, &remoteComp); err != nil {
				return errors.Trace(err)
			}

			err = txn.ReadLocalManifest(fmt.Sprintf("%d.%s", compVersion, fileName), &localComp)
			if os.IsNotExist(err) {
				err = pkg.MergeComponent(compVersion+1, &remoteComp, &remoteComp, s.keys[model.Owner])
				if err != nil {
					return errors.Trace(err)
				}
				// no need to merge
			} else if err != nil {
				return errors.Trace(err)
			} else {
				err = pkg.MergeComponent(compVersion+1, &localComp, &remoteComp, s.keys[model.Owner])
				if err != nil {
					return errors.Trace(err)
				}
			}

			if err = txn.WriteManifest(fmt.Sprintf("%d.%s", compVersion+1, fileName), &remoteComp); err != nil {
				return errors.Trace(err)
			}

			fileStat, err := txn.Stat(fmt.Sprintf("%d.%s", compVersion+1, fileName))
			if err != nil {
				return errors.Trace(err)
			}
			localSnapshot.Signed.Meta["/"+fileName] = v1manifest.FileVersion{Version: compVersion + 1, Length: uint(fileStat.Size())}
		}

		md.UpdateSnapshotManifest(time.Now(), func(*model.SnapshotManifest) *model.SnapshotManifest {
			return &localSnapshot
		})

		if err = md.UpdateTimestampManifest(time.Now()); err != nil {
			return errors.Trace(err)
		}

		log.Infof("-----commiting")
		if err = txn.Commit(); err == nil { // commit succeeded
			s.upstreamCache.UpdateCacheMTime()
		}
	}

	log.Infof("mergeUpstream succeeded")
	return nil
}

func proxyUpstream(w http.ResponseWriter, r *http.Request, upstream string) error {
	url, err := url.Parse(upstream)
	if err != nil {
		return errors.Trace(err)
	}

	r.Host = url.Host
	r.URL.Host = url.Host
	r.URL.Scheme = url.Scheme

	httputil.NewSingleHostReverseProxy(url).ServeHTTP(w, r)
	return nil
}

func (s *server) static(prefix, root, upstream string) http.Handler {
	return http.StripPrefix(prefix, s.staticServer(root, upstream))
}
