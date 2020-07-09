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
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"sync"

	cjson "github.com/gibson042/canonicaljson-go"
	"github.com/pingcap/errors"
	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
	"github.com/pingcap/tiup/server/pkg"
	"github.com/pingcap/tiup/server/session"
	"github.com/pingcap/tiup/server/store"
	tools "github.com/pingcap/tiup/server/tools/pkg"
)

type server struct {
	root          string
	upstream      string
	upstreamCache *pkg.UpstreamCache
	keys          map[string]*v1manifest.KeyInfo
	sm            session.Manager
}

// NewServer returns a pointer to server
func newServer(rootDir, upstream, upstreamHome, indexKey, snapshotKey, timestampKey, ownerKey, ownerPubKey string) (*server, error) {
	upstreamHome = strings.TrimSuffix(upstreamHome, "/")
	upstreamCache := pkg.NewUpstreamCache(upstreamHome)

	s := &server{
		root:          rootDir,
		upstream:      upstream,
		upstreamCache: upstreamCache,
		keys:          make(map[string]*v1manifest.KeyInfo),
		sm:            session.New(store.NewStore(rootDir, upstream), new(sync.Map)),
	}

	kmap := map[string]string{
		v1manifest.ManifestTypeIndex:     indexKey,
		v1manifest.ManifestTypeSnapshot:  snapshotKey,
		v1manifest.ManifestTypeTimestamp: timestampKey,
		model.Owner:                      ownerKey,
	}

	for ty, kfile := range kmap {
		k, err := model.LoadPrivateKey(kfile)
		if err != nil {
			return nil, err
		}
		s.keys[ty] = k
	}

	if err := s.mergeUpstream(); err != nil {
		return nil, errors.Trace(err)
	}

	if err := s.ensureOwnerKey("pingcap", ownerPubKey); err != nil {
		return nil, errors.Trace(err)
	}

	return s, nil
}

// ensureOwnerKey ensure the owner key is in index.json
func (s *server) ensureOwnerKey(ownerId string, publicFile string) error {
	timestampFile := path.Join(s.root, v1manifest.ManifestFilenameTimestamp)
	snapshotFile := path.Join(s.root, v1manifest.ManifestFilenameSnapshot)

	data, err := ioutil.ReadFile(snapshotFile)
	if err != nil {
		return errors.Trace(err)
	}

	var snapshot model.SnapshotManifest
	err = cjson.Unmarshal(data, &snapshot)
	if err != nil {
		return errors.Trace(err)
	}

	indexVersion := snapshot.Signed.Meta[v1manifest.ManifestURLIndex].Version
	indexFile := path.Join(s.root, fmt.Sprintf("%d.index.json", indexVersion))

	return tools.AddOwnerKey(ownerId, publicFile, indexFile, snapshotFile, timestampFile, s.keys)
}

func (s *server) run(addr string) error {
	fmt.Println(addr)
	return http.ListenAndServe(addr, s.router())
}
