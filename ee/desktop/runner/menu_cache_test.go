package runner

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/base_menu.json
var baseMenu []byte

//go:embed testdata/menu_pending_registration.json
var pendingRegistrationMenu []byte

//go:embed testdata/menu_blocked_status.json
var blockedStatusMenu []byte

func Test_menuItemCache_recordMenuUpdates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args [][]byte
		want [][]menuChangeSet
	}{
		{
			name: "startup behavior",
			args: [][]byte{baseMenu},
			want: [][]menuChangeSet{
				[]menuChangeSet{ // we always detect new sections on start up
					{"", "tester@example.test"},
					{"", "About Kolide..."},
					{"", "✔ All Good!"},
					{"", "View Details..."},
					{"", "Debug"},
				},
			},
		},
		{
			name: "new pending registration",
			args: [][]byte{baseMenu, pendingRegistrationMenu},
			want: [][]menuChangeSet{
				[]menuChangeSet{
					{"", "tester@example.test"},
					{"", "About Kolide..."},
					{"", "✔ All Good!"},
					{"", "View Details..."},
					{"", "Debug"},
				},
				[]menuChangeSet{ // the second run should only view the pending request as new
					{"", "❗ Pending Registration Request"},
				},
			},
		},
		{
			name: "removed pending registration",
			args: [][]byte{pendingRegistrationMenu, baseMenu},
			want: [][]menuChangeSet{
				[]menuChangeSet{
					{"", "tester@example.test"},
					{"", "About Kolide..."},
					{"", "✔ All Good!"},
					{"", "View Details..."},
					{"", "❗ Pending Registration Request"},
					{"", "Debug"},
				},
				[]menuChangeSet{ // the second run should only remove the pending request
					{"❗ Pending Registration Request", ""},
				},
			},
		},
		{
			name: "moved to blocked status",
			args: [][]byte{baseMenu, blockedStatusMenu},
			want: [][]menuChangeSet{
				[]menuChangeSet{
					{"", "tester@example.test"},
					{"", "About Kolide..."},
					{"", "✔ All Good!"},
					{"", "View Details..."},
					{"", "Debug"},
				},
				[]menuChangeSet{ // the second run should detect the status change and the new blocked details section
					{"", "Device Blocked"},
					{"", "❌ Fix 2 Failing Checks..."},
					{"✔ All Good!", ""},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newMenuItemCache()
			for idx, menuData := range tt.args {
				got, _ := m.recordMenuUpdates(menuData)
				require.Equal(t, tt.want[idx], got)
			}
		})
	}
}
