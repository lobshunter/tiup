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
	"strings"
	"sync"

	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
	"github.com/pingcap/tiup/server/pkg"
	"github.com/pingcap/tiup/server/session"
	"github.com/pingcap/tiup/server/store"
)

const Owner = "pingcap"

type server struct {
	root          string
	upstream      string
	upstreamCache *pkg.UpstreamCache
	keys          map[string]*v1manifest.KeyInfo
	sm            session.Manager
}

// NewServer returns a pointer to server
func newServer(rootDir, upstream, upstreamHome, indexKey, snapshotKey, timestampKey, ownerKey string) (*server, error) {
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
		Owner:                            ownerKey,
	}

	for ty, kfile := range kmap {
		k, err := model.LoadPrivateKey(kfile)
		if err != nil {
			return nil, err
		}
		s.keys[ty] = k
	}

	return s, nil
}

func (s *server) run(addr string) error {
	fmt.Println(addr)
	return http.ListenAndServe(addr, s.router())
}
