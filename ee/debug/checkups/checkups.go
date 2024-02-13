// Package checkups contains small debugging funtions. These are designed to run as part of `doctor`, `flare` and
// `logCheckpoint`. They return a general status, and several types of information:
//
//  1. There is the _status_. This is an enum
//  2. There is a _summary_. This is meant to be a short string displayed during doctor and in logs
//  3. There may be a _data_ artifact. This is of type any, and is meant to end up in log checkpoints
//  4. There may be extra data, this is an io stream and is designed to be additional information to package into flare.
//
// The tricky part is that these get generated at different times. The extra data is generated during a checkup, but
// the other pieces happen after completion. This has some implications for how method signatures and data buffering work.
// Namely, it does not make sense to have the checkups comform to interfaces, and let the callers deal. Instead, we define
// a basic checkup interface, and export wrapper functions.
//
// TODO: The way this enumerates checkups in both Doctor and Flare feels awkward. Needs a rethink. Codegen might help?
package checkups

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/version"
	"github.com/kolide/launcher/ee/agent/types"
)

type Status string

const (
	Unknown       Status = "Unknown"
	Erroring      Status = "Error"         // The checkup was unable to run. Equivalent to a protocol error
	Informational Status = "Informational" // Checkup does not have pass/fail status, information only
	Passing       Status = "Passing"       // Checkup is passing
	Warning       Status = "Warning"       // Checkup is warning
	Failing       Status = "Failing"       // Checkup is failing
)

func (s Status) Emoji() string {
	switch s {
	case Informational:
		return " "
	case Passing:
		return "✅"
	case Warning:
		return "⚠️"
	case Failing:
		return "❌"
	case Erroring:
		return "❌"
	default:
		return "? "
	}
}

func writeSummary(w io.Writer, s Status, name, msg string) {
	fmt.Fprintf(w, "%s\t%s: %s\n", s.Emoji(), name, msg)
}

// checkupInt is the generalized checkup interface. It is not meant to be exported.
type checkupInt interface {
	Name() string                                         // Checkup name
	Run(ctx context.Context, extraWriter io.Writer) error // Run the checkup. Errors here are protocol level
	ExtraFileName() string                                // If this checkup will have extra data, what name should it use in flare
	Summary() string                                      // Short summary string about the status
	Status() Status                                       // State of this checkup
	Data() any                                            // What data objects exist, if any
}

type targetBits uint8

const (
	doctorSupported targetBits = 1 << iota
	flareSupported
	logSupported
)

//const checkupFor iota

func checkupsFor(k types.Knapsack, target targetBits) []checkupInt {
	// This encodes what checkups run in which contexts. This could be pushed down into the checkups directly,
	// but it seems nice to have it here. TBD
	var potentialCheckups = []struct {
		c       checkupInt
		targets targetBits
	}{
		{&Platform{}, doctorSupported | flareSupported | logSupported},
		{&Version{k: k}, doctorSupported | flareSupported | logSupported},
		{&hostInfoCheckup{k: k}, doctorSupported | flareSupported | logSupported},
		{&Processes{}, doctorSupported | flareSupported},
		{&RootDirectory{k: k}, doctorSupported | flareSupported},
		{&Connectivity{k: k}, doctorSupported | flareSupported | logSupported},
		{&Logs{k: k}, doctorSupported | flareSupported},
		{&InitLogs{}, flareSupported},
		{&BinaryDirectory{}, doctorSupported | flareSupported},
		{&launchdCheckup{}, doctorSupported | flareSupported},
		{&runtimeCheckup{}, flareSupported},
		{&enrollSecretCheckup{}, doctorSupported | flareSupported},
		{&bboltdbCheckup{k: k}, flareSupported},
		{&networkCheckup{}, doctorSupported | flareSupported},
		{&installCheckup{}, flareSupported},
		{&servicesCheckup{}, doctorSupported | flareSupported},
		{&powerCheckup{}, flareSupported},
		{&osqueryCheckup{k: k}, doctorSupported | flareSupported},
		{&launcherFlags{}, doctorSupported | flareSupported},
		{&gnomeExtensions{}, doctorSupported | flareSupported},
		{&quarantine{}, doctorSupported | flareSupported},
		{&systemTime{}, doctorSupported | flareSupported},
		{&dnsCheckup{k: k}, doctorSupported | flareSupported | logSupported},
		{&tufCheckup{k: k}, doctorSupported | flareSupported},
		{&osqConfigConflictCheckup{}, doctorSupported | flareSupported},
		{&serverDataCheckup{k: k}, doctorSupported | flareSupported | logSupported},
		{&osqDataCollector{k: k}, doctorSupported | flareSupported},
	}

	checkupsToRun := make([]checkupInt, 0)
	for _, p := range potentialCheckups {
		if p.targets&target == 0 {
			continue
		}

		// Use the absence of a name as a shorthand for not supported. This lets is avoid platform
		// flavors of this method
		if p.c.Name() == "" {
			continue
		}

		checkupsToRun = append(checkupsToRun, p.c)
	}

	return checkupsToRun
}

// doctorCheckup runs a checkup for the doctor command line. Its a small bit of sugar over the io channels
func doctorCheckup(ctx context.Context, c checkupInt, w io.Writer) {
	if err := c.Run(ctx, io.Discard); err != nil {
		writeSummary(w, Erroring, c.Name(), fmt.Sprintf("failed to run: %s", err))
		return
	}

	writeSummary(w, c.Status(), c.Name(), c.Summary())
}

type zipFile interface {
	Create(name string) (io.Writer, error)
}

func flareCheckup(ctx context.Context, c checkupInt, combinedSummary io.Writer, flare zipFile) {
	// zip can only have a single open file. So defer writing the summary.
	summary := bytes.Buffer{}
	defer func() {
		// This path is used by the zip writer, thus not filepath
		summaryFlareFH, err := flare.Create(path.Join(c.Name(), "summary.log"))
		if err != nil {
			writeSummary(&summary, Erroring, c.Name(), fmt.Sprintf("error creating flare summary file: %s", err))
			return
		}

		summaryFH := io.MultiWriter(summaryFlareFH, combinedSummary)

		summaryFH.Write(summary.Bytes())
	}()

	fullFH := io.Discard
	if filename := c.ExtraFileName(); filename != "" {
		var err error
		fullFH, err = flare.Create(path.Join(c.Name(), filename))

		if err != nil {
			writeSummary(&summary, Erroring, c.Name(), fmt.Sprintf("error creating flare full file: %s", err))
			return
		}
	}

	if err := c.Run(ctx, fullFH); err != nil {
		writeSummary(&summary, Erroring, c.Name(), fmt.Sprintf("failed to run: %s", err))
		return
	}

	writeSummary(&summary, c.Status(), c.Name(), c.Summary())

	if data := c.Data(); data != nil {
		dataFH, err := flare.Create(path.Join(c.Name(), "data.json"))
		if err != nil {
			writeSummary(&summary, Erroring, c.Name(), fmt.Sprintf("error creating flare data.json file: %s", err))
			return
		}

		if err := json.NewEncoder(dataFH).Encode(data); err != nil {
			writeSummary(&summary, Erroring, c.Name(), fmt.Sprintf("unable to marshal data: %s", err))
			return
		}
	}
}

func logCheckup(ctx context.Context, c checkupInt, logger log.Logger) { // nolint:unused
	if err := c.Run(ctx, io.Discard); err != nil {
		level.Debug(logger).Log(
			"name", c.Name(),
			"msg", "error running checkup",
			"err", err,
			"status", Erroring,
		)
		return
	}

	level.Debug(logger).Log(
		"name", c.Name(),
		"msg", c.Summary(),
		"status", c.Status(),
		"data", c.Data(), // NOTE: on windows, this may serialize poorly. Consider #1246
	)
}

func RunDoctor(ctx context.Context, k types.Knapsack, w io.Writer) {
	failingCheckups := []string{}
	warningCheckups := []string{}

	for _, c := range checkupsFor(k, doctorSupported) {
		ctx, cancel := context.WithTimeout(context.TODO(), 10*time.Second)
		defer cancel()

		doctorCheckup(ctx, c, w)

		switch c.Status() {
		case Warning:
			warningCheckups = append(warningCheckups, c.Name())
		case Failing, Erroring:
			failingCheckups = append(failingCheckups, c.Name())
		}
	}

	// Now print some handy information

	if len(warningCheckups) > 0 {
		fmt.Fprintf(w, "\nCheckups with warnings:\n")
		for _, n := range warningCheckups {
			fmt.Fprintf(w, "\t* %s\n", n)
		}
		fmt.Fprintf(w, "\n")
	}

	if len(failingCheckups) > 0 {
		fmt.Fprintf(w, "\nCheckups with failures:\n")
		for _, n := range failingCheckups {
			fmt.Fprintf(w, "\t* %s\n", n)
		}
		fmt.Fprintf(w, "\n")
	}
}

type runtimeEnvironmentType string

const (
	StandaloneEnviroment runtimeEnvironmentType = "standalone"
	InSituEnvironment    runtimeEnvironmentType = "in situ"
)

func RunFlare(ctx context.Context, k types.Knapsack, flareStream io.WriteCloser, runtimeEnvironment runtimeEnvironmentType) error {
	flare := zip.NewWriter(flareStream)
	combinedSummary := bytes.Buffer{}

	close := func() error {
		closeFlares := func() error {
			return errors.Join(flare.Close(), flareStream.Close())
		}

		// zip can only handle one file being written at a time. So defer writing the summary till the end
		zipSummary, err := flare.Create("doctor.log")
		if err != nil {
			return errors.Join(fmt.Errorf("creating doctor.log: %w", err), closeFlares())
		}

		if _, err := zipSummary.Write(combinedSummary.Bytes()); err != nil {
			return errors.Join(fmt.Errorf("writing doctor.log: %w", err), closeFlares())
		}

		return closeFlares()
	}

	// Note our runtime context.
	writeSummary(&combinedSummary, Informational, "flare", fmt.Sprintf("running %s", runtimeEnvironment))
	if err := writeFlareEnv(flare, runtimeEnvironment); err != nil {
		return errors.Join(fmt.Errorf("writing flare environment: %w", err), close())
	}

	for _, c := range checkupsFor(k, flareSupported) {
		flareCheckup(ctx, c, &combinedSummary, flare)
		if err := flare.Flush(); err != nil {
			return errors.Join(fmt.Errorf("writing flare zip: %w", err), close())
		}
	}

	// we could defer this close, but we want to return any errors
	return close()
}

func writeFlareEnv(z *zip.Writer, runtimeEnvironment runtimeEnvironmentType) error {
	if _, err := z.Create(fmt.Sprintf("FLARE_RUNNING_%s", strings.ReplaceAll(strings.ToUpper(string(runtimeEnvironment)), " ", "_"))); err != nil {
		return fmt.Errorf("making env note file: %s", err)
	}

	flareEnvironment := map[string]any{
		"goos":    runtime.GOOS,
		"goarch":  runtime.GOARCH,
		"mode":    runtimeEnvironment,
		"version": version.Version(),
	}

	flareEnvironmentPlatformSpecifics(flareEnvironment)

	out, err := z.Create("metadata.json")
	if err != nil {
		return fmt.Errorf("making metadata.json: %s", err)
	}

	if err := json.NewEncoder(out).Encode(flareEnvironment); err != nil {
		return fmt.Errorf("marshaling metadata.json: %s", err)
	}

	return nil
}
