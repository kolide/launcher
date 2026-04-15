package secretscan

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	typesmocks "github.com/kolide/launcher/v2/ee/agent/types/mocks"
	"github.com/kolide/launcher/v2/ee/tables/ci"
	"github.com/kolide/launcher/v2/ee/tables/tablehelpers"
	"github.com/kolide/launcher/v2/pkg/log/multislogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zricethezav/gitleaks/v8/report"
)

func TestSecretScan(t *testing.T) {
	t.Parallel()

	// Extract test data once for all subtests
	projectDir := extractTestData(t)

	for _, tt := range []struct {
		name string
		// Input configuration
		scanType          string                    // "path" or "raw_data"
		targetPath        string                    // relative path within test_data (for file/directory) or raw content
		setupFunc         func(t *testing.T) string // optional setup that returns absolute path to scan
		minFindingsCount  int
		expectedRuleIDs   []string // rule IDs we expect to find (at least one)
		expectedFileNames []string // file names we expect in findings (at least one)
		forbiddenFiles    []string // file names that should NOT appear in findings
	}{
		{
			name:              "scan directory finds multiple secrets",
			scanType:          "path",
			targetPath:        "", // root of test_data
			minFindingsCount:  2,
			expectedRuleIDs:   []string{"slack-bot-token", "github-pat"},
			expectedFileNames: []string{"config.yaml", "github_token.env"},
			forbiddenFiles:    []string{"clean_file.txt", "low_entropy.txt"},
		},
		{
			name:              "scan single file with slack token",
			scanType:          "path",
			targetPath:        "config.yaml",
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"slack-bot-token"},
			expectedFileNames: []string{"config.yaml"},
		},
		{
			name:              "scan subdirectory file with github token",
			scanType:          "path",
			targetPath:        "subdir/github_token.env", // Will be converted via filepath.FromSlash
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"github-pat"},
			expectedFileNames: []string{"github_token.env"},
		},
		{
			name:       "scan clean file finds no secrets",
			scanType:   "path",
			targetPath: "clean_file.txt",
		},
		{
			name:       "scan low entropy AWS example keys finds no secrets",
			scanType:   "path",
			targetPath: "low_entropy.txt", // Contains AWS EXAMPLE keys which are low entropy and should be ignored
		},
		{
			name:     "scan raw data with slack token",
			scanType: "raw_data",
			targetPath: `config:
  slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"
`,
			minFindingsCount: 1,
			expectedRuleIDs:  []string{"slack-bot-token"},
		},
		{
			name:       "scan raw data without secrets",
			scanType:   "raw_data",
			targetPath: "app_name = 'my-app'\nversion = '1.0.0'\n",
		},
		{
			name:       "scan nonexistent file returns empty",
			scanType:   "path",
			targetPath: "nonexistent_file_that_does_not_exist.txt", // Relative path that won't exist
		},
		{
			name:     "symlink to file with secret is scanned",
			scanType: "path",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				secretFile := filepath.Join(tempDir, "secret.txt")
				err := os.WriteFile(secretFile, []byte(`slack_token: xoxb-9876543210-9876543210-zyxwvutsrqponmlk`), 0644)
				require.NoError(t, err)
				symlinkPath := filepath.Join(tempDir, "secret_link")
				err = os.Symlink(secretFile, symlinkPath)
				require.NoError(t, err)
				return symlinkPath
			},
			minFindingsCount:  1,
			expectedRuleIDs:   []string{"slack-bot-token"},
			expectedFileNames: []string{"secret_link"},
		},
		{
			name:     "broken symlink returns empty",
			scanType: "path",
			setupFunc: func(t *testing.T) string {
				tempDir := t.TempDir()
				brokenLink := filepath.Join(tempDir, "broken_link")
				err := os.Symlink(filepath.Join(tempDir, "nonexistent"), brokenLink)
				require.NoError(t, err)
				return brokenLink
			},
			// Broken symlink causes stat error, which is logged and skipped (returns empty)
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tbl := &Table{
				slogger: multislogger.NewNopLogger(),
			}

			var queryContext map[string][]string

			switch tt.scanType {
			case "path":
				var fullPath string
				if tt.setupFunc != nil {
					// Use custom setup function to create test fixtures
					fullPath = tt.setupFunc(t)
				} else if tt.targetPath == "" {
					fullPath = projectDir
				} else if !filepath.IsAbs(tt.targetPath) {
					// Convert forward slashes to OS-specific separator
					fullPath = filepath.Join(projectDir, filepath.FromSlash(tt.targetPath))
				} else {
					fullPath = tt.targetPath
				}
				queryContext = map[string][]string{"path": {fullPath}}
			case "raw_data":
				queryContext = map[string][]string{"raw_data": {tt.targetPath}}
			default:
				t.Fatalf("unknown scanType: %s", tt.scanType)
			}

			results, err := tbl.generate(t.Context(), tablehelpers.MockQueryContext(queryContext))
			require.NoError(t, err)

			// Check findings count
			if tt.minFindingsCount == 0 {
				assert.Empty(t, results, "expected no secrets to be found")
				return
			}

			require.GreaterOrEqual(t, len(results), tt.minFindingsCount,
				"expected at least %d findings, got %d", tt.minFindingsCount, len(results))

			// Collect actual findings
			foundRuleIDs := make(map[string]bool)
			foundFileNames := make(map[string]bool)
			for _, row := range results {
				foundRuleIDs[row["rule_id"]] = true
				foundFileNames[filepath.Base(row["path"])] = true

				// Verify columns are properly populated
				assert.NotEmpty(t, row["rule_id"], "rule_id should be populated")
				assert.NotEmpty(t, row["description"], "description should be populated")
				assert.NotEqual(t, "0", row["line_number"], "line_number should be > 0")

				// For raw_data scans, verify the original input is returned (for SQLite filtering to work)
				if tt.scanType == "raw_data" {
					assert.Equal(t, tt.targetPath, row["raw_data"], "raw_data should contain the original input")
				}
			}

			// Verify expected rule IDs were found
			for _, expectedRule := range tt.expectedRuleIDs {
				assert.True(t, foundRuleIDs[expectedRule],
					"expected to find rule %q, found: %v", expectedRule, foundRuleIDs)
			}

			// Verify expected file names were found
			for _, expectedFile := range tt.expectedFileNames {
				assert.True(t, foundFileNames[expectedFile],
					"expected findings in file %q, found: %v", expectedFile, foundFileNames)
			}

			// Verify forbidden files were not flagged
			for _, forbiddenFile := range tt.forbiddenFiles {
				assert.False(t, foundFileNames[forbiddenFile],
					"file %q should not have any findings", forbiddenFile)
			}
		})
	}
}

func Test_FindingsToNames(t *testing.T) {
	t.Parallel()

	// Extract test data
	projectDir := extractTestData(t)

	mockQC := tablehelpers.MockQueryContext(map[string][]string{
		"path": {filepath.Join(projectDir, "misc.env")},
	})

	tbl := &Table{
		slogger: multislogger.NewNopLogger(),
	}

	results, err := tbl.generate(t.Context(), mockQC)
	require.NoError(t, err)

	// Filter down the results to only the columns we're testing against
	desiredKeys := []string{"name", "rule_id", "line_number", "column_start"}
	resultsFiltered := make([]map[string]string, len(results))

	for i, row := range results {
		resultsFiltered[i] = make(map[string]string, len(desiredKeys))
		for k, v := range row {
			if !slices.Contains(desiredKeys, k) {
				continue
			}
			resultsFiltered[i][k] = v
		}
	}

	// The goal here is to try to ensure that we get the right names back. Notably, that we get two blank
	// names for the row that has two secrets. I've opted to do this by checking each line and column. Future
	// libraries might change that, and require a different testing strategy.
	tests := []map[string]string{
		{"name": "CONFIG_KEY", "rule_id": "generic-api-key", "line_number": "2", "column_start": "2"},
		{"name": "CONFIG_KEY2", "rule_id": "generic-api-key", "line_number": "3", "column_start": "2"},
		{"name": "SPACE_KEY", "rule_id": "generic-api-key", "line_number": "6", "column_start": "2"},
		{"name": "COLON_KEY", "rule_id": "generic-api-key", "line_number": "7", "column_start": "2"},
		{"name": "figma_key", "rule_id": "generic-api-key", "line_number": "17", "column_start": "18"},
		{"name": "BUNDLE_GEMS__CONTRIBSYS__COM", "rule_id": "sidekiq-secret", "line_number": "19", "column_start": "2"},
		{"name": "NPM_TOKEN", "rule_id": "npm-access-token", "line_number": "18", "column_start": "13"},

		// There are two secrets on line 14, but we expect name to be blank
		{"name": "", "rule_id": "generic-api-key", "line_number": "14", "column_start": "2"},
		{"name": "", "rule_id": "generic-api-key", "line_number": "14", "column_start": "152"},
	}

	// Make sure we covered all the test cases
	require.Equal(t, len(tests), len(resultsFiltered))

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			require.Contains(t, resultsFiltered, tt)

		})
	}
}

func TestHashing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		input             string
		argonSalt         string
		expectedArgonHash string
	}{
		{
			name:              "have salt1 expect hash",
			input:             `slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"`,
			argonSalt:         "Hxx5g0dYT4OVzrVc1iskyA==",
			expectedArgonHash: "b22083",
		},
		{
			name:              "have salt2 expect hash2",
			input:             `slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"`,
			argonSalt:         "yg9UwWbxYpxawmjNRTl4Cw==",
			expectedArgonHash: "942613",
		},
		{
			name:              "no salt no hash",
			input:             `slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"`,
			argonSalt:         "",
			expectedArgonHash: "",
		},
		{
			name:              "short salt no hash",
			input:             `slack_bot_token: "xoxb-9876543210-9876543210-zyxwvutsrqponmlk"`,
			argonSalt:         "8vWnXlDI6adqBAFMwkmo",
			expectedArgonHash: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockQC := tablehelpers.MockQueryContext(map[string][]string{
				"raw_data":           {tt.input},
				"hash_argon2id_salt": {tt.argonSalt},
			})

			tbl := &Table{
				slogger: multislogger.NewNopLogger(),
			}

			results, err := tbl.generate(t.Context(), mockQC)
			require.NoError(t, err)
			require.Equal(t, 1, len(results))
			result := results[0]

			require.Contains(t, result["description"], "Slack Bot token")
			require.Equal(t, tt.expectedArgonHash, result["hash_argon2id"])

		})
	}
}

// Helper functions

// extractTestData reads the test zip file from disk and extracts it to a temp directory.
// Returns the path to the extracted test_data directory.
// Uses testing.TB interface to work with both *testing.T and *testing.B.
func extractTestData(tb testing.TB) string {
	tb.Helper()

	tempDir := tb.TempDir()

	// Use filepath.FromSlash for cross-platform compatibility
	zipPath := filepath.FromSlash("test_data/test_data.zip")
	zipReader, err := zip.OpenReader(zipPath)
	require.NoError(tb, err, "opening zip file")
	defer zipReader.Close()

	for _, fileInZip := range zipReader.File {
		// Zip files always use forward slashes - convert to OS separator
		// Also clean the path to remove any potential path traversal
		cleanName := filepath.FromSlash(fileInZip.Name)
		if strings.Contains(cleanName, "..") {
			continue // Skip potentially malicious paths
		}
		filePath := filepath.Join(tempDir, cleanName)

		if fileInZip.FileInfo().IsDir() {
			require.NoError(tb, os.MkdirAll(filePath, 0755), "creating dir")
			continue
		}

		require.NoError(tb, os.MkdirAll(filepath.Dir(filePath), 0755), "creating parent dir")

		fileInZipReader, err := fileInZip.Open()
		require.NoError(tb, err, "opening file in zip")

		// Use 0644 as a safe default permission that works on all platforms
		outFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		require.NoError(tb, err, "opening output file")

		_, err = io.Copy(outFile, fileInZipReader)
		fileInZipReader.Close()
		outFile.Close()
		require.NoError(tb, err, "copying from zip to temp dir")
	}

	return filepath.Join(tempDir, "test_data")
}

// Benchmarks

func BenchmarkSecretScanDirectory(b *testing.B) {
	projectDir := extractTestData(b)

	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	secretScanTable := TablePlugin(mockFlags, multislogger.NewNopLogger())
	require.NotNil(b, secretScanTable)

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		response := secretScanTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("path", projectDir))
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message)
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}

func BenchmarkSecretScanFile(b *testing.B) {
	projectDir := extractTestData(b)
	filePath := filepath.Join(projectDir, "config.yaml")

	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	secretScanTable := TablePlugin(mockFlags, multislogger.NewNopLogger())
	require.NotNil(b, secretScanTable)

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		response := secretScanTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("path", filePath))
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message)
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}

func Test_isEncryptedJWTFamilyValue(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		testCaseName           string
		encryptedJWT           string
		expectedIsEncryptedJWT bool
	}{
		{
			testCaseName:           "encrypted JWK, Appendix C RFC example", // https://datatracker.ietf.org/doc/html/rfc7517#appendix-C
			encryptedJWT:           "eyJhbGciOiJQQkVTMi1IUzI1NitBMTI4S1ciLCJwMnMiOiIyV0NUY0paMVJ2ZF9DSnVKcmlwUTF3IiwicDJjIjo0MDk2LCJlbmMiOiJBMTI4Q0JDLUhTMjU2IiwiY3R5IjoiandrK2pzb24ifQ.TrqXOwuNUfDV9VPTNbyGvEJ9JMjefAVn-TR1uIxR9p6hsRQh9Tk7BA.Ye9j1qs22DmRSAddIh-VnA.AwhB8lxrlKjFn02LGWEqg27H4Tg9fyZAbFv3p5ZicHpj64QyHC44qqlZ3JEmnZTgQowIqZJ13jbyHB8LgePiqUJ1hf6M2HPLgzw8L-mEeQ0jvDUTrE07NtOerBk8bwBQyZ6g0kQ3DEOIglfYxV8-FJvNBYwbqN1Bck6d_i7OtjSHV-8DIrp-3JcRIe05YKy3Oi34Z_GOiAc1EK21B11c_AE11PII_wvvtRiUiG8YofQXakWd1_O98Kap-UgmyWPfreUJ3lJPnbD4Ve95owEfMGLOPflo2MnjaTDCwQokoJ_xplQ2vNPz8iguLcHBoKllyQFJL2mOWBwqhBo9Oj-O800as5mmLsvQMTflIrIEbbTMzHMBZ8EFW9fWwwFu0DWQJGkMNhmBZQ-3lvqTc-M6-gWA6D8PDhONfP2Oib2HGizwG1iEaX8GRyUpfLuljCLIe1DkGOewhKuKkZh04DKNM5Nbugf2atmU9OP0Ldx5peCUtRG1gMVl7Qup5ZXHTjgPDr5b2N731UooCGAUqHdgGhg0JVJ_ObCTdjsH4CF1SJsdUhrXvYx3HJh2Xd7CwJRzU_3Y1GxYU6-s3GFPbirfqqEipJDBTHpcoCmyrwYjYHFgnlqBZRotRrS95g8F95bRXqsaDY7UgQGwBQBwy665d0zpvTasvfXf_c0MWAl-neFaKOW_Px6g4EUDjG1GWSXV9cLStLw_0ovdApDIFLHYHePyagyHjouQUuGiq7BsYwYrwaF06tgB8hV8omLNfMEmDPJaZUzMuHw6tBDwGkzD-tS_ub9hxrpJ4UsOWnt5rGUyoN2N_c1-TQlXxm5oto14MxnoAyBQBpwIEgSH3Y4ZhwKBhHPjSo0cdwuNdYbGPpb-YUvF-2NZzODiQ1OvWQBRHSbPWYz_xbGkgD504LRtqRwCO7CC_CyyURi1sEssPVsMJRX_U4LFEOc82TiDdqjKOjRUfKK5rqLi8nBE9soQ0DSaOoFQZiGrBrqxDsNYiAYAmxxkos-i3nX4qtByVx85sCE5U_0MqG7COxZWMOPEFrDaepUV-cOyrvoUIng8i8ljKBKxETY2BgPegKBYCxsAUcAkKamSCC9AiBxA0UOHyhTqtlvMksO7AEhNC2-YzPyx1FkhMoS4LLe6E_pFsMlmjA6P1NSge9C5G5tETYXGAn6b1xZbHtmwrPScro9LWhVmAaA7_bxYObnFUxgWtK4vzzQBjZJ36UTk4OTB-JvKWgfVWCFsaw5WCHj6Oo4jpO7d2yN7WMfAj2hTEabz9wumQ0TMhBduZ-QON3pYObSy7TSC1vVme0NJrwF_cJRehKTFmdlXGVldPxZCplr7ZQqRQhF8JP-l4mEQVnCaWGn9ONHlemczGOS-A-wwtnmwjIB1V_vgJRf4FdpV-4hUk4-QLpu3-1lWFxrtZKcggq3tWTduRo5_QebQbUUT_VSCgsFcOmyWKoj56lbxthN19hq1XGWbLGfrrR6MWh23vk01zn8FVwi7uFwEnRYSafsnWLa1Z5TpBj9GvAdl2H9NHwzpB5NqHpZNkQ3NMDj13Fn8fzO0JB83Etbm_tnFQfcb13X3bJ15Cz-Ww1MGhvIpGGnMBT_ADp9xSIyAM9dQ1yeVXk-AIgWBUlN5uyWSGyCxp0cJwx7HxM38z0UIeBu-MytL-eqndM7LxytsVzCbjOTSVRmhYEMIzUAnS1gs7uMQAGRdgRIElTJESGMjb_4bZq9s6Ve1LKkSi0_QDsrABaLe55UY0zF4ZSfOV5PMyPtocwV_dcNPlxLgNAD1BFX_Z9kAdMZQW6fAmsfFle0zAoMe4l9pMESH0JB4sJGdCKtQXj1cXNydDYozF7l8H00BV_Er7zd6VtIw0MxwkFCTatsv_R-GsBCH218RgVPsfYhwVuT8R4HarpzsDBufC4r8_c8fc9Z278sQ081jFjOja6L2x0N_ImzFNXU6xwO-Ska-QeuvYZ3X_L31ZOX4Llp-7QSfgDoHnOxFv1Xws-D5mDHD3zxOup2b2TppdKTZb9eW2vxUVviM8OI9atBfPKMGAOv9omA-6vv5IxUH0-lWMiHLQ_g8vnswp-Jav0c4t6URVUzujNOoNd_CBGGVnHiJTCHl88LQxsqLHHIu4Fz-U2SGnlxGTj0-ihit2ELGRv4vO8E1BosTmf0cx3qgG0Pq0eOLBDIHsrdZ_CCAiTc0HVkMbyq1M6qEhM-q5P6y1QCIrwg.0HFmhOzsQ98nNWJjIHkR7A",
			expectedIsEncryptedJWT: true,
		},
		{
			testCaseName:           "JWE, Appendix A.1 example", // https://datatracker.ietf.org/doc/html/rfc7516#appendix-A.1
			encryptedJWT:           "eyJhbGciOiJSU0EtT0FFUCIsImVuYyI6IkEyNTZHQ00ifQ.OKOawDo13gRp2ojaHV7LFpZcgV7T6DVZKTyKOMTYUmKoTCVJRgckCL9kiMT03JGeipsEdY3mx_etLbbWSrFr05kLzcSr4qKAq7YN7e9jwQRb23nfa6c9d-StnImGyFDbSv04uVuxIp5Zms1gNxKKK2Da14B8S4rzVRltdYwam_lDp5XnZAYpQdb76FdIKLaVmqgfwX7XWRxv2322i-vDxRfqNzo_tETKzpVLzfiwQyeyPGLBIO56YJ7eObdv0je81860ppamavo35UgoRdbYaBcoh9QcfylQr66oc6vFWXRcZ_ZT2LawVCWTIy3brGPi6UklfCpIMfIjf7iGdXKHzg.48V1_ALb6US04U3b.5eym8TW_c8SuK0ltJ3rpYIzOeDQz7TALvtu6UG9oMo4vpzs9tX_EFShS8iB7j6jiSdiwkIr3ajwQzaBtQD_A.XFBoMYUZodetZdvTiFvSkQ",
			expectedIsEncryptedJWT: true,
		},
		{
			testCaseName:           "JWE, Appendix A.2 example", // https://datatracker.ietf.org/doc/html/rfc7516#appendix-A.2
			encryptedJWT:           "eyJhbGciOiJSU0ExXzUiLCJlbmMiOiJBMTI4Q0JDLUhTMjU2In0.UGhIOguC7IuEvf_NPVaXsGMoLOmwvc1GyqlIKOK1nN94nHPoltGRhWhw7Zx0-kFm1NJn8LE9XShH59_i8J0PH5ZZyNfGy2xGdULU7sHNF6Gp2vPLgNZ__deLKxGHZ7PcHALUzoOegEI-8E66jX2E4zyJKx-YxzZIItRzC5hlRirb6Y5Cl_p-ko3YvkkysZIFNPccxRU7qve1WYPxqbb2Yw8kZqa2rMWI5ng8OtvzlV7elprCbuPhcCdZ6XDP0_F8rkXds2vE4X-ncOIM8hAYHHi29NX0mcKiRaD0-D-ljQTP-cFPgwCp6X-nZZd9OHBv-B3oWh2TbqmScqXMR4gp_A.AxY8DCtDaGlsbGljb3RoZQ.KDlTtXchhZTGufMYmOYGS4HffxPSUrfmqCHXaI9wOGY.9hH0vgRfYgPnAHOd8stkvw",
			expectedIsEncryptedJWT: true,
		},
		{
			testCaseName:           "JWE, Appendix A.3 example", // https://datatracker.ietf.org/doc/html/rfc7516#appendix-A.3
			encryptedJWT:           "eyJhbGciOiJBMTI4S1ciLCJlbmMiOiJBMTI4Q0JDLUhTMjU2In0.6KB707dM9YTIgHtLvtgWQ8mKwboJW3of9locizkDTHzBC2IlrT1oOQ.AxY8DCtDaGlsbGljb3RoZQ.KDlTtXchhZTGufMYmOYGS4HffxPSUrfmqCHXaI9wOGY.U0m_YmjN04DJvceFICbCVQ",
			expectedIsEncryptedJWT: true,
		},
		{
			testCaseName:           "unencrypted JWT",
			encryptedJWT:           "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwiaWF0IjoxNTE2MjM5MDIyfQ.gog41qgIIHkH2h-83gwRq5-NYOciZ4DgN4ulHFSkh6k",
			expectedIsEncryptedJWT: false,
		},
	} {
		t.Run(tt.testCaseName, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.expectedIsEncryptedJWT, isEncryptedJWTFamilyValue(report.Finding{Secret: tt.encryptedJWT}))
		})
	}
}

func BenchmarkSecretScanRawData(b *testing.B) {
	// Use single-line data to avoid JSON escaping issues in the request builder
	rawData := `slack_bot_token: xoxb-9876543210-9876543210-zyxwvutsrqponmlk`
	mockFlags := typesmocks.NewFlags(b)
	mockFlags.On("TableGenerateTimeout").Return(1 * time.Minute)
	mockFlags.On("RegisterChangeObserver", mock.Anything, mock.Anything).Return()

	secretScanTable := TablePlugin(mockFlags, multislogger.NewNopLogger())
	require.NotNil(b, secretScanTable)

	baselineStats := ci.BaselineStats(b)
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		response := secretScanTable.Call(b.Context(), ci.BuildRequestWithSingleEqualConstraint("raw_data", rawData))
		require.Equal(b, int32(0), response.Status.Code, response.Status.Message)
	}

	ci.ReportNonGolangMemoryUsage(b, baselineStats)
}
