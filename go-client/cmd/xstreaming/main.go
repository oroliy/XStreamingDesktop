// XStreaming's native Go desktop client is under development.
package main

import (
	"context"
	"fmt"
	"image"
	"log"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/catalog"
	"github.com/oroliy/XStreamingDesktop/go-client/controller"
	"github.com/oroliy/XStreamingDesktop/go-client/media"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
	"github.com/oroliy/XStreamingDesktop/go-client/streaming"
	"github.com/pion/webrtc/v4"
)

type app struct {
	win               *mygo.Window
	ctx               context.Context
	cancel            context.CancelFunc
	busy              bool
	status, code, uri string
	consoles          []streaming.Console
	client            *streaming.Client
	cloud             *streaming.Client
	titles            []streaming.Title
	cloudStatus       string
	credentials       *auth.Session
	streamCancel      context.CancelFunc
	frame             *ui.Bitmap
	framePending      atomic.Bool
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "XStreaming").FontSize(32).Bold()
		ui.Text(c, "Experimental preview — controller and playback support are unverified.")
		if a.frame != nil {
			ui.Image(c, a.frame).Width(640).Height(360).Fit(ui.Contain)
		}
		if a.streamCancel != nil {
			if ui.Button(c.Key("stop"), "Stop stream").Clicked() {
				a.streamCancel()
				a.streamCancel = nil
				a.busy = true
				a.frame = nil
				a.status = "Stopping stream…"
			}
		}
		if a.status != "" {
			ui.Text(c, a.status)
		}
		if a.code != "" {
			ui.Text(c, "Visit: "+a.uri)
			ui.Text(c, "Code: "+a.code).FontSize(24).Bold()
		}
		if !a.busy && a.code == "" && a.client == nil {
			if ui.PrimaryButton(c, "Sign in with Microsoft").Clicked() {
				a.busy = true
				a.status = "Requesting sign-in code…"
				go a.signIn()
			}
		}
		ui.Scroll(c.Key("library")).Grow(1).Width(680).Gap(8).Children(func() {
			ui.Text(c, "Consoles").Bold()
			for _, console := range a.consoles {
				ui.Text(c, fmt.Sprintf("%s (%s) — %s", console.Name, console.ConsoleType, console.PowerState))
				if !a.busy && a.streamCancel == nil && console.ServerID != "" {
					if ui.Button(c.Key(console.ServerID), "Start stream").Clicked() {
						ctx, cancel := context.WithCancel(a.ctx)
						a.streamCancel = cancel
						a.busy = true
						a.status = "Starting stream…"
						go a.startStream(ctx, a.client, a.credentials, console.ServerID)
					}
				}
			}
			ui.Text(c, "Cloud games").Bold()
			if a.cloudStatus != "" {
				ui.Text(c, a.cloudStatus)
			}
			for index, title := range a.titles {
				if title.TitleID == "" {
					continue
				}
				ui.Text(c, title.Name)
				if !a.busy && a.streamCancel == nil && a.cloud != nil {
					if ui.Button(c.Key(fmt.Sprintf("cloud-%d-%s", index, title.TitleID)), "Play").Clicked() {
						ctx, cancel := context.WithCancel(a.ctx)
						a.streamCancel = cancel
						a.busy = true
						a.status = "Starting cloud game…"
						go a.startStream(ctx, a.cloud, a.credentials, title.TitleID)
					}
				}
			}
		})
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
	credentials := auth.NewSession(token.RefreshToken)
	client.SetRefresh(func(ctx context.Context) (string, string, error) { return credentials.StreamingToken(ctx, "home") })
	consoles, err := client.Consoles(a.ctx)
	if err != nil {
		a.fail(err)
		return
	}
	a.update(func() {
		a.busy = false
		a.client = client
		a.credentials = credentials
		a.consoles = consoles
		a.status = fmt.Sprintf("Signed in. Found %d console(s).", len(consoles))
		a.cloudStatus = "Loading cloud games…"
	})
	go a.loadCloud(xbox, xsts.Token, credentials)
}
func (a *app) loadCloud(xbox *auth.XboxClient, token string, credentials *auth.Session) {
	cloud, err := xbox.Streaming(a.ctx, token, "xgpuweb")
	if err != nil {
		cloud, err = xbox.Streaming(a.ctx, token, "xgpuwebf2p")
	}
	if err != nil {
		a.update(func() { a.cloudStatus = "Cloud games unavailable for this account or region." })
		return
	}
	region, err := cloud.DefaultRegion()
	if err != nil {
		a.update(func() { a.cloudStatus = "Cloud region unavailable." })
		return
	}
	client, err := streaming.NewClient(region.BaseURI, cloud.GSToken, "cloud", nil)
	if err != nil {
		a.update(func() { a.cloudStatus = "Invalid cloud service region." })
		return
	}
	client.SetRefresh(func(ctx context.Context) (string, string, error) { return credentials.StreamingToken(ctx, "cloud") })
	titles, err := client.Titles(a.ctx)
	if err != nil {
		a.update(func() { a.cloudStatus = "Unable to load cloud games." })
		return
	}
	names, err := catalog.NewClient().Resolve(a.ctx, titles, "US", "en-US")
	if err != nil {
		names = titles
		for i := range names {
			names[i].Name = names[i].Details.ProductID
			if names[i].Name == "" {
				names[i].Name = names[i].TitleID
			}
		}
	}
	a.update(func() { a.cloud = client; a.titles = names; a.cloudStatus = fmt.Sprintf("%d games", len(names)) })
}
func (a *app) startStream(ctx context.Context, client *streaming.Client, credentials *auth.Session, target string) {
	onError := func(err error) {
		if ctx.Err() == nil {
			a.update(func() {
				if ctx.Err() == nil {
					a.status = "Stream error: " + err.Error()
				}
			})
		}
	}
	track := func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if t.Kind() == webrtc.RTPCodecTypeAudio {
			if !strings.EqualFold(t.Codec().MimeType, webrtc.MimeTypeOpus) {
				onError(fmt.Errorf("unsupported audio codec: %s", t.Codec().MimeType))
				return
			}
			channels := t.Codec().Channels
			if channels == 0 {
				channels = 2
			}
			player, err := media.NewAudioPlayer(ctx, channels)
			if err != nil {
				onError(err)
				return
			}
			defer player.Close()
			if err = media.ReceiveOpus(t, player); err != nil && ctx.Err() == nil {
				onError(err)
			}
			return
		}
		if !strings.EqualFold(t.Codec().MimeType, webrtc.MimeTypeH264) {
			onError(fmt.Errorf("unsupported media codec: %s", t.Codec().MimeType))
			return
		}
		decoder, err := media.NewVideoDecoder(ctx, func(img image.Image) {
			if ctx.Err() != nil || !a.framePending.CompareAndSwap(false, true) {
				return
			}
			a.update(func() {
				defer a.framePending.Store(false)
				if ctx.Err() != nil {
					return
				}
				if a.frame == nil {
					a.frame = ui.NewBitmap(img)
				} else {
					a.frame.Update(img)
				}
			})
		})
		if err != nil {
			onError(err)
			return
		}
		defer decoder.Close()
		if err = media.ReceiveH264(t, decoder); err != nil && ctx.Err() == nil {
			onError(err)
		}
	}
	s, err := session.Start(ctx, client, session.Options{Play: streaming.PlayOptions{Target: target, Resolution: 1080}, TransferToken: func(ctx context.Context) (string, error) {
		return credentials.Transfer(ctx)
	}, RTC: webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:worldaz.relay.teams.microsoft.com:3478"}}}}, OnTrack: track, OnError: onError})
	if err != nil {
		a.update(func() {
			a.busy = false
			a.streamCancel = nil
			if ctx.Err() != nil {
				a.status = "Stream stopped."
			} else {
				a.status = "Stream failed: " + err.Error()
			}
		})
		return
	}
	go func() {
		if err := controller.Run(ctx, s.Input(), s.InputChannel(), s.Ready()); err != nil {
			onError(err)
		}
	}()
	a.update(func() {
		if ctx.Err() == nil {
			a.busy = false
			a.status = "Connected; waiting for video…"
		}
	})
	<-ctx.Done()
	_ = s.Close()
	a.update(func() {
		if a.streamCancel == nil {
			a.busy = false
			a.status = "Stream stopped."
		}
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
