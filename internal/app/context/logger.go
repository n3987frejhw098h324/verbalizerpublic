package context

import (
	"bytes"
	"fmt"
	"io"

	"github.com/n3987frejhw098h324/verbalizerpublic/internal/app/config"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/color"
	"github.com/n3987frejhw098h324/verbalizerpublic/internal/console"
)

type logger struct {
	History  *bytes.Buffer
	writer   io.Writer
	progress *console.Progress
	verbose  bool
}

func newLogger(progress *console.Progress) *logger {
	var b bytes.Buffer
	w := io.MultiWriter(&b, progress)

	return &logger{
		History:  &b,
		writer:   w,
		progress: progress,
		verbose:  config.GetBool("verbose"),
	}
}

func (l *logger) Error(a ...any) {
	color.Error.Fprintln(l.writer, a...)
}

func (l *logger) Info(a ...any) {
	color.Info.Fprintln(l.writer, a...)
}

func (l *logger) Println(a ...any) {
	fmt.Fprintln(l.writer, a...)
}

func (l *logger) Success(a ...any) {
	color.Success.Fprintln(l.writer, a...)
}

func (l *logger) Warn(a ...any) {
	color.Warn.Fprintln(l.writer, a...)
}

func (l *logger) Verbose(a ...any) {
	if !l.verbose {
		return
	}
	color.Verbose.Fprintln(l.writer, a...)
}
