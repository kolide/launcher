// Package checkups contains small debugging funtions. These are designed to run as part of `doctor`, `flare` and
// `logCheckpoint`
//
// Finding a common interface is somewhat tricky. There are 4 chunks of data, and different callers use different ones.
//  1. There is the _status_. This is an enum
//  2. There is a _summery_. This is meant to be a short string
//  3. There may be a _data_ artifact. This is of type any, and is meant to end up in log checkpoints
//  4. There may be a _detailed_ log, this is an io stream and is designed to be additional information and is used by flare.
//
// The tricky part is that these get generated at different times. The detailed log is generated during a checkup, but
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
	"fmt"
	"io"
	"path"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/types"
)

type Status string

const (
	Unknown       Status = "Unknown"
	Erroring      Status = "Error"         // The checkup had an error generating data
	Informational Status = "Informational" // Just an FYI
	Passing       Status = "Passing"       // Checkup passes
	Warning       Status = "Warning"       // Checkup is warning
	Failing       Status = "Failing"       // Checkup fails
)

func CharFor(s Status) string {
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

// checkupInt is the generalized checkup interface. It is not meant to be exported.
type checkupInt interface {
	Name() string
	Run(ctx context.Context, fullWriter io.Writer) error
	ExtraFileName() string
	Summary() string
	Status() Status
	Data() any
}

// DoctorCheckup runs a checkup for the doctor command line. Its a small bit of sugar over the io channels
func doctorCheckup(ctx context.Context, c checkupInt, w io.Writer) {
	// TODO: maybe a tab writer?

	if err := c.Run(ctx, io.Discard); err != nil {
		fmt.Fprintf(w, "%s %s: Failed to run: %s\n", CharFor(Erroring), c.Name(), err)
		return
	}

	fmt.Fprintf(w, "%s %s: %s\n", CharFor(c.Status()), c.Name(), c.Summary())
}

type zipFile interface {
	Create(name string) (io.Writer, error)
}

func flareCheckup(ctx context.Context, c checkupInt, combinedSummary io.Writer, flare zipFile) {
	// zip can only have a single open file. So defer writing the summary.
	summary := bytes.Buffer{}
	defer func() {
		summaryFlareFH, err := flare.Create(path.Join(c.Name(), "summary.log"))
		if err != nil {
			fmt.Fprintf(combinedSummary, "%s %s: error creating flare summary file: %s\n", CharFor(Erroring), c.Name(), err)
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
			fmt.Fprintf(&summary, "%s %s: error creating flare full file: %s\n", CharFor(Erroring), c.Name(), err)
			return
		}
	}

	if err := c.Run(ctx, fullFH); err != nil {
		fmt.Fprintf(&summary, "%s %s: Failed to run: %s\n", CharFor(Erroring), c.Name(), err)
		return
	}

	fmt.Fprintf(&summary, "%s %s: %s\n", CharFor(c.Status()), c.Name(), c.Summary())

	if data := c.Data(); data != nil {
		dataFH, err := flare.Create(path.Join(c.Name(), "data.json"))
		if err != nil {
			fmt.Fprintf(&summary, "%s %s: error creating flare data.json file: %s\n", CharFor(Erroring), c.Name(), err)
			return
		}

		if err := json.NewEncoder(dataFH).Encode(data); err != nil {
			fmt.Fprintf(&summary, "%s %s: unable to marshal data: %s\n", CharFor(Erroring), c.Name(), err)
			return
		}
	}
}

func logCheckup(ctx context.Context, c checkupInt, logger log.Logger) {
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
		"data", c.Data(),
	)
}

func RunDoctor(ctx context.Context, k types.Knapsack, w io.Writer) {
	var checkupsToRun = []checkupInt{
		&Processes{},
		&Platform{},
		&Version{k: k},
		&RootDirectory{k: k},
		&Connectivity{k: k},
		&Logs{k: k},
	}

	failingCheckups := []string{}
	warningCheckups := []string{}

	for _, c := range checkupsToRun {
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
		fmt.Fprintf(w, "\nCheckups with warnings: ")
		for _, n := range warningCheckups {
			fmt.Fprintf(w, "\t* %s", n)
		}
	}

	if len(failingCheckups) > 0 {
		fmt.Fprintf(w, "\nCheckups with failures or warnings: ")
		for _, n := range failingCheckups {
			fmt.Fprintf(w, "\t* %s", n)
		}
	}
}

func RunFlare(ctx context.Context, k types.Knapsack, flareStream io.Writer) error {
	var checkupsToRun = []checkupInt{
		&Processes{},
		&Platform{},
		&Version{k: k},
		&RootDirectory{k: k},
		&Connectivity{k: k},
		&Logs{k: k},
	}

	flare := zip.NewWriter(flareStream)
	defer func() {
		_ = flare.Close()
	}()

	// zip can only handle one file being written at a time. So defer writing the summary till the end
	combinedSummary := bytes.Buffer{}
	defer func() {
		zipSummary, err := flare.Create("doctor.log")
		if err != nil {
			// Oh well
			return
		}

		zipSummary.Write(combinedSummary.Bytes())
	}()

	for _, c := range checkupsToRun {
		flareCheckup(ctx, c, &combinedSummary, flare)
		if err := flare.Flush(); err != nil {
			return fmt.Errorf("writing flare file: %w", err)
		}
	}

	return nil
}
