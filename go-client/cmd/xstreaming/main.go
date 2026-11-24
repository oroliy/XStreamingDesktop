// XStreaming's native Go desktop client is under development.
package main

import (
	"context"
	"errors"
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
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/oroliy/XStreamingDesktop/go-client/session"
	"github.com/oroliy/XStreamingDesktop/go-client/settings"
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
	accountCtx        context.Context
	accountCancel     context.CancelFunc
	streamCancel      context.CancelFunc
	frame             *ui.Bitmap
	framePending      atomic.Bool
	cloudGeneration   atomic.Uint64
	prefs             settings.Settings
	settingsPath      string
	filter            string
	keyboard          controller.Keyboard
	keyboardEnabled   bool
	fullscreen        bool
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "XStreaming").FontSize(32).Bold()
		ui.Text(c, "Experimental preview — controller and playback support are unverified.")
		if a.frame != nil {
			ui.Image(c, a.frame).FillWidth().Grow(1).Fit(ui.Contain)
		}
		if a.streamCancel != nil {
			label := "Full screen"
			if a.fullscreen {
				label = "Exit full screen"
			}
			if ui.Button(c.Key("fullscreen"), label).Clicked() && a.win != nil {
				a.win.ToggleFullScreen()
			}
			if ui.Button(c.Key("stop"), "Stop stream").Clicked() {
				if a.fullscreen && a.win != nil {
					a.win.SetFullScreen(false)
				}
				a.streamCancel()
				a.streamCancel = nil
				a.busy = true
				a.frame = nil
				a.keyboardEnabled = false
				a.keyboard.Set(false, protocol.Gamepad{})
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
		if a.streamCancel != nil {
			label := "Enable keyboard controller"
			if a.keyboardEnabled {
				label = "Disable keyboard controller"
			}
			if ui.Button(c.Key("keyboard"), label).Clicked() {
				a.keyboardEnabled = !a.keyboardEnabled
			}
			if a.keyboardEnabled {
				a.keyboard.Set(true, controller.MapKeys(c.KeyDown))
				ui.Box(c.Key("keyboard-capture")).Focusable().AutoFocus().Label("Keyboard controller").HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
						a.keyboardEnabled = false
						a.keyboard.Set(false, protocol.Gamepad{})
					}
					return ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp
				}).Children(func() {
					ui.Text(c, "WASD: left stick · TFGH: right stick · IJKL: buttons · arrows: D-pad · Esc: disable")
				})
			} else {
				a.keyboard.Set(false, protocol.Gamepad{})
			}
			return
		}
		if !a.busy && a.code == "" && a.credentials == nil {
			if ui.PrimaryButton(c, "Sign in with Microsoft").Clicked() {
				a.busy = true
				a.status = "Requesting sign-in code…"
				ctx, cancel := context.WithCancel(a.ctx)
				a.accountCtx = ctx
				a.accountCancel = cancel
				go a.signIn(ctx, a.prefs)
			}
		}
		if a.credentials != nil && !a.busy && a.streamCancel == nil {
			if ui.Button(c.Key("signout"), "Sign out").Clicked() {
				a.busy = true
				a.status = "Signing out…"
				a.accountCancel()
				credentials := a.credentials
				go func() {
					err := credentials.Logout()
					a.update(func() {
						a.busy = false
						a.client = nil
						a.cloud = nil
						a.credentials = nil
						a.consoles = nil
						a.titles = nil
						a.cloudStatus = ""
						a.status = "Signed out."
						if err != nil {
							a.status = "Signed out locally; unable to remove saved login: " + err.Error()
						}
					})
				}()
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
						go a.startStream(ctx, a.client, a.credentials, console.ServerID, a.prefs)
					}
				}
			}
			ui.Text(c, "Settings").Bold()
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "Resolution")
				changed := ui.Select(c.Key("resolution"), &a.prefs.Resolution, []string{"720p", "1080p", "1080p HQ"}).Changed()
				ui.Text(c, "Language")
				changed = ui.Select(c.Key("language"), &a.prefs.Language, []string{"en-US", "zh-TW", "zh-CN", "ja-JP"}).Changed() || changed
				ui.Text(c, "Market")
				changed = ui.Select(c.Key("market"), &a.prefs.Market, []string{"US", "TW", "JP", "GB"}).Changed() || changed
				if changed && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "Starting volume")
				if ui.Slider(c.Key("volume"), &a.prefs.Volume, 0, 100).Label("Starting volume").Width(180).Changed() && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
				ui.Text(c, fmt.Sprintf("%.0f%%", a.prefs.Volume))
			})
			ui.Text(c, "Cloud games").Bold()
			ui.TextInput(c.Key("search"), &a.filter).Placeholder("Search games").Label("Search games").Width(320)
			if a.credentials != nil && a.accountCtx != nil && !a.busy && a.streamCancel == nil {
				if ui.Button(c.Key("reload-cloud"), "Reload cloud catalog").Clicked() {
					a.cloudStatus = "Loading cloud games…"
					go a.loadCloud(a.accountCtx, a.credentials, a.prefs)
				}
			}
			if a.cloudStatus != "" {
				ui.Text(c, a.cloudStatus)
			}
			for index, title := range a.titles {
				if title.TitleID == "" || !strings.Contains(strings.ToLower(title.Name), strings.ToLower(strings.TrimSpace(a.filter))) {
					continue
				}
				ui.Text(c, title.Name)
				if !a.busy && a.streamCancel == nil && a.cloud != nil {
					if ui.Button(c.Key(fmt.Sprintf("cloud-%d-%s", index, title.TitleID)), "Play").Clicked() {
						ctx, cancel := context.WithCancel(a.ctx)
						a.streamCancel = cancel
						a.busy = true
						a.status = "Starting cloud game…"
						go a.startStream(ctx, a.cloud, a.credentials, title.TitleID, a.prefs)
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

type secretStore struct{}

const tokenKey = "microsoft-refresh-token"

func (secretStore) Save(token string) error {
	return mygo.Secrets.Service("XStreamingDesktop-Go").Set(tokenKey, []byte(token))
}
func (secretStore) Delete() error {
	return mygo.Secrets.Service("XStreamingDesktop-Go").Delete(tokenKey)
}
func (a *app) signIn(ctx context.Context, prefs settings.Settings) {
	microsoft := auth.NewClient()
	code, err := microsoft.Start(ctx)
	if err != nil {
		a.fail(err)
		return
	}
	a.update(func() {
		if ctx.Err() == nil {
			a.code = code.UserCode
			a.uri = code.VerificationURI
			a.status = "Complete sign-in in your browser."
		}
	})
	token, err := microsoft.Poll(ctx, code)
	if err != nil {
		if ctx.Err() == nil {
			a.fail(err)
		}
		return
	}
	a.connectAccount(ctx, token.RefreshToken, prefs)
}
func (a *app) connectAccount(ctx context.Context, refresh string, prefs settings.Settings) {
	a.update(func() {
		if ctx.Err() == nil {
			a.code = ""
			a.uri = ""
			a.status = "Connecting to Xbox…"
		}
	})
	credentials := auth.NewSession(refresh)
	persistErr := credentials.SetStore(secretStore{}, func(error) {
		a.update(func() {
			if ctx.Err() == nil {
				a.status = "Unable to save renewed login; sign in may be required next time."
			}
		})
	})

	token, region, homeErr := credentials.StreamingToken(ctx, "home")
	var client *streaming.Client
	var consoles []streaming.Console
	if homeErr == nil {
		client, homeErr = streaming.NewClient(region, token, "home", nil)
		if homeErr == nil {
			client.SetRefresh(func(ctx context.Context) (string, string, error) { return credentials.StreamingToken(ctx, "home") })
			consoles, homeErr = client.Consoles(ctx)
		}
	}
	if ctx.Err() != nil {
		return
	}
	a.update(func() {
		if ctx.Err() != nil {
			return
		}
		a.busy = false
		a.credentials = credentials
		a.cloudStatus = "Loading cloud games…"
		if homeErr != nil {
			a.status = "Home streaming unavailable; checking cloud games."
		} else {
			a.client = client
			a.consoles = consoles
			a.status = fmt.Sprintf("Signed in. Found %d console(s).", len(consoles))
		}
		if persistErr != nil {
			a.status += " Login could not be saved in the system credential store."
		}
	})
	go a.loadCloud(ctx, credentials, prefs)
}
func (a *app) loadCloud(ctx context.Context, credentials *auth.Session, prefs settings.Settings) {
	generation := a.cloudGeneration.Add(1)
	token, region, err := credentials.StreamingToken(ctx, "cloud")
	if err != nil {
		a.update(func() {
			if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
				a.cloudStatus = "Cloud games unavailable for this account or region."
			}
		})
		return
	}
	client, err := streaming.NewClient(region, token, "cloud", nil)
	if err != nil {
		a.update(func() {
			if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
				a.cloudStatus = "Invalid cloud service region."
			}
		})
		return
	}
	client.SetRefresh(func(ctx context.Context) (string, string, error) { return credentials.StreamingToken(ctx, "cloud") })
	titles, err := client.Titles(ctx)
	if err != nil {
		a.update(func() {
			if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
				a.cloudStatus = "Unable to load cloud games."
			}
		})
		return
	}
	names, err := catalog.NewClient().Resolve(ctx, titles, prefs.Market, prefs.Language)
	if err != nil {
		names = titles
		for i := range names {
			names[i].Name = names[i].Details.ProductID
			if names[i].Name == "" {
				names[i].Name = names[i].TitleID
			}
		}
	}
	a.update(func() {
		if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
			a.cloud = client
			a.titles = names
			a.cloudStatus = fmt.Sprintf("%d games", len(names))
		}
	})
}
func (a *app) restore(prefs settings.Settings) {
	token, err := mygo.Secrets.Service("XStreamingDesktop-Go").Get(tokenKey)
	if err != nil || len(token) == 0 {
		a.update(func() {
			a.busy = false
			if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
				a.status = "System credential store unavailable; sign in manually."
			}
		})
		return
	}
	a.update(func() { a.status = "Restoring sign-in…" })
	a.connectAccount(a.accountCtx, string(token), prefs)
}
func (a *app) startStream(ctx context.Context, client *streaming.Client, credentials *auth.Session, target string, prefs settings.Settings) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := media.CheckDependencies(); err != nil {
		stopped := ctx.Err() != nil
		a.update(func() {
			a.busy = false
			a.streamCancel = nil
			if a.fullscreen && a.win != nil {
				a.win.SetFullScreen(false)
			}
			if stopped {
				a.status = "Stream stopped."
			} else {
				a.status = err.Error()
			}
		})
		return
	}
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
			player, err := media.NewAudioPlayer(ctx, channels, int(prefs.Volume))
			if err != nil {
				onError(err)
				return
			}
			readErr := media.ReceiveOpus(t, player)
			closeErr := player.Close()
			if ctx.Err() == nil {
				if closeErr != nil {
					onError(closeErr)
				} else if readErr != nil {
					onError(readErr)
				}
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
		readErr := media.ReceiveH264(t, decoder)
		closeErr := decoder.Close()
		if ctx.Err() == nil {
			if closeErr != nil {
				onError(closeErr)
			} else if readErr != nil {
				onError(readErr)
			}
		}
	}
	s, err := session.Start(ctx, client, session.Options{Play: streaming.PlayOptions{Target: target, Resolution: prefs.ResolutionCode(), Locale: prefs.Language}, TransferToken: func(ctx context.Context) (string, error) {
		return credentials.Transfer(ctx)
	}, RTC: webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:worldaz.relay.teams.microsoft.com:3478"}}}}, OnTrack: track, OnError: onError, OnConnectionState: func(state webrtc.PeerConnectionState) {
		if ctx.Err() != nil {
			return
		}
		switch state {
		case webrtc.PeerConnectionStateConnected:
			a.update(func() {
				if ctx.Err() == nil {
					a.status = "Connected; waiting for video…"
				}
			})
		case webrtc.PeerConnectionStateDisconnected:
			a.update(func() {
				if ctx.Err() == nil {
					a.status = "Connection interrupted…"
				}
			})
		case webrtc.PeerConnectionStateFailed:
			a.update(func() {
				if ctx.Err() == nil {
					a.status = "Connection failed; cleaning up…"
				}
			})
		}
	}})
	if err != nil {
		stopped := ctx.Err() != nil
		a.update(func() {
			a.busy = false
			a.streamCancel = nil
			if a.fullscreen && a.win != nil {
				a.win.SetFullScreen(false)
			}
			if stopped {
				a.status = "Stream stopped."
			} else {
				a.status = "Stream failed: " + err.Error()
			}
		})
		return
	}
	go func() {
		if err := controller.Run(ctx, s.Input(), s.InputChannel(), s.Ready(), &a.keyboard); err != nil {
			onError(err)
		}
	}()
	a.update(func() {
		if ctx.Err() == nil {
			a.busy = false
			if a.status == "Starting stream…" || a.status == "Starting cloud game…" {
				a.status = "Negotiation complete; waiting for connection…"
			}
		}
	})
	select {
	case <-ctx.Done():
	case <-s.Done():
	}
	failed := ctx.Err() == nil
	_ = s.Close()
	a.update(func() {
		a.busy = false
		a.streamCancel = nil
		if a.fullscreen && a.win != nil {
			a.win.SetFullScreen(false)
		}
		a.frame = nil
		a.keyboardEnabled = false
		a.keyboard.Set(false, protocol.Gamepad{})
		if failed {
			a.status = "Connection failed; please retry."
		} else {
			a.status = "Stream stopped."
		}
	})
}
func main() {
	mygo.App.WhenReady(func() {
		ctx, cancel := context.WithCancel(context.Background())
		accountCtx, accountCancel := context.WithCancel(ctx)
		prefs := settings.Defaults()
		path, err := settings.Path()
		if err == nil {
			if saved, e := settings.Load(path); e == nil {
				prefs = saved
			}
		}
		a := &app{ctx: ctx, cancel: cancel, accountCtx: accountCtx, accountCancel: accountCancel, busy: true, prefs: prefs, settingsPath: path}
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "XStreaming (Go preview)", Content: ui.View(a.view)})
		a.win.OnEnterFullScreen(func() { a.fullscreen = true; a.win.Invalidate() })
		a.win.OnLeaveFullScreen(func() { a.fullscreen = false; a.win.Invalidate() })
		a.win.OnClosed(a.cancel)
		go a.restore(prefs)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
