package pkg

import (
	"strings"
	"time"

	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
)

const versionSuffix = "qa"

// MergeComponent merge manifest from src to dst
//
// Typically, src is local manifest needs update, dst is updated upstream manifest
func MergeComponent(src, dst *model.ComponentManifest, keys ...*v1manifest.KeyInfo) error {
	for platformName, platform := range src.Signed.Platforms {
		if dst.Signed.Platforms[platformName] == nil {
			dst.Signed.Platforms[platformName] = make(map[string]v1manifest.VersionItem)
		}
		for version, versionItem := range platform {
			// if dst doesn't have the version, or the version contains special prefix
			// use Item in src
			if _, ok := dst.Signed.Platforms[platformName][version]; (!ok) ||
				strings.HasPrefix(version, versionSuffix) {
				dst.Signed.Platforms[platformName][version] = versionItem
			}
		}
	}

	dst.Signed.Version = max(dst.Signed.Version, src.Signed.Version) + 1
	v1manifest.RenewManifest(&dst.Signed, time.Now())

	var err error
	dst.Signatures, err = model.Sign(dst.Signed, keys...)
	return err
}

// MergeIndex will merge ComponentItem from src to dst
//
// Typically, src is local manifest needs update, dst is updated upstream manifest
func MergeIndex(src, dst *model.IndexManifest, keys ...*v1manifest.KeyInfo) error {
	for compName, comp := range src.Signed.Components {
		if _, ok := dst.Signed.Components[compName]; !ok {
			dst.Signed.Components[compName] = comp
		}
	}

	dst.Signed.Version = max(dst.Signed.Version, src.Signed.Version) + 1
	v1manifest.RenewManifest(&dst.Signed, time.Now())

	var err error
	dst.Signatures, err = model.Sign(dst.Signed, keys...)
	return err
}

//
func MergeSnapshot(src, dst *model.SnapshotManifest, keys ...*v1manifest.KeyInfo) {
	for k, v := range src.Signed.Meta {
		if _, ok := dst.Signed.Meta[k]; !ok {
			dst.Signed.Meta[k] = v
		}
	}

	v1manifest.RenewManifest(&dst.Signed, time.Now())
}

func max(a, b uint) uint {
	if a > b {
		return a
	}
	return b
}
