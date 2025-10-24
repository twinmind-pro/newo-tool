package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// App coordinates CLI command registration and execution.
type App struct {
	commands map[string]Command
	stdout   io.Writer
	stderr   io.Writer
}

// New creates a new CLI application, pre-registering the built-in commands.
func New(stdout, stderr io.Writer) *App {
	app := &App{
		commands: make(map[string]Command),
		stdout:   stdout,
		stderr:   stderr,
	}

	app.Register(&HelpCommand{app: app})
	app.Register(&VersionCommand{writer: stdout})
	app.Register(NewPullCommand(stdout, stderr))
	app.Register(NewPushCommand(stdout, stderr))
	app.Register(NewStatusCommand(stdout, stderr))
	app.Register(NewLintCommand(stdout, stderr))
	app.Register(NewFmtCommand(stdout, stderr))
	app.Register(NewGenerateCommand(stdout, stderr))
	app.Register(NewHealthcheckCommand(stdout, stderr))
	app.Register(NewMergeCommand(stdout, stderr))
	app.Register(NewDeployCommand(stdout, stderr))

	return app
}

// Register adds a command to the application. Duplicate names result in panic.
func (a *App) Register(cmd Command) {
	if _, exists := a.commands[cmd.Name()]; exists {
		panic(fmt.Sprintf("cli: command %q already registered", cmd.Name()))
	}
	a.commands[cmd.Name()] = cmd
}

// Execute runs the command specified by args, defaulting to help.
func (a *App) Execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return nil
	}

	target, ok := a.commands[args[0]]
	if !ok {
		a.printUnknownCommand(args[0])
		return fmt.Errorf("unknown command: %s", args[0])
	}

	fs := flag.NewFlagSet(target.Name(), flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	target.RegisterFlags(fs)

	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			a.printCommandUsage(target, fs)
			return nil
		}
		return err
	}

	return target.Run(ctx, fs.Args())
}

func (a *App) printUsage() {
	_, _ = fmt.Fprintf(a.stderr, "Usage:\n")
	_, _ = fmt.Fprintf(a.stderr, "  %s <command> [flags]\n\n", executableName())
	_, _ = fmt.Fprintf(a.stderr, "Available commands:\n")

	names := make([]string, 0, len(a.commands))
	for name := range a.commands {
		if name == "help" {
			// help is implicit; show it last.
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		cmd := a.commands[name]
		_, _ = fmt.Fprintf(a.stderr, "  %-10s %s\n", cmd.Name(), cmd.Summary())
	}

	// Ensure help stays at the bottom.
	if helpCmd, exists := a.commands["help"]; exists {
		_, _ = fmt.Fprintf(a.stderr, "  %-10s %s\n", helpCmd.Name(), helpCmd.Summary())
	}
}

func (a *App) printUnknownCommand(name string) {
	_, _ = fmt.Fprintf(a.stderr, "Unknown command %q\n\n", name)
	a.printUsage()
}

func (a *App) printCommandUsage(cmd Command, fs *flag.FlagSet) {
	_, _ = fmt.Fprintf(a.stderr, "Usage: %s %s [flags]\n\n", executableName(), cmd.Name())
	if summary := cmd.Summary(); summary != "" {
		_, _ = fmt.Fprintf(a.stderr, "%s\n\n", summary)
	}
	fs.PrintDefaults()
}

func executableName() string {
	name := os.Args[0]
	if name == "" {
		return "newo"
	}
	return filepath.Base(name)
}
