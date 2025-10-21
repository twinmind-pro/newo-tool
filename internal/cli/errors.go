package cli

type exitError struct {
	msg    string
	code   int
	silent bool
}

func (e exitError) Error() string {
	return e.msg
}

func (e exitError) ExitCode() int {
	if e.code == 0 {
		return 1
	}
	return e.code
}

func (e exitError) Silent() bool {
	return e.silent
}

func newSilentExitError(code int) error {
	return exitError{code: code, silent: true}
}
