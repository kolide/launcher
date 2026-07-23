//go:build darwin

package pkgutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kolide/launcher/v2/ee/tables/pkgutil/mocks"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/osquery/osquery-go/plugin/table"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestGeneratePkgutilData(t *testing.T) {
	t.Parallel()

	type args struct {
		queryContext table.QueryContext
	}

	var tests = []struct {
		name           string
		args           args
		execReturnFile string
		want           []map[string]string
		assertion      assert.ErrorAssertionFunc
	}{
		{
			name: "valid nonempty results",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"package_id": {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{}},
						"volume":     {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{}},
					},
				},
			},
			execReturnFile: "valid_nonempty.output",
			want: []map[string]string{
				{
					"package_id": "com.apple.pkg.CLTools_SDK_macOS13",
					"volume":     rootVolume,
				},
				{
					"package_id": "org.golang.go",
					"volume":     rootVolume,
				},
				{
					"package_id": "com.tinyspeck.slackmacgap",
					"volume":     rootVolume,
				},
				{
					"package_id": "com.google.Chrome",
					"volume":     rootVolume,
				},
			},
			assertion: assert.NoError,
		},
		{
			name: "valid nonempty results with volume constraint",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"package_id": {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{}},
						"volume":     {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: "testdata"}}},
					},
				},
			},
			execReturnFile: "valid_user_volume.output",
			want: []map[string]string{
				{
					"package_id": "com.example.test.receipt",
					"volume":     "testdata",
				},
			},
			assertion: assert.NoError,
		},
		{
			name: "valid empty results",
			args: args{
				queryContext: table.QueryContext{
					Constraints: map[string]table.ConstraintList{
						"package_id": {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{}},
						"volume":     {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{}},
					},
				},
			},
			execReturnFile: "valid_empty.output",
			want:           []map[string]string{},
			assertion:      assert.NoError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			execReturn, err := os.ReadFile(filepath.Join("testdata", tt.execReturnFile))
			require.NoError(t, err, "read exec return file")

			volumeEqualsExpression := rootVolume
			if volumeConstraints, ok := tt.args.queryContext.Constraints["volume"]; ok {
				if len(volumeConstraints.Constraints) > 0 {
					volumeEqualsExpression = volumeConstraints.Constraints[0].Expression
				}
			}

			executor := mocks.NewExecutor(t)
			executor.On("ExecPackages", volumeEqualsExpression).Return(execReturn, nil).Once()

			got, err := generatePackagesData(t.Context(), tt.args.queryContext, executor, multislogger.NewNopLogger())
			tt.assertion(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGeneratePackageInfoData(t *testing.T) {
	t.Parallel()

	var tests = []struct {
		name           string
		packageID      string
		execReturnFile string
		want           []map[string]string
	}{
		{
			name:           "with groups",
			packageID:      "com.apple.pkg.XProtectPlistConfigData_10_15.16U4437",
			execReturnFile: "valid_pkg_info.output",
			want: []map[string]string{
				{
					"package_id":   "com.apple.pkg.XProtectPlistConfigData_10_15.16U4437",
					"version":      "5351.1783908380",
					"volume":       "/",
					"location":     "/",
					"install_time": "1784293286",
					"groups":       "com.apple.FindSystemFiles.pkg-group",
				},
			},
		},
		{
			name:           "without groups",
			packageID:      "org.golang.go",
			execReturnFile: "valid_pkg_info_no_groups.output",
			want: []map[string]string{
				{
					"package_id":   "org.golang.go",
					"version":      "1.22.0",
					"volume":       "/",
					"location":     "/",
					"install_time": "1700000000",
				},
			},
		},
		{
			name:           "duplicate groups",
			packageID:      "com.example.pkg",
			execReturnFile: "valid_pkg_info_duplicate_groups.output",
			want: []map[string]string{
				{
					"package_id":   "com.example.pkg",
					"groups":       "group.one,group.two",
					"install_time": "1700000000",
				},
			},
		},
		{
			name:           "ignores unmapped keys",
			packageID:      "com.example.pkg",
			execReturnFile: "valid_pkg_info_ignores_extras.output",
			want: []map[string]string{
				{
					"package_id":   "com.example.pkg",
					"install_time": "1700000000",
				},
			},
		},
		{
			name:           "empty output",
			packageID:      "com.example.pkg",
			execReturnFile: "valid_pkg_info_empty.output",
			want:           []map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			execReturn, err := os.ReadFile(filepath.Join("testdata", tt.execReturnFile))
			require.NoError(t, err, "read exec return file")

			executor := mocks.NewExecutor(t)
			executor.On("ExecPackageInfo", rootVolume, tt.packageID).Return(execReturn, nil).Once()

			queryContext := table.QueryContext{
				Constraints: map[string]table.ConstraintList{
					"package_id": {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{{Operator: table.OperatorEquals, Expression: tt.packageID}}},
					"volume":     {Affinity: table.ColumnTypeText, Constraints: []table.Constraint{}},
				},
			}

			got, err := generatePackageInfoData(t.Context(), queryContext, executor, multislogger.NewNopLogger())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
