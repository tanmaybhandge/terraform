package gcs

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/hashicorp/terraform/internal/backend"
	"github.com/hashicorp/terraform/internal/states/remote"
)

const (
	noPrefix        = ""
	noEncryptionKey = ""
)

// See https://cloud.google.com/storage/docs/using-encryption-keys#generating_your_own_encryption_key
var encryptionKey = "yRyCOikXi1ZDNE0xN3yiFsJjg7LGimoLrGFcLZgQoVk="

// var keyRingName = "gcs-backend-acceptance-tests"
// var keyRingLocation = "???"

func TestStateFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		prefix        string
		name          string
		wantStateFile string
		wantLockFile  string
	}{
		{"state", "default", "state/default.tfstate", "state/default.tflock"},
		{"state", "test", "state/test.tfstate", "state/test.tflock"},
		{"state", "test", "state/test.tfstate", "state/test.tflock"},
		{"state", "test", "state/test.tfstate", "state/test.tflock"},
	}
	for _, c := range cases {
		b := &Backend{
			prefix: c.prefix,
		}

		if got := b.stateFile(c.name); got != c.wantStateFile {
			t.Errorf("stateFile(%q) = %q, want %q", c.name, got, c.wantStateFile)
		}

		if got := b.lockFile(c.name); got != c.wantLockFile {
			t.Errorf("lockFile(%q) = %q, want %q", c.name, got, c.wantLockFile)
		}
	}
}

func TestRemoteClient(t *testing.T) {
	t.Parallel()

	bucket := bucketName(t)
	be := setupBackend(t, bucket, noPrefix, noEncryptionKey)
	defer teardownBackend(t, be, noPrefix)

	ss, err := be.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatalf("be.StateMgr(%q) = %v", backend.DefaultStateName, err)
	}

	rs, ok := ss.(*remote.State)
	if !ok {
		t.Fatalf("be.StateMgr(): got a %T, want a *remote.State", ss)
	}

	remote.TestClient(t, rs.Client)
}
func TestRemoteClientWithEncryption(t *testing.T) {
	t.Parallel()

	bucket := bucketName(t)
	be := setupBackend(t, bucket, noPrefix, encryptionKey)
	defer teardownBackend(t, be, noPrefix)

	ss, err := be.StateMgr(backend.DefaultStateName)
	if err != nil {
		t.Fatalf("be.StateMgr(%q) = %v", backend.DefaultStateName, err)
	}

	rs, ok := ss.(*remote.State)
	if !ok {
		t.Fatalf("be.StateMgr(): got a %T, want a *remote.State", ss)
	}

	remote.TestClient(t, rs.Client)
}

func TestRemoteLocks(t *testing.T) {
	t.Parallel()

	bucket := bucketName(t)
	be := setupBackend(t, bucket, noPrefix, noEncryptionKey)
	defer teardownBackend(t, be, noPrefix)

	remoteClient := func() (remote.Client, error) {
		ss, err := be.StateMgr(backend.DefaultStateName)
		if err != nil {
			return nil, err
		}

		rs, ok := ss.(*remote.State)
		if !ok {
			return nil, fmt.Errorf("be.StateMgr(): got a %T, want a *remote.State", ss)
		}

		return rs.Client, nil
	}

	c0, err := remoteClient()
	if err != nil {
		t.Fatalf("remoteClient(0) = %v", err)
	}
	c1, err := remoteClient()
	if err != nil {
		t.Fatalf("remoteClient(1) = %v", err)
	}

	remote.TestRemoteLocks(t, c0, c1)
}

func TestBackend(t *testing.T) {
	t.Parallel()

	bucket := bucketName(t)

	be0 := setupBackend(t, bucket, noPrefix, noEncryptionKey)
	defer teardownBackend(t, be0, noPrefix)

	be1 := setupBackend(t, bucket, noPrefix, noEncryptionKey)

	backend.TestBackendStates(t, be0)
	backend.TestBackendStateLocks(t, be0, be1)
	backend.TestBackendStateForceUnlock(t, be0, be1)
}

func TestBackendWithPrefix(t *testing.T) {
	t.Parallel()

	prefix := "test/prefix"
	bucket := bucketName(t)

	be0 := setupBackend(t, bucket, prefix, noEncryptionKey)
	defer teardownBackend(t, be0, prefix)

	be1 := setupBackend(t, bucket, prefix+"/", noEncryptionKey)

	backend.TestBackendStates(t, be0)
	backend.TestBackendStateLocks(t, be0, be1)
}
func TestBackendWithEncryption(t *testing.T) {
	t.Parallel()

	bucket := bucketName(t)

	be0 := setupBackend(t, bucket, noPrefix, encryptionKey)
	defer teardownBackend(t, be0, noPrefix)

	be1 := setupBackend(t, bucket, noPrefix, encryptionKey)

	backend.TestBackendStates(t, be0)
	backend.TestBackendStateLocks(t, be0, be1)
}

// func TestBackendWithKMS(t *testing.T) {
// 	t.Parallel()

// 	bucket := bucketName(t)
// 	keyName := keyName(t)
// 	projectID := os.Getenv("GOOGLE_PROJECT")

// 	kmsDetails := map[string]string{
// 		"project":  projectID,
// 		"location": keyRingLocation,
// 		"key_ring": keyRingName,
// 		"key":      keyName,
// 	}
// 	// kmsName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
// 	// 	kmsDetails["project"],
// 	// 	kmsDetails["location"],
// 	// 	kmsDetails["key_ring"],
// 	// 	kmsDetails["key"],
// 	// )

// 	setupKey(t, kmsDetails)
// 	be0 := setupBackend(t, bucket, noPrefix, noEncryptionKey, kmsDetails)
// 	defer teardownBackend(t, be0, noPrefix)

// 	be1 := setupBackend(t, bucket, noPrefix, noEncryptionKey, kmsDetails)

// 	backend.TestBackendStates(t, be0)
// 	backend.TestBackendStateLocks(t, be0, be1)
// }

// setupBackend returns a new GCS backend.
// func setupBackend(t *testing.T, bucket, prefix, key string, kmsDetails map[string]interface{}) backend.Backend {
func setupBackend(t *testing.T, bucket, prefix, key string) backend.Backend {
	t.Helper()

	projectID := os.Getenv("GOOGLE_PROJECT")
	if projectID == "" || os.Getenv("TF_ACC") == "" {
		t.Skip("This test creates a bucket in GCS and populates it. " +
			"Since this may incur costs, it will only run if " +
			"the TF_ACC and GOOGLE_PROJECT environment variables are set.")
	}

	config := map[string]interface{}{
		"bucket":         bucket,
		"prefix":         prefix,
		"encryption_key": key,
		// "kms_key":        kmsDetails,
	}

	b := backend.TestBackendConfig(t, New(), backend.TestWrapConfig(config))
	be := b.(*Backend)

	// create the bucket if it doesn't exist
	bkt := be.storageClient.Bucket(bucket)
	_, err := bkt.Attrs(be.storageContext)
	if err != nil {
		if err != storage.ErrBucketNotExist {
			t.Fatal(err)
		}

		attrs := &storage.BucketAttrs{
			Location: os.Getenv("GOOGLE_REGION"),
		}
		err := bkt.Create(be.storageContext, projectID, attrs)
		if err != nil {
			t.Fatal(err)
		}
	}

	return b
}

// // setupKey creates a new KMS Key in an existing KMS Keyring.
// func setupKey(t *testing.T, keyDetails map[string]string) {
// 	// Create key using details
// }

// teardownBackend deletes all states from be except the default state.
func teardownBackend(t *testing.T, be backend.Backend, prefix string) {
	t.Helper()
	gcsBE, ok := be.(*Backend)
	if !ok {
		t.Fatalf("be is a %T, want a *gcsBackend", be)
	}
	ctx := gcsBE.storageContext

	bucket := gcsBE.storageClient.Bucket(gcsBE.bucketName)
	objs := bucket.Objects(ctx, nil)

	for o, err := objs.Next(); err == nil; o, err = objs.Next() {
		if err := bucket.Object(o.Name).Delete(ctx); err != nil {
			log.Printf("Error trying to delete object: %s %s\n\n", o.Name, err)
		} else {
			log.Printf("Object deleted: %s", o.Name)
		}
	}

	// Delete the bucket itself.
	if err := bucket.Delete(ctx); err != nil {
		t.Errorf("deleting bucket %q failed, manual cleanup may be required: %v", gcsBE.bucketName, err)
	}
}

// // teardownKmsKeys disables the Cloud KMS Key used for testing
// func teardownKmsKeys(t *testing.T, keyLocaton, keyRing, keyName string) {
// 	t.Helper()

// 	// No existing KMS Client in backend.Backend in test
// 	ctx := context.TODO()
// 	c, err := kms.NewKeyManagementClient(ctx)
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	defer c.Close()

// 	// Iterate through Keys in Key Chain and delete
// 	// Note: Key Chains cannot be deleted from projects, only emptied

// 	req := &kmspb.ListCryptoKeysRequest{
// 		// TODO: Fill request struct fields.
// 		// See https://pkg.go.dev/google.golang.org/genproto/googleapis/cloud/kms/v1#ListCryptoKeysRequest.
// 	}
// 	it := c.ListCryptoKeys(ctx, req)
// 	for {
// 		resp, err := it.Next()
// 		if err == iterator.Done {
// 			break
// 		}
// 		if err != nil {
// 			t.Errorf("error iterating through keys in %s keyring: %v", keyRingName, err)
// 		}
// 		// TODO: Use resp.
// 		_ = resp
// 	}

// 	projectID := os.Getenv("GOOGLE_PROJECT")
// }

// bucketName returns a valid bucket name for this test.
func bucketName(t *testing.T) string {
	name := fmt.Sprintf("tf-%x-%s", time.Now().UnixNano(), t.Name())

	// Bucket names must contain 3 to 63 characters.
	if len(name) > 63 {
		name = name[:63]
	}

	return strings.ToLower(name)
}

// // keyName returns a valid key name for this test.
// func keyName(t *testing.T) string {
// 	name := fmt.Sprintf("tf-key-%x-%s", time.Now().UnixNano(), t.Name())

// 	// Key names must match regex : [a-zA-Z0-9_-]{1,63}
// 	if len(name) > 63 {
// 		name = name[:63]
// 	}
// 	re := regexp.MustCompile("[a-zA-Z0-9_-]{1,63}")
// 	ok := re.Match([]byte(name))
// 	if !ok {
// 		t.Errorf("cannot use invalid key name: %s", name)
// 	}

// 	return strings.ToLower(name)
// }
