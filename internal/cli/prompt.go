package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// stdinIsTerminal reports whether a person could be at the other end of stdin.
// It is a variable so tests can play both parts.
// A character-device check is not enough: /dev/null is one, and it is exactly
// what CI and agent harnesses hand a process for stdin.
var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// approve asks for a yes unless one was already given. It returns false with an
// error when nobody can answer: stdin is not a terminal and holds no reply. An
// answer piped in (`yes | ilk add …`) still counts, as it always has.
func approve(yes bool, prompt string) (bool, error) {
	if yes {
		return true, nil
	}
	return confirm(prompt)
}

func confirm(prompt string) (bool, error) {
	printf("%s [y/N] ", prompt)
	var answer string
	if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil {
		if errors.Is(err, io.EOF) && answer == "" && !stdinIsTerminal() {
			println()
			return false, errConfirmationRequired("re-run with --yes to apply")
		}
		return false, nil
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}
