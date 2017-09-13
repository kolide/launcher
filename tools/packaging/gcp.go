package packaging

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
)

// SetGCPProject will set the local GCP project to the supplied project name
func SetGCPProject(project string) error {
	cmd := exec.Command("gcloud", "config", "set", "project", project)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CopyContentsToCloudStorage recursively copies contents of the path uploadRoot to
// the named cloud storage bucket. Items that exist in cloud storage but not in uploadRoot
// are removed
func CopyContentsToCloudStorage(uploadRoot, bucketName string) error {
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return errors.Wrapf(err, "preparing to copy %s to storage", uploadRoot)
	}
	defer client.Close()
	// Clear out old objects. This is a fairly naive way to do this. We could use paging
	// to grab batches of object attributes, record their names in a lookup table, and then remove
	// the name if an object of the same name is uploaded, then delete what's left.
	iter := client.Bucket(bucketName).Objects(ctx, nil)
	for {
		objAttr, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return errors.Wrap(err, "deleting existing objects")
		}
		if err := client.Bucket(bucketName).Object(objAttr.Name).Delete(ctx); err != nil {
			return errors.Wrapf(err, "deleting %s", objAttr.Name)
		}
	}
	// recursively upload new objects
	err = filepath.Walk(uploadRoot, func(path string, info os.FileInfo, _ error) error {
		if info.IsDir() {
			return nil
		}
		objectName := strings.TrimLeft(strings.Replace(path, uploadRoot, "", 1), "/")
		obj := client.Bucket(bucketName).Object(objectName)
		rdr, err := os.Open(path)
		if err != nil {
			return err
		}
		defer rdr.Close()
		wtr := obj.NewWriter(ctx)
		defer wtr.Close()
		if _, err = io.Copy(wtr, rdr); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "uploading from %s", uploadRoot)
	}
	return nil
}
