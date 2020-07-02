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
		// NOTE: hardcoding here, only call updateUpstream when downloading timestamp
		if r.URL.Path == "timestamp.json" {
			pkg.Log("----getting timestamp----")
			err := s.mergeUpstream()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(err.Error()))
				return
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

func (s *server) mergeUpstream() error {
	result, err := s.upstreamCache.UpdateUpstream()
	if err != nil {
		return errors.Trace(err)
	}

	if result.Updated {
		pkg.Log("----upstream updated----")
		uuid := uuid.New().String()
		err = s.sm.Begin(uuid)
		if err != nil {
			return errors.Trace(err)
		}

		txn := s.sm.Load(uuid)
		md := model.New(txn, s.keys)

		var localSnapshot model.SnapshotManifest
		err = txn.ReadManifest(v1manifest.ManifestFilenameSnapshot, &localSnapshot)
		if err != nil {
			return errors.Trace(err)
		}

		// FIXME: retry for reason
		{
			// merge index
			var localIndex, remoteIndex model.IndexManifest
			indexVersion := localSnapshot.Signed.Meta["/index.json"].Version
			txn.ReadLocalManifest(fmt.Sprintf("%d.index.json", indexVersion), &localIndex)
			err = cjson.Unmarshal(result.Changedfile[v1manifest.ManifestFilenameIndex], &remoteIndex)
			if os.IsNotExist(err) {
				// no local index
			} else if err != nil {
				return errors.Trace(err)
			} else {
				err = pkg.MergeIndex(&localIndex, &remoteIndex)
				if err != nil {
					return errors.Trace(err)
				}
			}

			v1manifest.RenewManifest(&remoteIndex.Signed, time.Now())
			remoteIndex.Signatures, err = model.Sign(remoteIndex.Signed, s.keys[v1manifest.ManifestTypeIndex])
			err = txn.WriteManifest(fmt.Sprintf("%d.index.json", indexVersion+1), &remoteIndex)
			if err != nil {
				return errors.Trace(err)
			}

			indexStat, err := txn.Stat(fmt.Sprintf("%d.index.json", indexVersion+1))
			if err != nil {
				return errors.Trace(err)
			}
			localSnapshot.Signed.Meta["/"+v1manifest.ManifestFilenameIndex] = v1manifest.FileVersion{Version: indexVersion + 1, Length: uint(indexStat.Size())}

			// merge snapshot
			var remoteSnapshot model.SnapshotManifest
			err = cjson.Unmarshal(result.Changedfile[v1manifest.ManifestFilenameSnapshot], &remoteSnapshot)
			if err != nil {
				return errors.Trace(err)
			}
			pkg.MergeSnapshot(&remoteSnapshot, &localSnapshot)

			// merge component manifest
			delete(result.Changedfile, v1manifest.ManifestFilenameRoot)
			delete(result.Changedfile, v1manifest.ManifestFilenameTimestamp)
			delete(result.Changedfile, v1manifest.ManifestFilenameIndex)
			for fileName, content := range result.Changedfile {
				var remoteComp, localComp model.ComponentManifest
				compVersion := localSnapshot.Signed.Meta["/"+fileName].Version
				err = txn.ReadManifest(fmt.Sprintf("%d.%s", compVersion, fileName), &localComp)
				if os.IsNotExist(err) { // DEBUG: ???
					continue
				}

				err = cjson.Unmarshal(content, &remoteComp)
				if err != nil {
					return errors.Trace(err)
				}
				err = pkg.MergeComponent(&localComp, &remoteComp)
				if err != nil {
					return errors.Trace(err)
				}

				v1manifest.RenewManifest(&remoteComp.Signed, time.Now())
				remoteComp.Signatures, err = model.Sign(remoteComp.Signed, s.keys[Owner])
				err = txn.WriteManifest(fmt.Sprintf("%d.%s", compVersion+1, fileName), &remoteComp)
				if err != nil {
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
			err = md.UpdateTimestampManifest(time.Now())
			if err != nil {
				return errors.Trace(err)
			}

			pkg.Log("----- commiting")
			return txn.Commit()
		}
	}
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
