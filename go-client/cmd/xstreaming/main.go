// XStreaming's native Go desktop client is under development.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "XStreaming (Go preview)",
			Content: ui.View(func(c *ui.Context) {
				ui.Column(c).Fill().Center().Gap(12).Children(func() {
					ui.Text(c, "XStreaming").FontSize(32).Bold()
					ui.Text(c, "Go client in development")
					ui.Text(c, "Sign-in and streaming are not available yet.")
				})
			}),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
