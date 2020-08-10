package pkg

import (
	"strings"
	"time"

	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
)

const versionSuffix = "qa"

// MergeComponent merge manifest and resign dst
func MergeComponent(version uint, src, dst *model.ComponentManifest, keys ...*v1manifest.KeyInfo) error {
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

	dst.Signed.Version = version
	v1manifest.RenewManifest(&dst.Signed, time.Now())

	var err error
	dst.Signatures, err = model.Sign(dst.Signed, keys...)
	return err
}

// MergeIndex merge manifest and resign dst
func MergeIndex(version uint, src, dst *model.IndexManifest, keys ...*v1manifest.KeyInfo) error {
	for compName, comp := range src.Signed.Components {
		if _, ok := dst.Signed.Components[compName]; !ok {
			dst.Signed.Components[compName] = comp
		}
	}

	// merge owner info
	for ownerId, ownerVal := range src.Signed.Owners {
		if dstOwner, hasOwner := dst.Signed.Owners[ownerId]; hasOwner {
			for keyId, keyVal := range ownerVal.Keys {
				if _, hasKey := dstOwner.Keys[keyId]; !hasKey {
					dst.Signed.Owners[ownerId].Keys[keyId] = keyVal
				}
			}
		} else {
			dst.Signed.Owners[ownerId] = ownerVal
		}
	}

	dst.Signed.Version = version
	v1manifest.RenewManifest(&dst.Signed, time.Now())

	var err error
	dst.Signatures, err = model.Sign(dst.Signed, keys...)
	return err
}

// MergeSnapshot merge manifest and resign dst
func MergeSnapshot(src, dst *model.SnapshotManifest, keys ...*v1manifest.KeyInfo) error {
	for k, v := range src.Signed.Meta {
		if _, ok := dst.Signed.Meta[k]; !ok {
			dst.Signed.Meta[k] = v
		}
	}

	v1manifest.RenewManifest(&dst.Signed, time.Now())

	var err error
	dst.Signatures, err = model.Sign(dst.Signed, keys...)
	return err
}
