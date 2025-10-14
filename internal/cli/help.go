package cli

import (
	"context"
	"flag"
	"fmt"
)

// HelpCommand prints usage information for commands.
type HelpCommand struct {
	app *App
}

func (c *HelpCommand) Name() string {
	return "help"
}

func (c *HelpCommand) Summary() string {
	return "Show usage information for a command"
}

func (c *HelpCommand) RegisterFlags(_ *flag.FlagSet) {}

func (c *HelpCommand) Run(_ context.Context, args []string) error {
	if len(args) == 0 {
		c.app.printUsage()
		return nil
	}

	target, ok := c.app.commands[args[0]]
	if !ok {
		c.app.printUnknownCommand(args[0])
		return fmt.Errorf("unknown command: %s", args[0])
	}

	fs := flag.NewFlagSet(target.Name(), flag.ContinueOnError)
	fs.SetOutput(c.app.stderr)
	target.RegisterFlags(fs)
	c.app.printCommandUsage(target, fs)
	return nil
}
