package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/doctor"
	"github.com/shaynemeyer/groundskeeper/internal/fixtures"
)

const guidePath = "../../docs/user-guide.md"

func readGuide(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// guideCommands returns the arguments of every gk command line in the
// guide's code blocks, without the leading gk and any trailing comment.
func guideCommands(guide string) [][]string {
	var cmds [][]string
	inBlock := false
	for _, line := range strings.Split(guide, "\n") {
		if strings.HasPrefix(line, "```") {
			inBlock = !inBlock
			continue
		}
		line = strings.TrimSpace(line)
		if !inBlock || !strings.HasPrefix(line, "gk ") {
			continue
		}
		line, _, _ = strings.Cut(line, " #")
		cmds = append(cmds, strings.Fields(line)[1:])
	}
	return cmds
}

// guideRoot is the command tree as Fang runs it: with the help command and
// the --version flag Fang adds.
func guideRoot() *cobra.Command {
	root := NewRoot()
	root.Version = "test"
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	return root
}

func TestGuideCommandsParse(t *testing.T) {
	cmds := guideCommands(readGuide(t))
	if len(cmds) == 0 {
		t.Fatal("the guide has no gk commands in code blocks")
	}
	for _, args := range cmds {
		root := guideRoot()
		cmd, rest, err := root.Find(args)
		if err != nil {
			t.Errorf("gk %s: %v", strings.Join(args, " "), err)
			continue
		}
		if cmd.Name() == "help" {
			if _, _, err := root.Find(rest); err != nil {
				t.Errorf("gk %s: %v", strings.Join(args, " "), err)
			}
			continue
		}
		cmd.InitDefaultHelpFlag()
		cmd.InitDefaultVersionFlag()
		if err := cmd.ParseFlags(rest); err != nil {
			t.Errorf("gk %s: %v", strings.Join(args, " "), err)
		}
	}
}

func TestGuideMentionsEveryCommand(t *testing.T) {
	guide := readGuide(t)
	var check func(*cobra.Command)
	check = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			mention := "gk " + strings.TrimPrefix(sub.CommandPath(), "groundskeeper ")
			if !sub.Runnable() && !sub.HasSubCommands() {
				mention = "gk help " + sub.Name()
			}
			if !strings.Contains(guide, mention) {
				t.Errorf("the guide never mentions %q", mention)
			}
			check(sub)
		}
	}
	check(NewRoot())
}

func TestGuideDocumentsEveryConfigKey(t *testing.T) {
	_, section, found := strings.Cut(readGuide(t), "\n## Configuration\n")
	if !found {
		t.Fatal("the guide has no Configuration section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	fields := reflect.TypeFor[config.Config]()
	for i := range fields.NumField() {
		key, _, _ := strings.Cut(fields.Field(i).Tag.Get("json"), ",")
		if !strings.Contains(section, "`"+key+"`") {
			t.Errorf("the Configuration section never mentions `%s`", key)
		}
	}
}

func TestGuideHasNoPersonalData(t *testing.T) {
	// The LaunchAgent's label is Groundskeeper's own, not personal data.
	guide := strings.ReplaceAll(readGuide(t), doctor.Label, "")
	if found := fixtures.Check([]byte(guide)); len(found) > 0 {
		t.Errorf("the guide looks like it holds personal data: %v", found)
	}
	for _, home := range []string{"/Users/", "/home/"} {
		if strings.Contains(guide, home) {
			t.Errorf("the guide holds a home path (%s...); write ~ instead", home)
		}
	}
}
