package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/shaynemeyer/groundskeeper/internal/config"
	"github.com/shaynemeyer/groundskeeper/internal/iphone"
)

func press(q photosQuestion, keys ...tea.KeyPressMsg) (photosQuestion, bool) {
	quit := false
	for _, k := range keys {
		m, cmd := q.Update(k)
		q = m.(photosQuestion)
		quit = cmd != nil
	}
	return q, quit
}

var (
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyEsc   = tea.KeyPressMsg{Code: tea.KeyEscape}
)

func TestPhotosQuestionAnswers(t *testing.T) {
	cases := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{"yes", []tea.KeyPressMsg{keyEnter}, iphone.SourceMacLibrary},
		{"no", []tea.KeyPressMsg{keyDown, keyEnter}, iphone.SourceDevice},
		{"stops at the top", []tea.KeyPressMsg{keyUp, keyEnter}, iphone.SourceMacLibrary},
		{"stops at the bottom", []tea.KeyPressMsg{keyDown, keyDown, keyDown, keyUp, keyEnter}, iphone.SourceDevice},
	}
	for _, c := range cases {
		q, quit := press(photosQuestion{}, c.keys...)
		if q.source != c.want || !quit {
			t.Errorf("%s: got %q, quit %v; want %q", c.name, q.source, quit, c.want)
		}
	}
}

func TestPhotosQuestionNotSureAsksAgain(t *testing.T) {
	q, quit := press(photosQuestion{}, keyDown, keyDown, keyEnter)
	if quit || q.source != "" || !q.showHow {
		t.Fatalf("got %+v, quit %v", q, quit)
	}
	if view := q.View().Content; !strings.Contains(view, "Sync this iPhone") {
		t.Errorf("view doesn't say where to check:\n%s", view)
	}
	if q, quit = press(q, keyUp, keyEnter); q.source != iphone.SourceDevice || !quit {
		t.Errorf("after not sure: got %q, quit %v", q.source, quit)
	}
}

func TestPhotosQuestionCancel(t *testing.T) {
	q, quit := press(photosQuestion{}, keyDown, keyEsc)
	if q.source != "" || !quit {
		t.Errorf("got %q, quit %v; want cancelled", q.source, quit)
	}
}

func TestAskPhotosSourceReadsKeys(t *testing.T) {
	// "j" moves down and "\r" is enter, as a terminal sends them.
	var out bytes.Buffer
	got, err := askPhotosSource(strings.NewReader("j\r"), &out)
	if err != nil {
		t.Fatal(err)
	}
	if got != iphone.SourceDevice {
		t.Errorf("got %q, want %q", got, iphone.SourceDevice)
	}
}

func answer(t *testing.T, source string) {
	t.Helper()
	photosAsker = func(io.Writer) (string, error) { return source, nil }
	t.Cleanup(func() { photosAsker = askInTerminal })
}

func TestSetupPhotosSavesAnswer(t *testing.T) {
	home := fakeHome(t)
	answer(t, iphone.SourceDevice)
	out := run(t, "setup", "photos")
	if !strings.Contains(out, `Saved photosSource "device"`) {
		t.Errorf("output %q", out)
	}
	cfg, err := config.Load(config.DefaultPath(home))
	if err != nil || cfg.PhotosSource != iphone.SourceDevice || cfg.MinSizeMB != 0 {
		t.Errorf("config %+v, %v", cfg, err)
	}
}

func TestSetupPhotosCancelChangesNothing(t *testing.T) {
	home := fakeHome(t)
	answer(t, "")
	path := config.DefaultPath(home)
	before, _ := os.ReadFile(path)
	if out := run(t, "setup", "photos"); !strings.Contains(out, "Nothing changed.") {
		t.Errorf("output %q", out)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Errorf("config changed to %s", after)
	}
}

func TestSetupPhotosNeedsTerminal(t *testing.T) {
	// go test has no terminal, so the real asker refuses.
	home := fakeHome(t)
	root := NewRoot()
	root.SetArgs([]string{"setup", "photos"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	err := root.ExecuteContext(context.Background())
	if !errors.Is(err, errNoTerminal) || !strings.Contains(err.Error(), config.DefaultPath(home)) {
		t.Errorf("got %v", err)
	}
}
