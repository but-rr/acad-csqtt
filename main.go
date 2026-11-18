package main

import (
	"embed"
	"log"
	"os"
	"strings"

	"csqtt.desktop/internal/tray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/icons/CSQTT.ico
var trayIcon []byte

//go:embed VERSION
var version string

func main() {
	waitForPrevious()
	app := NewApp(appDirectory(), strings.TrimSpace(version), hasArgument("--smoke"))
	err := wails.Run(&options.App{Title: "CSQTT", Width: 1180, Height: 840, MinWidth: 880, MinHeight: 620, Frameless: true, BackgroundColour: &options.RGBA{R: 20, G: 21, B: 23, A: 255}, AssetServer: &assetserver.Options{Assets: assets}, OnStartup: app.startup, OnDomReady: app.domReady, OnBeforeClose: app.beforeClose, OnShutdown: app.shutdown, Bind: []interface{}{app}, Windows: &windows.Options{Theme: windows.Dark, WebviewUserDataPath: os.Getenv("CSQTT_WEBVIEW_DATA"), ResizeDebounceMS: 16, WebviewIsTransparent: false, WindowIsTranslucent: false}, SingleInstanceLock: &options.SingleInstanceLock{UniqueId: tray.Identity, OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.restoreWindow() }}})
	if err != nil {
		log.Fatal(err)
	}
}
