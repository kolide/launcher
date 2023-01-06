package notify

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kolide/launcher/ee/desktop/assets"
	"github.com/kolide/launcher/pkg/osquery"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func Test_setIconPath(t *testing.T) {
	t.Parallel()

	// Create a temp directory to use as our root directory
	rootDir := t.TempDir()

	// Test that if the icon doesn't exist in the root dir, the notifier will create it.
	iconPath, err := setIconPath(rootDir)
	require.NoError(t, err, "expected no error when setting icon path")
	require.True(t, strings.HasPrefix(iconPath, rootDir), "unexpected location for icon")
	require.True(t, strings.HasSuffix(iconPath, assets.KolideIconFilename), "unexpected file name for icon")

	// Test that if the icon already exists, the notifier will return the correct location.
	preexistingIconPath, err := setIconPath(rootDir)
	require.NoError(t, err, "expected no error when setting icon path")
	require.True(t, strings.HasPrefix(preexistingIconPath, rootDir), "unexpected location for icon")
	require.True(t, strings.HasSuffix(preexistingIconPath, assets.KolideIconFilename), "unexpected file name for icon")
}

func Test_notificationAlreadySent_markNotificationSent(t *testing.T) {
	t.Parallel()

	// Create a temp directory to hold our bbolt db
	dbDir := t.TempDir()

	// Create database; ensure we clean it up after the test
	db, err := bbolt.Open(filepath.Join(dbDir, "notifier_test.db"), 0600, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// The last of the test setup
	notifier, err := New(db, WithNotificationTtl(time.Hour*5))
	require.NoError(t, err, "could not set up db for test")
	testNotification := sentNotification{
		Title: "Test notification title",
		Body:  "Test notification body",
		UUID:  "c72f5e7a-8d36-46fc-8c1e-782d340681f9",
	}
	testNotificationKey := notificationKey(testNotification.Title, testNotification.Body)

	// Check a new notification that hasn't been sent before
	alreadySent := notifier.notificationAlreadySent(testNotification.Title, testNotification.Body)
	require.False(t, alreadySent, "empty db, notification should not show up as sent")

	// Mark notification as sent
	testNotification.SentAt = time.Now()
	notifier.markNotificationSent(testNotification)

	// Verify by looking in the db that the notification was marked as sent
	err = db.View(func(tx *bbolt.Tx) error {
		sentNotificationRaw := tx.Bucket([]byte(osquery.SentNotificationsBucket)).Get(testNotificationKey)
		require.NotNil(t, sentNotificationRaw, "expected sent notification to be in bucket")

		return nil
	})
	require.NoError(t, err, "could not view sent notification in db")

	// Confirm that calling notificationAlreadySent with the same notification returns true
	alreadySentAgain := notifier.notificationAlreadySent(testNotification.Title, testNotification.Body)
	require.True(t, alreadySentAgain, "previously-sent notification should show as previously sent")

	// Change the TTL to 0 to simulate the notification expiring -- confirm that it no longer returns true
	notifier.notificationTtl = 0 * time.Second
	alreadySentExpired := notifier.notificationAlreadySent(testNotification.Title, testNotification.Body)
	require.False(t, alreadySentExpired, "with a ttl of 0, previously-sent notification should not be counted")
}
