package clean

import (
	"slices"

	"github.com/shaynemeyer/groundskeeper/internal/iphone"
	"github.com/shaynemeyer/groundskeeper/internal/logging"
)

// Outcomes recorded for each iPhone app.
const (
	WouldUninstall = "would uninstall"
	Uninstalled    = "uninstalled"
	Declined       = "kept, not confirmed"
)

// AppAction is what happened, or would happen, to one iPhone app.
type AppAction struct {
	App     iphone.App
	Outcome string
	Reason  string // why it was refused or failed
}

// IPhoneApps uninstalls iPhone apps by bundle ID. It only accepts IDs from
// the last iPhone snapshot, refuses Apple's apps and anything in Keep, and
// asks Confirm before each uninstall when Apply is set.
type IPhoneApps struct {
	IDs       []string
	Snapshot  iphone.Phone
	Keep      []string
	Apply     bool
	Confirm   func(iphone.App) bool
	Uninstall func(bundleID string) error
	Log       *logging.Logger
}

// Clean plans or performs the uninstalls, one action per ID.
func (c IPhoneApps) Clean() []AppAction {
	var out []AppAction
	for _, id := range c.IDs {
		out = append(out, c.act(id))
	}
	return out
}

func (c IPhoneApps) act(id string) AppAction {
	app, reason := c.allowed(id)
	a := AppAction{App: app}
	if reason != "" {
		a.Outcome, a.Reason = Refused, reason
		c.Log.Warn("refused", logging.ID("bundle_id", id), logging.ID("reason", reason))
		return a
	}
	a.Outcome = WouldUninstall
	if c.Apply {
		a.Outcome = c.uninstall(app)
		if a.Outcome == Failed {
			a.Reason = "the iPhone refused or the connection dropped"
		}
	}
	c.Log.Info("app", logging.ID("bundle_id", id), logging.Bytes(app.Bytes()),
		logging.ID("outcome", a.Outcome), logging.Flag("dry_run", !c.Apply))
	return a
}

func (c IPhoneApps) uninstall(app iphone.App) string {
	if !c.Confirm(app) {
		return Declined
	}
	if err := c.Uninstall(app.BundleID); err != nil {
		return Failed
	}
	return Uninstalled
}

// allowed finds id in the snapshot, or says why it may not be uninstalled.
func (c IPhoneApps) allowed(id string) (iphone.App, string) {
	i := slices.IndexFunc(c.Snapshot.Apps, func(a iphone.App) bool { return a.BundleID == id })
	if i < 0 {
		return iphone.App{BundleID: id, Name: id}, "not in the last iPhone scan; run gk iphone first"
	}
	app := c.Snapshot.Apps[i]
	switch {
	case app.Apple():
		return app, "one of Apple's apps"
	case slices.Contains(c.Keep, id):
		return app, "listed in iphoneKeep"
	}
	return app, ""
}
