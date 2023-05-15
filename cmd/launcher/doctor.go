package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kolide/kit/version"

	"github.com/fatih/color"
	"golang.org/x/exp/slices"
)

var (
	cRunning  = color.New(color.FgCyan)
	cHeader   = color.New(color.FgHiWhite).Add(color.Bold)
	cProperty = color.New(color.FgWhite)
	cPass     = color.New(color.FgGreen)
	cFail     = color.New(color.FgRed).Add(color.Bold)
	cWarning  = color.New(color.BgHiYellow)

	// Create a custom print function for convenience
	pass = color.New(color.FgGreen).PrintlnFunc()
	warn = color.New(color.FgHiYellow).PrintlnFunc()
	fail = color.New(color.FgRed).Add(color.Bold).PrintlnFunc()

	cInfo = color.New(color.FgWhite)
	info  = func(a ...interface{}) {
		cInfo.Println(fmt.Sprintf("    %s", a...))
	}
)

type checkup struct {
	name   string
	check  func() (string, error)
	checks []func() (string, error)
}

func runDoctor(args []string) error {
	flagset := flag.NewFlagSet("launcher doctor", flag.ExitOnError)
	var (
	// configFile = env.String("config", "/var/kolide-k2/k2device-preprod.kolide.com/launcher.flags", "")

	// not documented via flags on purpose
	// configFile = flagset.String("config", "", "config file to parse options from (optional)")

	// configFile = env.String("config", "/etc/kolide-k2/launcher.flags")
	// etcDir  = env.String("KOLIDE_LAUNCHER_ETC_DIRECTORY", "/etc/kolide-k2/")
	// rootDir = env.String("KOLIDE_LAUNCHER_ROOT_DIRECTORY", "/var/kolide-k2/k2device-preprod.kolide.com/")

	// flControlRequestInterval = flagset.Duration("control_request_interval", 60*time.Second, "The interval at which the control server requests will be made")
	// flKolideServerURL        = flagset.String("hostname", "", "The hostname of the gRPC server")
	)
	flagset.Usage = commandUsage(flagset, "launcher doctor")
	if err := flagset.Parse(args); err != nil {
		return err
	}

	cRunning.Println("Running Kolide launcher checkups...")

	checkups := []*checkup{
		{
			name: "Check platform",
			check: func() (string, error) {
				return checkupPlatform(runtime.GOOS)
			},
		},
		{
			name: "Directory contents",
			check: func() (string, error) {
				return checkupRootDir(getFilepaths("/var/kolide-k2/k2device-preprod.kolide.com/", "*"))
			},
			// checks: []func() (string, error){
			// 	func() (string, error) {
			// 		entries, err := os.ReadDir(rootDir)
			// 		if err != nil {
			// 			fmt.Println(err)
			// 		}

			// 		for _, e := range entries {
			// 			fmt.Println(e.Name())
			// 		}
			// 		return "", nil
			// 	},
			// 	func() (string, error) {
			// 		entries, err := os.ReadDir(etcDir)
			// 		if err != nil {
			// 			fmt.Println(err)
			// 		}

			// 		for _, e := range entries {
			// 			fmt.Println(e.Name())
			// 		}
			// 		return "", nil
			// 	},
			// },
		},
		{
			name: "Check communication with Kolide",
			check: func() (string, error) {
				return "Successfully communicated with Kolide", nil
			},
		},
		{
			name: "Check version",
			check: func() (string, error) {
				return checkupVersion(version.Version())
			},
		},
		{
			name: "Check config file",
			check: func() (string, error) {
				// "/var/kolide-k2/k2device-preprod.kolide.com/launcher.flags"
				return checkupConfigFile(getFilepaths("/etc/kolide-k2/", "launcher.flags"))
			},

			// checks: []func() (string, error){
			// 	func() (string, error) {
			// 		data, err := os.ReadFile(*configFile)
			// 		if data != nil {
			// 			fmt.Println(string(data))
			// 		}
			// 		if err != nil {
			// 			return "", fmt.Errorf("No config file found")
			// 		}
			// 		return "Config file found", nil
			// 	},
			// },
		},
		{
			name: "Check logs",
			check: func() (string, error) {
				// "/var/kolide-k2/k2device-preprod.kolide.com/launcher.flags"
				// /var/kolide-k2/k2device-preprod.kolide.com/launcher
				return checkupLogFiles(getFilepaths("/var/kolide-k2/k2device-preprod.kolide.com/", "debug*"))
			},

			checks: []func() (string, error){
				func() (string, error) {
					// cPass.Println("Kolide")
					return "", nil
				},
			},
		},
	}

	failedCheckups := []*checkup{}

	for _, c := range checkups {
		c.run()
	}

	if len(failedCheckups) > 0 {
		fail("Some checkups failed:")

		for _, c := range failedCheckups {
			fail("%s\n", c.name)
		}
	} else {
		pass("All checkups passed! Your Kolide launcher is healthy.")
	}

	return nil
}

func (c *checkup) run() {
	cRunning.Printf("Running checkup: ")
	cHeader.Printf("%s\n", c.name)

	var err error
	if c.check != nil {
		result, err := c.check()
		if err != nil {
			cProperty.Println(result)
			cProperty.Printf("❌  %s\n", err)
		} else {
			cProperty.Printf("✅  %s\n", result)
		}
	}

	if err != nil {
		fail("𐄂 Checkup failed!")
	} else {
		pass("✔ Checkup passed!")
	}
	pass("")
}

func checkupPlatform(os string) (string, error) {
	if slices.Contains([]string{"windows", "darwin", "linux"}, os) {
		return fmt.Sprintf("Platform: %s", os), nil
	}
	return "", fmt.Errorf("Unsupported platform: %s", os)
}

func checkupRootDir(filepaths []string) (string, error) {
	if filepaths != nil && len(filepaths) > 0 {
		for _, fp := range filepaths {
			// fmt.Println(filepath.Base(fp))
			info(filepath.Base(fp))
		}
		// data, _ := os.ReadFile(filepaths[0])
		// if data != nil {
		// fmt.Println(string(data))
		return "Root directory files found", nil
		// }
	}

	return "", fmt.Errorf("No root directory files found")
}

func checkupVersion(v version.Info) (string, error) {
	warn("Latest available version: Unknown")
	return fmt.Sprintf("Current version: %s\n", v.Version), nil
}

func checkupConfigFile(filepaths []string) (string, error) {
	if filepaths != nil && len(filepaths) > 0 {
		// data, _ := os.ReadFile(filepaths[0])
		// if data != nil {
		// 	// info(string(data))
		// 	return "Config file found", nil
		// }

		file, err := os.Open(filepaths[0])
		if err != nil {
			return "", fmt.Errorf("No config file found")
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			info(scanner.Text())
		}

		return "Config file found", nil
	}

	return "", fmt.Errorf("No config file found")
}

func checkupLogFiles(filepaths []string) (string, error) {
	var foundCurrentLogFile bool
	for _, f := range filepaths {
		filename := filepath.Base(f)
		// fmt.Println(filename)
		info(filename)
		// pass("%s found", filename)
		if filename == "debug.json" {
			// pass("%s found")
			foundCurrentLogFile = true
		}
		// fmt.Println(filepath.Base(fp))
	}
	// if filepaths != nil && len(filepaths) > 0 {
	// 	// data, _ := os.ReadFile(filepaths[0])
	// 	// if data != nil {
	// 	// 	fmt.Println(string(data))
	// 	// 	return "Log files found", nil
	// 	// }
	// }

	if foundCurrentLogFile {
		return "Log file found", nil
	}
	return "", fmt.Errorf("No log file found")
}

func getFilepaths(elem ...string) []string {
	fileGlob := filepath.Join(elem...)
	filepaths, err := filepath.Glob(fileGlob)

	if err == nil && len(filepaths) > 0 {
		return filepaths
	}
	// entries, err := os.ReadDir(dir)
	// if err != nil {
	// 	fmt.Println(err)
	// }

	// for _, e := range entries {
	// 	fmt.Println(e.Name())
	// }

	return nil
}
