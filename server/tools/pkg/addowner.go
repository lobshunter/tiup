package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"

	cjson "github.com/gibson042/canonicaljson-go"
	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
)

const DEFAULT_FILE_MODE = 0644

func AddOwnerKey(owner, publicFile, indexFile, snapshotFile, timestampFile string, keys map[string]*v1manifest.KeyInfo) error {
	publicData, err := ioutil.ReadFile(publicFile)
	if err != nil {
		return err
	}
	indexData, err := ioutil.ReadFile(indexFile)
	if err != nil {
		return err
	}
	snapshotData, err := ioutil.ReadFile(snapshotFile)
	if err != nil {
		return err
	}
	timestampData, err := ioutil.ReadFile(timestampFile)
	if err != nil {
		return err
	}

	var public v1manifest.KeyInfo
	var index model.IndexManifest
	var snapshot model.SnapshotManifest
	var timestamp model.TimestampManifest

	if err = cjson.Unmarshal(publicData, &public); err != nil {
		return err
	}
	if err = cjson.Unmarshal(indexData, &index); err != nil {
		return err
	}
	if err = cjson.Unmarshal(snapshotData, &snapshot); err != nil {
		return err
	}
	if err = cjson.Unmarshal(timestampData, &timestamp); err != nil {
		return err
	}

	id, err := public.ID()
	if err != nil {
		return err
	}

	// update index
	index.Signed.Owners[owner].Keys[id] = &public
	index.Signed.Version++
	index.Signatures, err = model.Sign(index.Signed, keys[v1manifest.ManifestTypeIndex])
	if err != nil {
		return err
	}

	data, err := cjson.Marshal(index)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(indexFile, data, DEFAULT_FILE_MODE); err != nil {
		return err
	}

	// update snapshot
	indexStat, err := os.Stat(indexFile)
	if err != nil {
		return err
	}

	snapshot.Signed.Meta["/index.json"] = v1manifest.FileVersion{
		Version: index.Signed.Version,
		Length:  uint(indexStat.Size()),
	}

	snapshot.Signatures, err = model.Sign(snapshot.Signed, keys[v1manifest.ManifestTypeSnapshot])
	if err != nil {
		return err
	}

	data, err = cjson.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(snapshotFile, data, DEFAULT_FILE_MODE); err != nil {
		return err
	}

	// update timestamp
	snapStat, err := os.Stat(snapshotFile)
	if err != nil {
		return err
	}

	snap, err := os.Open(snapshotFile)
	if err != nil {
		return err
	}
	defer snap.Close()

	h := sha256.New()
	if _, err = io.Copy(h, snap); err != nil {
		return err
	}
	timestamp.Signed.Version++
	timestamp.Signed.Meta["/snapshot.json"] = v1manifest.FileHash{
		Hashes: map[string]string{
			v1manifest.SHA256: hex.EncodeToString(h.Sum(nil)),
		},
		Length: uint(snapStat.Size()),
	}
	timestamp.Signatures, err = model.Sign(timestamp.Signed, keys[v1manifest.ManifestTypeTimestamp])
	if err != nil {
		return err
	}
	data, err = cjson.Marshal(timestamp)
	if err != nil {
		return err
	}
	if err = ioutil.WriteFile(timestampFile, data, DEFAULT_FILE_MODE); err != nil {
		return err
	}

	return nil
}
