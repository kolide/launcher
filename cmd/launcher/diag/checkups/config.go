package checkups

import (
	"fmt"
	"io"
	"os"

	"github.com/peterbourgon/ff/v3"
)

type Config struct {
	Filepath string
}

func (c *Config) Name() string {
	return "Check config file"
}

func (c *Config) Run(short io.Writer) (string, error) {
	return checkupConfigFile(short, c.Filepath)
}

// checkupConfigFile tests that the config file is valid and logs it's contents
func checkupConfigFile(short io.Writer, filepath string) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", fmt.Errorf("No config file found")
	}
	defer file.Close()

	// Parse the config file how launcher would
	err = ff.PlainParser(file, func(name, value string) error {
		info(short, fmt.Sprintf("%s\t%s", name, value))
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("Invalid config file")
	}
	return "Config file found", nil
}
