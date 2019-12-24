package restart

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// ErrMalformedArgs is returned when executable args are malformed.
	ErrMalformedArgs = errors.New("malformed args")
	// ErrAlreadyRestarting is returned on restarting attempt when restarting is in progress.
	ErrAlreadyRestarting = errors.New("already restarting")
)

// DefaultCheckDelay is a default delay for checking if a new instance is started successfully.
const DefaultCheckDelay = 5 * time.Second

// Context describes data required for restarting visor.
type Context struct {
	log              logrus.FieldLogger
	checkDelay       time.Duration
	workingDirectory string
	args             []string
	needsExit        bool // disable in (c *Context) Restart() tests
	isRestarting     int32
}

// CaptureContext captures data required for restarting visor.
// Data used by CaptureContext must not be modified before,
// therefore calling CaptureContext immediately after starting executable is recommended.
func CaptureContext() (*Context, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	args := os.Args

	context := &Context{
		checkDelay:       DefaultCheckDelay,
		workingDirectory: wd,
		args:             args,
		needsExit:        true,
	}

	return context, nil
}

// RegisterLogger registers a logger instead of standard one.
func (c *Context) RegisterLogger(logger logrus.FieldLogger) {
	if c != nil {
		c.log = logger
	}
}

// SetCheckDelay sets a check delay instead of standard one.
func (c *Context) SetCheckDelay(delay time.Duration) {
	if c != nil {
		c.checkDelay = delay
	}
}

// Restart restarts executable using Context.
func (c *Context) Restart() error {
	if !atomic.CompareAndSwapInt32(&c.isRestarting, 0, 1) {
		return ErrAlreadyRestarting
	}

	defer atomic.StoreInt32(&c.isRestarting, 0)

	if len(c.args) == 0 {
		return ErrMalformedArgs
	}

	executableRelPath := c.args[0]
	executableAbsPath := filepath.Join(c.workingDirectory, executableRelPath)

	c.infoLogger()("Starting new instance of executable (path: %q)", executableAbsPath)

	errCh := c.start(executableAbsPath)

	ticker := time.NewTicker(c.checkDelay)
	defer ticker.Stop()

	select {
	case err := <-errCh:
		c.errorLogger()("Failed to start new instance: %v", err)
		return err
	case <-ticker.C:
		c.infoLogger()("New instance started successfully, exiting")

		if c.needsExit {
			os.Exit(0)
		}

		// unreachable unless run in tests
		return nil
	}
}

func (c *Context) start(path string) chan error {
	errCh := make(chan error, 1)

	go func(path string) {
		defer close(errCh)

		normalizedPath, err := exec.LookPath(path)
		if err != nil {
			errCh <- err
			return
		}

		if len(c.args) == 0 {
			errCh <- ErrMalformedArgs
			return
		}

		args := c.args[1:]
		cmd := exec.Command(normalizedPath, args...) // nolint:gosec

		if err := cmd.Start(); err != nil {
			errCh <- err
			return
		}

		if err := cmd.Wait(); err != nil {
			errCh <- err
			return
		}
	}(path)

	return errCh
}

func (c *Context) infoLogger() func(string, ...interface{}) {
	if c.log != nil {
		return c.log.Infof
	}

	logger := log.New(os.Stdout, "[INFO] ", log.LstdFlags)

	return logger.Printf
}

func (c *Context) errorLogger() func(string, ...interface{}) {
	if c.log != nil {
		return c.log.Errorf
	}

	logger := log.New(os.Stdout, "[ERROR] ", log.LstdFlags)

	return logger.Printf
}
