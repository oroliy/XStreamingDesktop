// XStreaming's native Go desktop client is under development.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oroliy/XStreamingDesktop/go-client/auth"
	"github.com/oroliy/XStreamingDesktop/go-client/catalog"
	"github.com/oroliy/XStreamingDesktop/go-client/controller"
	"github.com/oroliy/XStreamingDesktop/go-client/media"
	"github.com/oroliy/XStreamingDesktop/go-client/protocol"
	"github.com/oroliy/XStreamingDesktop/go-client/remote"
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
	cancelingLogin    bool
	status, code, uri string
	wakeWarning       string
	consoles          []streaming.Console
	client            *streaming.Client
	cloud             *streaming.Client
	titles            []streaming.Title
	cloudStatus       string
	cloudRegions      []string
	credentials       *auth.Session
	accountCtx        context.Context
	accountCancel     context.CancelFunc
	streamCancel      context.CancelFunc
	streamWG          *sync.WaitGroup
	streamDone        <-chan error
	streamHomeID      string
	streamPowerOff    *atomic.Bool
	frame             *ui.Bitmap
	decodedFPS        float64
	framePending      atomic.Bool
	cloudGeneration   atomic.Uint64
	cloudCancel       context.CancelFunc
	prefs             settings.Settings
	settingsPath      string
	filter            string
	cloudPage         int
	keyboard          controller.Keyboard
	keyboardEnabled   bool
	fullscreen        bool
}

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(12).Children(func() {
		ui.Text(c, "XStreaming").FontSize(32).Bold()
		ui.Text(c, a.tr("Experimental preview — controller and playback support are unverified."))
		if a.frame != nil {
			ui.Image(c, a.frame).FillWidth().Grow(1).Fit(ui.Contain)
			ui.Text(c, fmt.Sprintf(a.tr("Decoded video: %.1f FPS"), a.decodedFPS))
		}
		if a.streamCancel != nil {
			label := "Full screen"
			if a.fullscreen {
				label = "Exit full screen"
			}
			if ui.Button(c.Key("fullscreen"), a.tr(label)).Clicked() && a.win != nil {
				a.win.ToggleFullScreen()
			}
			if ui.Button(c.Key("stop"), a.tr("Stop stream")).Clicked() {
				a.stopStream(false)
			}
			if a.streamCancel != nil && a.streamHomeID != "" && a.streamDone != nil && a.streamPowerOff != nil && a.accountCtx != nil && a.credentials != nil {
				if ui.Button(c.Key("stop-and-power-off"), a.tr("Disconnect and power off")).Clicked() {
					a.stopStream(true)
				}
			}
		}
		if a.status != "" {
			ui.Text(c, a.tr(a.status))
		}
		if a.wakeWarning != "" {
			ui.Text(c, a.tr(a.wakeWarning))
		}
		if a.code != "" {
			ui.Text(c, a.tr("Visit: ")+a.uri)
			ui.Text(c, a.tr("Code: ")+a.code).FontSize(24).Bold()
			if verificationURLAllowed(a.uri) {
				if ui.Button(c.Key("open-verification"), a.tr("Open Microsoft sign-in page")).Clicked() {
					address := a.uri
					c.OpenURLThen(address, func(err error) {
						if err != nil && a.uri == address {
							a.status = "Unable to open browser; enter the address manually."
						}
					})
				}
			}
		}
		if a.busy && a.credentials == nil && a.accountCancel != nil && !a.cancelingLogin {
			if ui.Button(c.Key("cancel-login"), a.tr("Cancel sign-in")).Clicked() {
				a.cancelingLogin = true
				a.accountCancel()
				a.code = ""
				a.uri = ""
				a.status = "Canceling sign-in…"
			}
		}
		if a.streamCancel != nil {
			label := "Enable keyboard controller"
			if a.keyboardEnabled {
				label = "Disable keyboard controller"
			}
			if ui.Button(c.Key("keyboard"), a.tr(label)).Clicked() {
				a.keyboardEnabled = !a.keyboardEnabled
			}
			if a.keyboardEnabled {
				a.keyboard.Set(true, controller.MapKeys(c.KeyDown))
				ui.Box(c.Key("keyboard-capture")).Focusable().AutoFocus().Label(a.tr("Keyboard controller")).HandleInput(func(ev ui.InputEvent) bool {
					if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEscape {
						a.keyboardEnabled = false
						a.keyboard.Set(false, protocol.Gamepad{})
					}
					return ev.Kind == ui.InputKeyDown || ev.Kind == ui.InputKeyUp
				}).Children(func() {
					ui.Text(c, a.tr("WASD: left stick · TFGH: right stick · IJKL: buttons · arrows: D-pad · Esc: disable"))
				})
			} else {
				a.keyboard.Set(false, protocol.Gamepad{})
			}
			return
		}
		if !a.busy && a.code == "" && a.credentials == nil {
			if ui.PrimaryButton(c, a.tr("Sign in with Microsoft")).Clicked() {
				a.busy = true
				a.status = "Requesting sign-in code…"
				ctx, cancel := context.WithCancel(a.ctx)
				a.accountCtx = ctx
				a.accountCancel = cancel
				go a.signIn(ctx, a.prefs)
			}
		}
		if a.credentials != nil && !a.busy && a.streamCancel == nil {
			if ui.Button(c.Key("signout"), a.tr("Sign out")).Clicked() {
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
						a.cloudRegions = nil
						a.wakeWarning = ""
						a.status = "Signed out."
						if err != nil {
							a.status = "Signed out locally; unable to remove saved login: " + err.Error()
						}
					})
				}()
			}
		}
		ui.Scroll(c.Key("library")).Grow(1).Width(680).Gap(8).Children(func() {
			ui.Text(c, a.tr("Consoles")).Bold()
			if a.credentials != nil && !a.busy && a.streamCancel == nil {
				if ui.Button(c.Key("refresh-consoles"), a.tr("Refresh consoles")).Clicked() {
					a.busy = true
					a.status = "Refreshing consoles…"
					go a.refreshConsoles(a.accountCtx, a.credentials, a.client)
				}
			}
			for _, console := range a.consoles {
				ui.Text(c, fmt.Sprintf("%s (%s) — %s", console.Name, console.ConsoleType, a.tr(console.PowerState)))
				if !a.busy && a.streamCancel == nil && console.ServerID != "" {
					if ui.Button(c.Key("wake-"+console.ServerID), a.tr("Wake console")).Clicked() {
						a.busy = true
						a.status = "Sending wake request…"
						go a.wakeConsole(a.accountCtx, a.credentials, console.ServerID)
					}
					if ui.Button(c.Key("power-off-"+console.ServerID), a.tr("Power off console")).Clicked() {
						a.busy = true
						a.status = "Sending power-off request…"
						go a.powerOffConsole(a.accountCtx, a.credentials, console.ServerID)
					}
					if ui.Button(c.Key(console.ServerID), a.tr("Start stream")).Clicked() {
						ctx, cancel := context.WithCancel(a.ctx)
						a.streamCancel = cancel
						a.busy = true
						a.status = "Starting stream…"
						a.wakeWarning = ""
						a.streamHomeID = console.ServerID
						wake := a.prefs.WakeBeforeStream && console.PowerState == "ConnectedStandby"
						a.startStreamAsync(ctx, a.client, a.credentials, console.ServerID, a.prefs, wake)
					}
				}
			}
			ui.Text(c, a.tr("Settings")).Bold()
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, a.tr("Interface language"))
				if ui.Select(c.Key("ui-language"), &a.prefs.UILanguage, []string{"English", "简体中文"}).Changed() && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, a.tr("Resolution"))
				changed := ui.Select(c.Key("resolution"), &a.prefs.Resolution, []string{"720p", "1080p", "1080p HQ"}).Changed()
				ui.Text(c, a.tr("Language"))
				languageChanged := ui.Select(c.Key("language"), &a.prefs.Language, settings.GameLanguages()).Changed()
				ui.Text(c, a.tr("Market"))
				marketChanged := ui.Select(c.Key("market"), &a.prefs.Market, settings.CatalogMarkets()).Changed()
				if (languageChanged || marketChanged) && a.credentials != nil && a.accountCtx != nil && a.accountCtx.Err() == nil && a.streamCancel == nil {
					a.startCloudLoad(a.accountCtx, a.credentials, a.prefs)
				}
				if (changed || languageChanged || marketChanged) && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
			})
			if a.credentials != nil {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, a.tr("Cloud region"))
					if ui.Select(c.Key("cloud-region"), &a.prefs.CloudRegion, cloudRegionOptions(a.cloudRegions, a.prefs.CloudRegion)).Changed() {
						if a.settingsPath != "" {
							if err := settings.Save(a.settingsPath, a.prefs); err != nil {
								a.status = "Unable to save settings: " + err.Error()
							}
						}
						if a.accountCtx != nil && a.accountCtx.Err() == nil {
							a.cloud = nil
							a.titles = nil
							a.startCloudLoad(a.accountCtx, a.credentials, a.prefs)
						}
					}
				})
			}
			if ui.Checkbox(c.Key("wake-before-stream"), &a.prefs.WakeBeforeStream, a.tr("Wake standby console before streaming")).Changed() && a.settingsPath != "" {
				if err := settings.Save(a.settingsPath, a.prefs); err != nil {
					a.status = "Unable to save settings: " + err.Error()
				}
			}
			if ui.Checkbox(c.Key("prefer-ipv6"), &a.prefs.PreferIPv6, a.tr("Prefer IPv6 candidates")).Changed() && a.settingsPath != "" {
				if err := settings.Save(a.settingsPath, a.prefs); err != nil {
					a.status = "Unable to save settings: " + err.Error()
				}
			}
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, a.tr("Starting volume"))
				if ui.Slider(c.Key("volume"), &a.prefs.Volume, 0, 100).Label(a.tr("Starting volume")).Width(180).Changed() && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
				ui.Text(c, fmt.Sprintf("%.0f%%", a.prefs.Volume))
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, a.tr("Stick dead zone"))
				if ui.Slider(c.Key("stick-deadzone"), &a.prefs.StickDeadzone, 0, 50).Label(a.tr("Stick dead zone")).Width(180).Changed() && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
				ui.Text(c, fmt.Sprintf("%.0f%%", a.prefs.StickDeadzone))
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, a.tr("Controller polling rate"))
				if ui.Select(c.Key("polling-rate"), &a.prefs.PollingRate, []string{"60 Hz", "125 Hz", "250 Hz"}).Changed() && a.settingsPath != "" {
					if err := settings.Save(a.settingsPath, a.prefs); err != nil {
						a.status = "Unable to save settings: " + err.Error()
					}
				}
			})
			ui.Text(c, a.tr("Cloud games")).Bold()
			if ui.TextInput(c.Key("search"), &a.filter).Placeholder(a.tr("Search games")).Label(a.tr("Search games")).Width(320).Changed() {
				a.cloudPage = 0
			}
			if a.credentials != nil && a.accountCtx != nil && !a.busy && a.streamCancel == nil {
				if ui.Button(c.Key("reload-cloud"), a.tr("Reload cloud catalog")).Clicked() {
					a.startCloudLoad(a.accountCtx, a.credentials, a.prefs)
				}
			}
			if a.cloudStatus != "" {
				ui.Text(c, a.tr(a.cloudStatus))
			}
			matches := filterCloudTitles(a.titles, a.filter)
			const pageSize = 40
			lastPage := max(0, (len(matches)-1)/pageSize)
			a.cloudPage = min(max(a.cloudPage, 0), lastPage)
			start := a.cloudPage * pageSize
			end := min(start+pageSize, len(matches))
			if len(matches) > 0 {
				ui.Row(c).Gap(8).Children(func() {
					if a.cloudPage > 0 && ui.Button(c.Key("cloud-previous"), a.tr("Previous games")).Clicked() {
						a.cloudPage--
					}
					ui.Text(c, fmt.Sprintf(a.tr("Games %d–%d of %d"), start+1, end, len(matches)))
					if a.cloudPage < lastPage && ui.Button(c.Key("cloud-next"), a.tr("Next games")).Clicked() {
						a.cloudPage++
					}
				})
			} else if a.filter != "" {
				ui.Text(c, a.tr("No matching games."))
			}
			for index, title := range matches[start:end] {
				ui.Text(c, title.Name)
				if !a.busy && a.streamCancel == nil && a.cloud != nil {
					if ui.Button(c.Key(fmt.Sprintf("cloud-%d-%s", start+index, title.TitleID)), a.tr("Play")).Clicked() {
						ctx, cancel := context.WithCancel(a.ctx)
						a.streamCancel = cancel
						a.busy = true
						a.status = "Starting cloud game…"
						a.wakeWarning = ""
						a.streamHomeID = ""
						a.startStreamAsync(ctx, a.cloud, a.credentials, title.TitleID, a.prefs, false)
					}
				}
			}
		})
	})
}

// verificationURLAllowed prevents opening non-HTTPS or unrelated URLs received from a service.
func verificationURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"microsoft.com", "microsoftonline.com", "live.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}
func (a *app) update(fn func()) { a.win.Update(fn) }
func (a *app) fail(ctx context.Context, err error) {
	a.update(func() {
		if ctx.Err() == nil {
			a.busy = false
			a.code = ""
			a.uri = ""
			a.status = "Sign-in failed: " + err.Error()
		}
	})
}
func (a *app) finishCanceledLogin(ctx context.Context) {
	if ctx.Err() == nil {
		return
	}
	a.update(func() {
		if a.accountCtx == ctx && a.cancelingLogin {
			a.busy = false
			a.cancelingLogin = false
			a.code = ""
			a.uri = ""
			a.status = "Sign-in canceled."
		}
	})
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
	defer a.finishCanceledLogin(ctx)
	microsoft := auth.NewClient()
	code, err := microsoft.Start(ctx)
	if err != nil {
		a.fail(ctx, err)
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
			a.fail(ctx, err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	a.connectAccount(ctx, token.RefreshToken, prefs, false)
}
func (a *app) connectAccount(ctx context.Context, refresh string, prefs settings.Settings, restoring bool) {
	a.update(func() {
		if ctx.Err() == nil {
			a.code = ""
			a.uri = ""
			a.status = "Connecting to Xbox…"
		}
	})
	if ctx.Err() != nil {
		return
	}
	credentials := auth.NewSession(refresh)
	onStoreError := func(error) {
		a.update(func() {
			if ctx.Err() == nil {
				a.status = "Unable to save renewed login; sign in may be required next time."
			}
		})
	}
	var persistErr error
	if restoring {
		persistErr = credentials.SetStore(secretStore{}, onStoreError)
	}

	client, consoles, homeErr := fetchHome(ctx, nil, func(ctx context.Context) (string, string, error) { return credentials.StreamingToken(ctx, "home") })
	if ctx.Err() != nil {
		return
	}
	if auth.IsInvalidGrant(homeErr) {
		a.update(func() {
			if ctx.Err() == nil {
				a.busy = false
				a.status = "Sign-in expired; please sign in again."
			}
		})
		return
	}
	a.update(func() {
		if ctx.Err() != nil {
			return
		}
		a.busy = false
		a.credentials = credentials
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
		if !restoring {
			if err := credentials.AttachStore(secretStore{}, onStoreError); err != nil {
				onStoreError(err)
			} else {
				go func() {
					if err := credentials.Persist(ctx); err != nil && ctx.Err() == nil {
						onStoreError(err)
					}
				}()
			}
		}
		a.startCloudLoad(ctx, credentials, a.prefs)
	})
}

// fetchHome uses the existing regional client, or acquires one on first use.
func fetchHome(ctx context.Context, client *streaming.Client, token func(context.Context) (string, string, error)) (*streaming.Client, []streaming.Console, error) {
	if client == nil {
		value, region, err := token(ctx)
		if err != nil {
			return nil, nil, err
		}
		client, err = streaming.NewClient(region, value, "home", nil)
		if err != nil {
			return nil, nil, err
		}
		client.SetRefresh(token)
	}
	consoles, err := client.Consoles(ctx)
	if err != nil {
		return nil, nil, err
	}
	return client, consoles, nil
}
func (a *app) refreshConsoles(ctx context.Context, credentials *auth.Session, client *streaming.Client) {
	client, consoles, err := fetchHome(ctx, client, func(ctx context.Context) (string, string, error) { return credentials.StreamingToken(ctx, "home") })
	a.update(func() {
		if ctx.Err() != nil {
			return
		}
		a.busy = false
		if err != nil {
			a.status = "Unable to refresh consoles: " + err.Error()
			return
		}
		a.client = client
		a.consoles = consoles
		a.status = fmt.Sprintf("Found %d console(s).", len(consoles))
	})
}

// beginCloudLoad runs on the UI thread so request order matches user actions.
func (a *app) beginCloudLoad(ctx context.Context) (context.Context, uint64) {
	if a.cloudCancel != nil {
		a.cloudCancel()
	}
	requestCtx, cancel := context.WithCancel(ctx)
	a.cloudCancel = cancel
	return requestCtx, a.cloudGeneration.Add(1)
}

func (a *app) startCloudLoad(ctx context.Context, credentials *auth.Session, prefs settings.Settings) {
	requestCtx, generation := a.beginCloudLoad(ctx)
	a.cloudStatus = "Loading cloud games…"
	go a.loadCloud(requestCtx, credentials, prefs, generation)
}

func (a *app) loadCloud(ctx context.Context, credentials *auth.Session, prefs settings.Settings, generation uint64) {
	selectedRegion := prefs.CloudRegion
	if selectedRegion == "Automatic" {
		selectedRegion = ""
	}
	token, region, regions, err := credentials.StreamingTokenForRegion(ctx, "cloud", selectedRegion)
	if len(regions) > 0 {
		a.update(func() {
			if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
				a.cloudRegions = regions
			}
		})
	}
	if err != nil {
		a.update(func() {
			if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
				a.cloudStatus = "Unable to connect to cloud games: " + err.Error()
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
	client.SetRefresh(func(ctx context.Context) (string, string, error) {
		token, region, _, err := credentials.StreamingTokenForRegion(ctx, "cloud", selectedRegion)
		return token, region, err
	})
	titles, err := client.Titles(ctx)
	if err != nil {
		a.update(func() {
			if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
				a.cloudStatus = "Unable to load cloud games: " + err.Error()
			}
		})
		return
	}
	names, err := catalog.NewClient().Resolve(ctx, titles, prefs.Market, prefs.Language)
	catalogUnavailable := err != nil
	if ctx.Err() != nil {
		return
	}
	names = playableTitles(names)
	a.update(func() {
		if ctx.Err() == nil && a.cloudGeneration.Load() == generation {
			a.cloud = client
			a.titles = names
			a.cloudPage = 0
			a.cloudStatus = fmt.Sprintf("%d games", len(names))
			if catalogUnavailable {
				a.cloudStatus += " (some catalog names unavailable)"
			}
		}
	})
}
func cloudRegionOptions(regions []string, selected string) []string {
	options := []string{"Automatic"}
	seen := map[string]bool{"Automatic": true}
	for _, name := range append(append([]string(nil), regions...), selected) {
		if name != "" && !seen[name] {
			options = append(options, name)
			seen[name] = true
		}
	}
	return options
}

func filterCloudTitles(titles []streaming.Title, filter string) []streaming.Title {
	filter = strings.ToLower(strings.TrimSpace(filter))
	matches := make([]streaming.Title, 0, len(titles))
	for _, title := range titles {
		if title.TitleID != "" && strings.Contains(strings.ToLower(title.Name), filter) {
			matches = append(matches, title)
		}
	}
	return matches
}

func playableTitles(titles []streaming.Title) []streaming.Title {
	out := make([]streaming.Title, 0, len(titles))
	for _, title := range titles {
		if title.TitleID != "" {
			out = append(out, title)
		}
	}
	return out
}
func (a *app) restore(ctx context.Context, prefs settings.Settings) {
	defer a.finishCanceledLogin(ctx)
	token, err := mygo.Secrets.Service("XStreamingDesktop-Go").Get(tokenKey)
	if err != nil || len(token) == 0 {
		a.update(func() {
			if ctx.Err() != nil {
				return
			}
			a.busy = false
			if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
				a.status = "System credential store unavailable; sign in manually."
			}
		})
		return
	}
	if ctx.Err() != nil {
		return
	}
	a.update(func() {
		if ctx.Err() == nil {
			a.status = "Restoring sign-in…"
		}
	})
	a.connectAccount(ctx, string(token), prefs, true)
}
func (a *app) wakeConsole(ctx context.Context, credentials *auth.Session, id string) {
	token, err := credentials.WebToken(ctx)
	if err == nil {
		err = remote.NewClient().Wake(ctx, id, token)
	}
	a.update(func() {
		if ctx.Err() != nil {
			return
		}
		a.busy = false
		if err != nil {
			a.status = "Wake request failed: " + err.Error()
		} else {
			a.status = "Wake request sent. The console may take a moment to start."
		}
	})
}

func (a *app) powerOffConsole(ctx context.Context, credentials *auth.Session, id string) {
	token, err := credentials.WebToken(ctx)
	if err == nil {
		err = remote.NewClient().PowerOff(ctx, id, token)
	}
	a.update(func() {
		if ctx.Err() != nil {
			return
		}
		a.busy = false
		if err != nil {
			a.status = "Power-off request failed: " + err.Error()
		} else {
			a.status = "Power-off request sent. Refresh the console list to check its state."
		}
	})
}

// showFrame runs on the UI thread, reusing the bitmap and updating startup status.
func (a *app) showFrame(img image.Image) {
	if a.frame == nil {
		a.frame = ui.NewBitmap(img)
	} else {
		a.frame.Update(img)
	}
	switch a.status {
	case "Connected; waiting for video…", "Negotiation complete; waiting for connection…", "Negotiating connection…":
		a.status = "Streaming video."
	}
}

// stopStream runs on the UI thread. A power-off request waits for cleanup first.
func (a *app) stopStream(powerOff bool) {
	if a.streamCancel == nil {
		return
	}
	if powerOff {
		flag, done := a.streamPowerOff, a.streamDone
		ctx, credentials, id := a.accountCtx, a.credentials, a.streamHomeID
		flag.Store(true)
		go func() {
			if err := waitForStreamCleanup(ctx, done); err != nil {
				if ctx.Err() == nil {
					a.update(func() {
						if ctx.Err() == nil {
							a.busy = false
							a.status = "Stream cleanup failed; console was not powered off: " + err.Error()
						}
					})
				}
				return
			}
			a.powerOffConsole(ctx, credentials, id)
		}()
	}
	if a.fullscreen && a.win != nil {
		a.win.SetFullScreen(false)
	}
	a.streamCancel()
	a.streamCancel = nil
	a.busy = true
	a.frame = nil
	a.decodedFPS = 0
	a.wakeWarning = ""
	a.keyboardEnabled = false
	a.keyboard.Set(false, protocol.Gamepad{})
	if powerOff {
		a.status = "Stopping stream before power-off…"
	} else {
		a.status = "Stopping stream…"
	}
}

func waitForStreamCleanup(ctx context.Context, done <-chan error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// startStreamAsync is called on the UI thread before shutdown can begin waiting.
func (a *app) startStreamAsync(ctx context.Context, client *streaming.Client, credentials *auth.Session, target string, prefs settings.Settings, wakeBefore bool) {
	a.streamWG.Add(1)
	done := make(chan error, 1)
	flag := &atomic.Bool{}
	a.streamDone, a.streamPowerOff = done, flag
	go func() {
		defer a.streamWG.Done()
		done <- a.startStream(ctx, client, credentials, target, prefs, flag, wakeBefore)
		close(done)
	}()
}

func (a *app) startStream(ctx context.Context, client *streaming.Client, credentials *auth.Session, target string, prefs settings.Settings, powerOff *atomic.Bool, wakeBefore bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := media.CheckDependencies(); err != nil {
		stopped := ctx.Err() != nil
		a.update(func() {
			if powerOff.Load() {
				return
			}
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
		return nil
	}
	if wakeBefore {
		a.update(func() {
			if ctx.Err() == nil {
				a.status = "Sending wake request…"
			}
		})
		token, err := credentials.WebToken(ctx)
		if err == nil {
			err = remote.NewClient().Wake(ctx, target, token)
		}
		if err != nil && ctx.Err() == nil {
			a.update(func() {
				if ctx.Err() == nil {
					a.wakeWarning = "Wake request failed; continuing stream: " + err.Error()
				}
			})
		}
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
	var decodedFrames atomic.Uint64
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
			if ctx.Err() != nil {
				return
			}
			decodedFrames.Add(1)
			if !a.framePending.CompareAndSwap(false, true) {
				return
			}
			a.update(func() {
				defer a.framePending.Store(false)
				if ctx.Err() != nil {
					return
				}
				a.showFrame(img)
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
	connected := make(chan struct{})
	var connectedOnce sync.Once
	s, err := session.Start(ctx, client, session.Options{Play: streaming.PlayOptions{Target: target, Resolution: prefs.ResolutionCode(), Locale: prefs.Language}, TransferToken: func(ctx context.Context) (string, error) {
		return credentials.Transfer(ctx)
	}, OnProgress: func(state string) {
		var text string
		switch state {
		case "Provisioning":
			text = "Preparing stream…"
		case "WaitingForResources":
			text = "Waiting for available capacity…"
		case "ReadyToConnect":
			text = "Authorizing connection…"
		case "Provisioned", "Negotiating":
			text = "Negotiating connection…"
		default:
			return
		}
		a.update(func() {
			if ctx.Err() == nil && a.busy && a.streamCancel != nil {
				a.status = text
			}
		})
		if state == "WaitingForResources" {
			go func() {
				waitCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				defer cancel()
				wait, err := client.WaitTime(waitCtx, target)
				if err != nil || ctx.Err() != nil {
					return
				}
				minutes := int((wait + time.Minute - 1) / time.Minute)
				a.update(func() {
					if ctx.Err() == nil && a.busy && a.streamCancel != nil && a.status == "Waiting for available capacity…" {
						a.status = fmt.Sprintf("Estimated queue wait: %d minute(s).", minutes)
					}
				})
			}()
		}
	}, RTC: webrtc.Configuration{ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:worldaz.relay.teams.microsoft.com:3478"}}}}, OnTrack: track, OnError: onError, PreferIPv6: prefs.PreferIPv6, OnConnectionState: func(state webrtc.PeerConnectionState) {
		if ctx.Err() != nil {
			return
		}
		switch state {
		case webrtc.PeerConnectionStateConnected:
			connectedOnce.Do(func() { close(connected) })
			a.update(func() {
				if ctx.Err() == nil {
					if a.frame == nil {
						a.status = "Connected; waiting for video…"
					} else {
						a.status = "Streaming video."
					}
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
		var cleanupErr *session.CleanupError
		errors.As(err, &cleanupErr)
		a.update(func() {
			if powerOff.Load() {
				return
			}
			a.busy = false
			a.streamCancel = nil
			if a.fullscreen && a.win != nil {
				a.win.SetFullScreen(false)
			}
			if stopped && cleanupErr != nil {
				a.status = streamEndStatus(false, cleanupErr.Err)
			} else if stopped {
				a.status = "Stream stopped."
			} else {
				a.status = "Stream failed: " + err.Error()
			}
		})
		if cleanupErr != nil {
			return cleanupErr.Err
		}
		return nil
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastTime, lastCount := time.Now(), decodedFrames.Load()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.Done():
				return
			case now := <-ticker.C:
				count := decodedFrames.Load()
				fps := decodedFrameRate(lastCount, count, now.Sub(lastTime))
				lastCount, lastTime = count, now
				a.update(func() {
					if ctx.Err() != nil {
						return
					}
					select {
					case <-s.Done():
						return
					default:
					}
					a.decodedFPS = fps
				})
			}
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-s.Done():
			return
		case <-connected:
		}
		monitorControllerReady(ctx, s.Done(), s.Ready(), func() bool {
			return s.InputChannel().ReadyState() == webrtc.DataChannelStateOpen
		}, 15*time.Second, 100*time.Millisecond, func(waiting bool) {
			a.update(func() {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-s.Done():
					return
				default:
				}
				if waiting {
					a.status = "Controller input is still connecting."
				} else if a.status == "Controller input is still connecting." {
					if a.frame == nil {
						a.status = "Connected; waiting for video…"
					} else {
						a.status = "Streaming video."
					}
				}
			})
		})
	}()
	go func() {
		if err := controller.Run(ctx, s.Input(), s.InputChannel(), s.Ready(), &a.keyboard, prefs.StickDeadzone/100, prefs.PollingRateHz()); err != nil {
			onError(err)
		}
	}()
	a.update(func() {
		if ctx.Err() == nil {
			a.busy = false
			if a.status == "Starting stream…" || a.status == "Starting cloud game…" || a.status == "Preparing stream…" || a.status == "Waiting for available capacity…" || a.status == "Authorizing connection…" || a.status == "Negotiating connection…" {
				a.status = "Negotiation complete; waiting for connection…"
			}
		}
	})
	select {
	case <-ctx.Done():
	case <-s.Done():
	}
	failed := ctx.Err() == nil
	connectionTimedOut := failed && errors.Is(s.Err(), session.ErrConnectionTimeout)
	cleanupErr := s.Close()
	a.update(func() {
		if powerOff.Load() {
			return
		}
		a.busy = false
		a.streamCancel = nil
		if a.fullscreen && a.win != nil {
			a.win.SetFullScreen(false)
		}
		a.frame = nil
		a.decodedFPS = 0
		a.wakeWarning = ""
		a.keyboardEnabled = false
		a.keyboard.Set(false, protocol.Gamepad{})
		if connectionTimedOut && cleanupErr == nil {
			a.status = "Connection timed out; please retry."
		} else if connectionTimedOut {
			a.status = "Connection timed out; remote session cleanup failed: " + cleanupErr.Error()
		} else {
			a.status = streamEndStatus(failed, cleanupErr)
		}
	})
	return cleanupErr
}

func decodedFrameRate(previous, current uint64, elapsed time.Duration) float64 {
	if current < previous || elapsed <= 0 {
		return 0
	}
	return float64(current-previous) / elapsed.Seconds()
}

// monitorControllerReady reports a delayed handshake without interrupting video.
func monitorControllerReady(ctx context.Context, sessionDone, ready <-chan struct{}, inputOpen func() bool, timeout, poll time.Duration, notify func(bool)) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	warned, handshake := false, false
	for {
		if !handshake {
			select {
			case <-ready:
				handshake = true
			default:
			}
		}
		if handshake && inputOpen() {
			if warned {
				notify(false)
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-sessionDone:
			return
		case <-timer.C:
			warned = true
			notify(true)
		case <-ticker.C:
		}
	}
}

func streamEndStatus(failed bool, cleanupErr error) string {
	if cleanupErr != nil {
		if failed {
			return "Connection failed; remote session cleanup failed: " + cleanupErr.Error()
		}
		return "Stream stopped; remote session cleanup failed: " + cleanupErr.Error()
	}
	if failed {
		return "Connection failed; please retry."
	}
	return "Stream stopped."
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	var streamWG sync.WaitGroup
	mygo.App.WhenReady(func() {
		accountCtx, accountCancel := context.WithCancel(ctx)
		prefs := settings.Defaults()
		path, err := settings.Path()
		if err == nil {
			if saved, e := settings.Load(path); e == nil {
				prefs = saved
			}
		}
		a := &app{ctx: ctx, cancel: cancel, accountCtx: accountCtx, accountCancel: accountCancel, busy: true, prefs: prefs, settingsPath: path, streamWG: &streamWG}
		a.win = mygo.NewWindow(mygo.WindowOptions{Title: "XStreaming (Go preview)", Content: ui.View(a.view)})
		a.win.OnEnterFullScreen(func() { a.fullscreen = true; a.win.Invalidate() })
		a.win.OnLeaveFullScreen(func() { a.fullscreen = false; a.win.Invalidate() })
		a.win.OnClosed(a.cancel)
		go a.restore(accountCtx, prefs)
	})
	err := mygo.App.Run()
	cancel()
	if !waitForStreams(&streamWG, 7*time.Second) {
		log.Print("timed out waiting for stream cleanup")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func waitForStreams(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
