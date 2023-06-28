package checkups

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

var (
	whiteText = color.New(color.FgWhite, color.BgBlack)

	// Indented output for checkup results
	info = func(w io.Writer, a ...interface{}) {
		whiteText.FprintlnFunc()(w, fmt.Sprintf("\t%s", a...))
	}
	fail = func(w io.Writer, a ...interface{}) {
		whiteText.FprintlnFunc()(w, fmt.Sprintf("❌\t%s", a...))
	}
	pass = func(w io.Writer, a ...interface{}) {
		whiteText.FprintlnFunc()(w, fmt.Sprintf("✅\t%s", a...))
	}
)
