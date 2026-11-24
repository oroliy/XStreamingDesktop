// XStreaming's native Go desktop client is under development.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
)

type app struct {
	win               *mygo.Window
	ctx               context.Context
	cancel            context.CancelFunc
	busy              bool
	status, code, uri string
	consoles          []streaming.Console
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "XStreaming").FontSize(32).Bold()
		ui.Text(c, "Go preview — streaming playback is not available yet.")
		if a.status != "" {
			ui.Text(c, a.status)
		}
		if a.code != "" {
			ui.Text(c, "Visit: "+a.uri)
			ui.Text(c, "Code: "+a.code).FontSize(24).Bold()
		}
		if !a.busy && a.code == "" {
			if ui.PrimaryButton(c, "Sign in with Microsoft").Clicked() {
				a.busy = true
				a.status = "Requesting sign-in code…"
				go a.signIn()
			}
		}
		for _, console := range a.consoles {
			ui.Text(c, fmt.Sprintf("%s (%s) — %s", console.Name, console.ConsoleType, console.PowerState))
		}
	})
}
func (a *app) update(fn func()) { a.win.Update(fn) }
func (a *app) fail(err error) {
	a.update(func() { a.busy = false; a.code = ""; a.uri = ""; a.status = "Sign-in failed: " + err.Error() })
}
func (a *app) signIn() {
	microsoft := auth.NewClient()
	code, err := microsoft.Start(a.ctx)
	if err != nil {
		a.fail(err)
		return
	}
	a.update(func() {
		a.code = code.UserCode
		a.uri = code.VerificationURI
		a.status = "Complete sign-in in your browser."
	})
	token, err := microsoft.Poll(a.ctx, code)
	if err != nil {
		a.fail(err)
		return
	}
	a.update(func() { a.code = ""; a.uri = ""; a.status = "Connecting to Xbox…" })
	xbox := auth.NewXboxClient()
	user, err := xbox.UserToken(a.ctx, token.AccessToken)
	if err != nil {
		a.fail(err)
		return
	}
	xsts, err := xbox.XSTS(a.ctx, user.Token, "http://gssv.xboxlive.com/")
	if err != nil {
		a.fail(err)
		return
	}
	home, err := xbox.Streaming(a.ctx, xsts.Token, "xhome")
	if err != nil {
		a.fail(err)
		return
	}
	region, err := home.DefaultRegion()
	if err != nil {
		a.fail(err)
		return
	}
	client, err := streaming.NewClient(region.BaseURI, home.GSToken, "home", nil)
	if err != nil {
		a.fail(err)
		return
	}
	consoles, err := client.Consoles(a.ctx)
	if err != nil {
		a.fail(err)
		return
	}
	a.update(func() {
		a.busy = false
		a.consoles = consoles
		a.status = fmt.Sprintf("Signed in. Found %d console(s).", len(consoles))
	})
}
func main() {
	mygo.App.WhenReady(func() {
		ctx, cancel := context.WithCancel(context.Background())
		a := &app{ctx: ctx, cancel: cancel}
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "XStreaming (Go preview)", Content: ui.View(a.view)})
		a.win.OnClosed(a.cancel)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
