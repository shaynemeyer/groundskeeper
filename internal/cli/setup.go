package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/ui"
)

// photosAsker asks the iCloud Photos question. Tests replace it.
var photosAsker = askInTerminal

var errNoTerminal = errors.New("this question needs a terminal")

func newSetupCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "setup",
		GroupID: groupSetup,
		Short:   "Answer setup questions again",
		Long: `Setup questions are asked the first time a command needs the answer. Run
one of these to answer again.

Safety: only changes the config file.`,
		Example: `  # Say again whether iCloud Photos is on
  gk setup photos`,
	}
	cmd.AddCommand(newSetupPhotosCmd(global))
	return cmd
}

func newSetupPhotosCmd(global *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "photos",
		Short: "Re-ask whether iCloud Photos is on",
		Long: `Asks whether iCloud Photos is on for the iPhone, and saves the answer as
photosSource in the config file. It decides where duplicate photos are
found and deleted: with iCloud Photos on, in this Mac's Photos library
(deletions sync to the phone and go to Recently Deleted); with it off, on
the phone over USB, where deletions are permanent.

You can also set photosSource to "mac-library" or "device" in the config.

Safety: only changes photosSource in the config file. Nothing is scanned
or deleted.

See also: gk iphone, gk help privacy`,
		Example: `  # Answer the iCloud Photos question again
  gk setup photos`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, _, err := loadConfig(global)
			if err != nil {
				return err
			}
			source, err := photosAsker(cmd.OutOrStdout())
			if errors.Is(err, errNoTerminal) {
				return fmt.Errorf("%w: run gk setup photos in a terminal, or set photosSource in %s", err, configPath(global, home))
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if source == "" {
				fmt.Fprintln(out, "Nothing changed.")
				return nil
			}
			path := configPath(global, home)
			if err := config.SetPhotosSource(path, source); err != nil {
				return fmt.Errorf("saving photosSource to %s: %w", path, err)
			}
			fmt.Fprintf(out, "Saved photosSource %q in %s.\n", source, path)
			return nil
		},
	}
}

// askInTerminal asks the question when stdin and out are both terminals.
func askInTerminal(out io.Writer) (string, error) {
	f, ok := out.(*os.File)
	if !ok || !ui.IsTerminal(f) || !ui.IsTerminal(os.Stdin) {
		return "", errNoTerminal
	}
	return askPhotosSource(os.Stdin, out)
}

// photosChoice is one answer to the iCloud Photos question. Not sure has no
// source: it shows where to check and asks again.
type photosChoice struct{ label, source string }

var photosChoices = []photosChoice{
	{"Yes — clean duplicates in this Mac's Photos library (changes sync to the phone)", iphone.SourceMacLibrary},
	{"No  — clean duplicates on the phone directly over USB", iphone.SourceDevice},
	{"Not sure — show me how to check", ""},
}

const howToCheck = `On the iPhone, open Settings › your name › iCloud › Photos.
If "Sync this iPhone" is on, iCloud Photos is on.`

// photosQuestion asks whether iCloud Photos is on. source holds the answer,
// and stays empty when the question is cancelled.
type photosQuestion struct {
	cursor  int
	showHow bool
	source  string
}

func (q photosQuestion) Init() tea.Cmd { return nil }

func (q photosQuestion) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return q, nil
	}
	switch key.String() {
	case "up", "k":
		q.cursor = max(q.cursor-1, 0)
	case "down", "j":
		q.cursor = min(q.cursor+1, len(photosChoices)-1)
	case "enter":
		q.source = photosChoices[q.cursor].source
		if q.source == "" {
			q.showHow = true
			return q, nil
		}
		return q, tea.Quit
	case "esc", "q", "ctrl+c":
		return q, tea.Quit
	}
	return q, nil
}

func (q photosQuestion) View() tea.View {
	var b strings.Builder
	b.WriteString("Is iCloud Photos turned on for this iPhone?\n")
	for i, c := range photosChoices {
		marker := "    "
		if i == q.cursor {
			marker = "  > "
		}
		b.WriteString(marker + c.label + "\n")
	}
	if q.showHow {
		b.WriteString("\n" + howToCheck + "\n")
	}
	b.WriteString("\nup/down to move, enter to choose, esc to cancel\n")
	return tea.NewView(b.String())
}

// askPhotosSource asks the question on out, reading keys from in. It returns
// the chosen photosSource, or "" if the question was cancelled.
func askPhotosSource(in io.Reader, out io.Writer) (string, error) {
	m, err := tea.NewProgram(photosQuestion{}, tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return "", err
	}
	return m.(photosQuestion).source, nil
}
