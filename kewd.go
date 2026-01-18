package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

var (
	supportedImg = []string{".png", ".jpg", ".jpeg", ".webp"}

	runtimeDir = func() string {
		if v := os.Getenv("XDG_RUNTIME_DIR"); v != "" {
			return v
		}
		return fmt.Sprintf("/tmp/live-%d", os.Getuid())
	}()

	sockPath = filepath.Join(runtimeDir, "live.sock")
	lock     sync.Mutex
)

/* ---------------- wallpaper resolver ---------------- */

func findWallpaper() string {
	home, _ := os.UserHomeDir()
	userDir := filepath.Join(home, ".config")
	sysDir := "/etc/kereta"

	for _, base := range []string{userDir, sysDir} {
		for _, ext := range supportedImg {
			p := filepath.Join(base, "wall"+ext)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

/* ---------------- desktop window ---------------- */

type Desktop struct {
	win   *gtk.Window
	image *gtk.Image
}

func NewDesktop() *Desktop {
	win, _ := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)

	win.SetDecorated(false)
	win.SetSkipTaskbarHint(true)
	win.SetSkipPagerHint(true)
	win.SetAcceptFocus(false)
	win.SetFocusOnMap(false)

	d := &Desktop{
		win:   win,
		image: nil,
	}

	win.Connect("realize", d.onRealize)
	d.loadWallpaper()

	win.ShowAll()
	return d
}

func (d *Desktop) loadWallpaper() {
	lock.Lock()
	defer lock.Unlock()

	children := d.win.GetChildren()
	children.Foreach(func(item interface{}) {
		if w, ok := item.(gtk.IWidget); ok {
			d.win.Remove(w)
		}
	})

	path := findWallpaper()
	if path == "" {
		d.win.OverrideBackgroundColor(
			gtk.STATE_FLAG_NORMAL,
			&gdk.RGBA{Red: 0, Green: 0, Blue: 0, Alpha: 1},
		)
		return
	}

	img, _ := gtk.ImageNewFromFile(path)
	img.SetHAlign(gtk.ALIGN_FILL)
	img.SetVAlign(gtk.ALIGN_FILL)

	d.image = img
	d.win.Add(img)
}

func (d *Desktop) reload() {
	fmt.Println("Reloading wallpaper")
	d.loadWallpaper()
	d.win.ShowAll()
}

func (d *Desktop) onRealize() {
	gdkWin, _ := d.win.GetWindow()
	gdkWin.SetTypeHint(gdk.WINDOW_TYPE_HINT_DESKTOP)
	gdkWin.SetKeepBelow(true)

	display, _ := gdk.DisplayGetDefault()
	screen, _ := display.GetDefaultScreen()

	w := screen.GetWidth()
	h := screen.GetHeight()

	d.win.Move(0, 0)
	d.win.Resize(w, h)
}

/* ---------------- control socket ---------------- */

func controlServer(d *Desktop) {
	os.MkdirAll(runtimeDir, 0700)
	os.Remove(sockPath)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	os.Chmod(sockPath, 0600)

	fmt.Println("Control socket:", sockPath)

	for {
		conn, err := l.Accept()
		if err != nil {
			continue
		}

		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 256)
			n, _ := c.Read(buf)

			cmd := strings.TrimSpace(strings.ToLower(string(buf[:n])))

			switch cmd {
			case "reload":
				glib.IdleAdd(func() {
					d.reload()
				})
				c.Write([]byte("OK reloaded\n"))

			case "exit":
				c.Write([]byte("OK exiting\n"))
				glib.IdleAdd(func() {
					gtk.MainQuit()
				})

			default:
				c.Write([]byte("ERR unknown command\n"))
			}
		}(conn)
	}
}

/* ---------------- main ---------------- */

func main() {
	gtk.Init(nil)

	desktop := NewDesktop()
	go controlServer(desktop)

	gtk.Main()
}
