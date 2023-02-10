package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/kit/logutil"
	"github.com/serenize/snaker"
)

// icoSizes are the ico sizes we generate. It's a tradeoff between size and quality.
var icoSizes = []string{
	"16",
	"32",
	"64",
	"128",
}

// Various globals that we use for processing. Globals are kinda :< but is a
// simple generator, and it's not worth _that_ much fussing.
var (
	tmpDir        string
	inDir         string
	outDir        string
	embeddedFiles []string
)

func main() {
	logger := logutil.NewCLILogger(true)

	flagset := flag.NewFlagSet("generate", flag.ExitOnError)
	var (
		flDebug  = flagset.Bool("debug", false, "use a debug logger")
		flOutdir = flagset.String("out", ".", "output directory")
		flIndir  = flagset.String("in", "source/", "input directory")
	)
	if err := flagset.Parse(os.Args[1:]); err != nil {
		level.Error(logger).Log("msg", "error parsing flags", "err", err)
		os.Exit(1)
	}

	// relevel with the debug flag
	logger = logutil.NewCLILogger(*flDebug)

	missingOpt := false
	for f, val := range map[string]string{
		"out": *flOutdir,
		"in":  *flIndir,
	} {
		if val == "" {
			level.Error(logger).Log("msg", "Missing required flag", "flag", f)
			missingOpt = true
		}
	}
	if missingOpt {
		os.Exit(1)
	}

	inDir = *flIndir
	outDir = *flOutdir

	// Find input icon names
	iconNames := make(map[string]bool)
	files, err := filepath.Glob(inDir + "/*")
	if err != nil {
		level.Error(logger).Log("msg", "error globbing input files", "error", err)
		os.Exit(1)
	}
	for _, file := range files {
		file = filepath.Base(file)
		file = strings.TrimSuffix(file, ".png")
		file = strings.TrimSuffix(file, ".svg")
		iconNames[file] = true
	}

	if dir, err := os.MkdirTemp("", "icon-generator"); err != nil {
		level.Error(logger).Log("msg", "error making tmpdir", "err", err)
		os.Exit(1)
	} else {
		tmpDir = dir
	}

	ctx := context.Background()

	for name, _ := range iconNames {
		if err := generateIco(ctx, logger, name); err != nil {
			level.Error(logger).Log(
				"msg", "error generating ico",
				"name", name,
				"err", err)
			os.Exit(1)
		}

		if err := generatePng(ctx, logger, name); err != nil {
			level.Error(logger).Log(
				"msg", "error generating png",
				"name", name,
				"err", err)
			os.Exit(1)
		}
	}

	if err := generateAssetGo(ctx, logger); err != nil {
		level.Error(logger).Log("msg", "error expanding template", "error", err)
		os.Exit(1)
	}

}

func generateAssetGo(ctx context.Context, logger log.Logger) error {
	output, err := os.Create(fmt.Sprintf("%s/assets.go", outDir))
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer output.Close()

	embeds := make(map[string]string, len(embeddedFiles))
	for _, filename := range embeddedFiles {
		// embeds don't support a directory path. Everything is just in the outDir.
		filename = filepath.Base(filename)
		embeds[constName(filename)] = filename
	}

	tmpl, err := template.ParseFiles("generator/assets.go.tmpl")
	if err != nil {
		return fmt.Errorf("loading template: %w", err)
	}

	return tmpl.Execute(output, embeds)
}

// generatePng generates the png file.
func generatePng(ctx context.Context, logger log.Logger, name string) error {
	input := fmt.Sprintf("%s/%s.png", inDir, name)
	output := fmt.Sprintf("%s/%s.png", outDir, name)

	// append output. If we throw an error, we're going to ignore it anyhow.
	embeddedFiles = append(embeddedFiles, output)

	if skip, err := skipFile(input, output); err != nil {
		return err
	} else if skip {
		level.Debug(logger).Log("msg", "skipping", "input", input, "output", output)
		return nil
	}

	// FIXME Does this need scaling?
	cmd := exec.CommandContext(ctx, "cp", input, output)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	return nil
}

// generateIco generates an ico file from source. It uses imagemagick's convert, and implements the
// timestamp behavior from make.
func generateIco(ctx context.Context, logger log.Logger, name string) error {
	input := fmt.Sprintf("%s/%s.svg", inDir, name)
	output := fmt.Sprintf("%s/%s.ico", outDir, name)

	// append output. If we throw an error, we're going to ignore it anyhow.
	embeddedFiles = append(embeddedFiles, output)

	if skip, err := skipFile(input, output); err != nil {
		return err
	} else if skip {
		level.Debug(logger).Log("msg", "skipping", "input", input, "output", output)
		return nil
	}

	// First, we need to generate all the sizes
	for _, size := range icoSizes {
		cmd := exec.CommandContext(
			ctx,
			"convert",
			"-resize", fmt.Sprintf("%sx%s", size, size),
			input,
			fmt.Sprintf("%s/%s-%s.ico", tmpDir, name, size),
		)
		level.Debug(logger).Log("msg", "Resizing with", "cmd", cmd.String())
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("generating ico for size %s: %w", size, err)
		}
	}

	// Now that we have the intermediary sizes, we can stich them into a single ico
	cmd := exec.CommandContext(ctx, "convert", fmt.Sprintf("%s/%s-*.ico", tmpDir, name), output)
	level.Debug(logger).Log("msg", "Consolodating ico with", "cmd", cmd.String())

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stiching ico subfiles together: %w", err)
	}

	return nil
}

func constName(name string) string {
	r := strings.NewReplacer(
		"-", "_",
		" ", "_",
		".", "_",
		"/", "_",
		"\\", "_",
	)

	return snaker.SnakeToCamel(r.Replace(name))
}

// skipFile implements timestamp logic akin to how Make does. This allows us to skip processing if the file
// is unchanged. An error is returned for unexpected cases.
func skipFile(input, output string) (bool, error) {
	inputInfo, err := os.Stat(input)
	if err != nil {
		return false, fmt.Errorf("statting input: %w", err)
	}

	outputInfo, err := os.Stat(output)
	if err != nil {
		// If the file doesn't exist, it cannot be newer
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("statting output: %w", err)
	}

	return inputInfo.ModTime().Before(outputInfo.ModTime()), nil
}
