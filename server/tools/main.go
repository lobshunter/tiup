package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	cjson "github.com/gibson042/canonicaljson-go"
	"github.com/pingcap/tiup/pkg/repository/crypto"
	"github.com/pingcap/tiup/pkg/repository/v1manifest"
	"github.com/pingcap/tiup/server/model"
	"github.com/pingcap/tiup/server/tools/pkg"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:   fmt.Sprintf("%s <command>", os.Args[0]),
		Short: "tools for tiup-server development",
		// Args: func(cmd *cobra.Command, args []string) error {
		// 	return nil
		// },
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newJsonCmd(),
		newAddOwnerKeyCmd(),
		newExtractPubKeyCmd(),
	)

	if err := cmd.Execute(); err != nil {
		log.Printf("Execute error: %v", err)
	}
}

func newExtractPubKeyCmd() *cobra.Command {
	var outFile string

	cmd := &cobra.Command{
		Use:   "getpub <private key file>",
		Short: "extract public key from private key file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cmd.Help()
			}

			privateFile := args[0]
			if len(outFile) == 0 {
				outFile = "public.json"
			}

			return extractPubKey(privateFile, outFile)
		},
	}

	cmd.Flags().StringVarP(&outFile, "outfile", "o", outFile, "specify outout file")

	return cmd
}

func extractPubKey(privateFile string, outFile string) error {
	privateData, err := ioutil.ReadFile(privateFile)
	if err != nil {
		return err
	}

	var private v1manifest.KeyInfo
	if err = cjson.Unmarshal(privateData, &private); err != nil {
		return err
	}

	////
	var privateKey crypto.RSAPrivKey
	if err = privateKey.Deserialize([]byte(private.Value["private"])); err != nil {
		return err
	}

	publicKey := privateKey.Public().(*crypto.RSAPubKey)
	pubdata, err := publicKey.Serialize()
	if err != nil {
		return err
	}

	public := v1manifest.KeyInfo{
		Scheme: private.Scheme,
		Type:   private.Type,
		Value:  map[string]string{"public": string(pubdata)},
	}

	data, err := cjson.Marshal(public)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(outFile, data, pkg.DEFAULT_FILE_MODE)
}

func newAddOwnerKeyCmd() *cobra.Command {
	index := v1manifest.ManifestFilenameIndex
	snapshot := v1manifest.ManifestFilenameSnapshot
	timestamp := v1manifest.ManifestFilenameTimestamp
	var indexKey, snapshotKey, timestampKey string

	cmd := &cobra.Command{
		Use:   "owneradd <owner ID> <public key file>",
		Short: "add a public key to index.json for the owner",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return cmd.Help()
			}

			owner, pubKey := args[0], args[1]

			var err error
			keys := make(map[string]*v1manifest.KeyInfo)
			if keys[v1manifest.ManifestTypeIndex], err = model.LoadPrivateKey(indexKey); err != nil {
				return err
			}
			if keys[v1manifest.ManifestTypeSnapshot], err = model.LoadPrivateKey(snapshotKey); err != nil {
				return err
			}
			if keys[v1manifest.ManifestTypeTimestamp], err = model.LoadPrivateKey(timestampKey); err != nil {
				return err
			}

			return pkg.AddOwnerKey(owner, pubKey, index, snapshot, timestamp, keys)
		},
	}

	cmd.Flags().StringVar(&index, "index", index, "specify index file to update")
	cmd.Flags().StringVar(&snapshot, "snapshot", snapshot, "specify snapshot file to update")
	cmd.Flags().StringVar(&timestamp, "timestamp", timestamp, "specify timestamp file to update")
	cmd.Flags().StringVar(&indexKey, "indexKey", "", "specific the private key for index")
	cmd.Flags().StringVar(&snapshotKey, "snapshotKey", "", "specific the private key for snapshot")
	cmd.Flags().StringVar(&timestampKey, "timestampKey", "", "specific the private key for timestamp")

	return cmd
}

func newJsonCmd() *cobra.Command {
	outFile := ""

	cmd := &cobra.Command{
		Use:   "json <filepath>",
		Short: "canonify json file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return cmd.Help()
			}

			inFile := args[0]
			if len(outFile) == 0 {
				outFile = inFile
			}

			return canonify(inFile, outFile)
		},
	}

	cmd.Flags().StringVarP(&outFile, "outfile", "o", "", "specify output file")

	return cmd
}

func canonify(inFile string, outFile string) error {
	data, err := ioutil.ReadFile(inFile)
	if err != nil {
		return err
	}

	var anyJson map[string]interface{}
	err = cjson.Unmarshal(data, &anyJson)
	if err != nil {
		return err
	}
	canonified, err := cjson.Marshal(anyJson)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(outFile, canonified, pkg.DEFAULT_FILE_MODE)
}
