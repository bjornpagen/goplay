package chrome

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	goruntime "runtime"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/dom"
	"github.com/mafredri/cdp/protocol/input"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
)

// also serves as a sort of mutex
var _port uint16

// Browser is a struct that contains all the top level variables.
type Browser struct {
	w       window
	options *options

	pid  int
	conn *rpcc.Conn
	c    *cdp.Client
	//sm   *session.Manager
}

type options struct {
}

type Option func(option *options) error

// New creates a new browser instance with the given context.
func New(opts ...Option) (*Browser, error) {
	option := &options{}
	for _, opt := range opts {
		err := opt(option)
		if err != nil {
			return nil, err
		}
	}

	return &Browser{
		options: option,
	}, nil
}

func (b *Browser) Close() error {
	// Close the connection to Chrome.
	err := b.conn.Close()
	if err != nil {
		return fmt.Errorf("close connection: %v", err)
	}

	// Kill the Chrome process.
	cmd := exec.Command("kill", "-9", strconv.Itoa(b.pid))
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("kill chrome process: %v", err)
	}

	return nil
}

func (b *Browser) Start(ctx context.Context) error {
	// Execute the following command to start Chrome with the default arguments:
	var startArgs []string = []string{"--disable-notifications", "--kiosk"}
	var chromeBinary string = "google-chrome"

	// If we're on macOS, use the default Chrome.app.
	if goruntime.GOOS == "darwin" {
		chromeBinary = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}

	// If the environment variable CHROME_BINARY is set, use that instead.
	if os.Getenv("CHROME_BINARY") != "" {
		chromeBinary = os.Getenv("CHROME_BINARY")
	}

	// Make temp directory
	tmpDirFlag := "--user-data-dir=" + os.TempDir()

	// Reserve port
	if _port == 0 {
		_port = 9000
	} else {
		return fmt.Errorf("port is reserved")
	}

	debuggingPortFlag := "--remote-debugging-port=" + strconv.Itoa(int(_port))

	// Add the dynamic flags
	startArgs = append(startArgs, tmpDirFlag, debuggingPortFlag)

	// Start Chrome.
	cmd := exec.Command(chromeBinary, startArgs...)
	err := cmd.Start()
	if err != nil {
		return err
	}

	// Set pid
	b.pid = cmd.Process.Pid

	// Wait for Chrome to start.
	time.Sleep(2 * time.Second)

	// Connect to Chrome.
	devt := devtool.New("http://localhost:" + strconv.Itoa(int(_port)))
	pageTarget, err := devt.Get(ctx, devtool.Page)
	if err != nil {
		return err
	}
	//b.conn, err = rpcc.DialContext(_ctx, pageTarget.WebSocketDebuggerURL, rpcc.WithWriteBufferSize(10485760))
	b.conn, err = rpcc.DialContext(ctx, pageTarget.WebSocketDebuggerURL)
	if err != nil {
		return err
	}

	// Create a new cdp.Client.
	b.c = cdp.NewClient(b.conn)

	// Enable the Page domain.
	err = b.c.Page.Enable(ctx)
	if err != nil {
		return err
	}

	// Enable the Runtime domain.
	err = b.c.Runtime.Enable(ctx)
	if err != nil {
		return err
	}

	// Navigate to about:blank to remove the bookmarks bar from window and screen size.
	err = b.Navigate(ctx, "about:blank")
	if err != nil {
		return err
	}

	b.setupCoords(ctx)

	return nil
}

// Navigate navigates to the given URL.
func (b *Browser) Navigate(ctx context.Context, url string) error {
	// Navigate to the page, block until ready.
	loadEventFired, err := b.c.Page.LoadEventFired(ctx)
	if err != nil {
		return err
	}

	_, err = b.c.Page.Navigate(ctx, page.NewNavigateArgs(url))
	if err != nil {
		return err
	}

	_, err = loadEventFired.Recv()
	if err != nil {
		return err
	}
	loadEventFired.Close()

	return nil
}

// Evaluate evaluates the given JavaScript expression.
func (b *Browser) Evaluate(ctx context.Context, exp string) (string, error) {
	// Evaluate the expression.
	res, err := b.c.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(exp))
	if err != nil {
		return "", err
	}

	// Unmarshal the result.
	var s string
	err = json.Unmarshal(res.Result.Value, &s)
	if err != nil {
		return "", err
	}

	return s, nil
}

type window struct {
	Top    float64
	Bottom float64
	Left   float64
	Right  float64
}

// domRect is a struct representing a domRect.
type domRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// size is a struct representing the viewport size.
type size struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

func (b *Browser) setupCoords(ctx context.Context) error {
	w, err := b.getWindowSize(ctx)
	if err != nil {
		return fmt.Errorf("get window size: %w", err)
	}

	s, err := b.getScreenSize(ctx)
	if err != nil {
		return fmt.Errorf("get screen size: %w", err)
	}

	b.w.Top = s.Height - w.Height
	b.w.Bottom = s.Height
	b.w.Left = 0
	b.w.Right = s.Width

	return nil
}

func (b *Browser) Select(ctx context.Context, selector string) (dom.NodeID, error) {
	root, err := b.c.DOM.GetDocument(ctx, dom.NewGetDocumentArgs().SetDepth(-1))
	if err != nil {
		return 0, err
	}

	node, err := b.c.DOM.QuerySelector(ctx, dom.NewQuerySelectorArgs(root.Root.NodeID, selector))
	if err != nil {
		return 0, err
	}

	return node.NodeID, nil
}

// getWindowSize returns the window size.
func (b *Browser) getWindowSize(ctx context.Context) (m size, err error) {
	s, err := b.Evaluate(ctx, `JSON.stringify({width: window.innerWidth, height: window.innerHeight});`)
	if err != nil {
		return m, err
	}

	// Unmarshal the result into a WindowSize struct.
	err = json.Unmarshal([]byte(s), &m)
	if err != nil {
		return m, err
	}

	return m, nil
}

// getScreenSize returns the screen size.
func (b *Browser) getScreenSize(ctx context.Context) (m size, err error) {
	s, err := b.Evaluate(ctx, `JSON.stringify({width: window.screen.width, height: window.screen.height});`)
	if err != nil {
		return m, err
	}

	// Unmarshal the result into a ScreenSize struct.
	err = json.Unmarshal([]byte(s), &m)
	if err != nil {
		return m, err
	}

	return m, nil
}

// getCoords returns the x and y coordinates of the given DOMRect.
func (b *Browser) getCoords(rect domRect) (x, y float64) {
	return rect.X + (rect.Width / 2), rect.Y + (rect.Height / 2) + b.w.Top
}

func (b *Browser) scrollTo(ctx context.Context, id dom.NodeID) error {
	scrollArgs := dom.NewScrollIntoViewIfNeededArgs().SetNodeID(id)
	err := b.c.DOM.ScrollIntoViewIfNeeded(ctx, scrollArgs)
	if err != nil {
		return fmt.Errorf("scroll into view: %w", err)
	}

	return nil
}

func (b *Browser) Click(ctx context.Context, id dom.NodeID) error {
	err := b.scrollTo(ctx, id)
	if err != nil {
		return fmt.Errorf("scroll to: %w", err)
	}

	boxArgs := dom.NewGetBoxModelArgs().SetNodeID(id)
	box, err := b.c.DOM.GetBoxModel(ctx, boxArgs)
	if err != nil {
		return fmt.Errorf("get box model: %w", err)
	}

	x, y := b.getCoords(quadToDOMRect(box.Model.Border))
	clickArgs := input.NewDispatchMouseEventArgs("mousePressed", x, y).
		SetButton("left").
		SetClickCount(1)
	err = b.c.Input.DispatchMouseEvent(ctx, clickArgs)
	if err != nil {
		return fmt.Errorf("mouse down: %w", err)
	}

	clickArgs.Type = "mouseReleased"
	err = b.c.Input.DispatchMouseEvent(ctx, clickArgs)
	if err != nil {
		return fmt.Errorf("mouse up: %w", err)
	}

	return nil
}

func quadToDOMRect(q dom.Quad) domRect {
	return domRect{
		X:      q[0],
		Y:      q[1],
		Width:  q[2] - q[0],
		Height: q[5] - q[1],
	}
}

func (b *Browser) Screenshot(ctx context.Context) ([]byte, error) {
	screenshotArgs := page.NewCaptureScreenshotArgs().SetFormat("png")
	screenshot, err := b.c.Page.CaptureScreenshot(ctx, screenshotArgs)
	if err != nil {
		return nil, err
	}

	return screenshot.Data, nil
}

func (b *Browser) Text(ctx context.Context, s string) error {
	args := input.NewInsertTextArgs(s)
	err := b.c.Input.InsertText(ctx, args)
	if err != nil {
		return fmt.Errorf("insert text: %w", err)
	}

	return nil
}

func (b *Browser) File(ctx context.Context, id dom.NodeID, paths []string) error {
	err := b.scrollTo(ctx, id)
	if err != nil {
		return fmt.Errorf("scroll to: %w", err)
	}

	args := dom.NewSetFileInputFilesArgs(paths).SetNodeID(id)
	err = b.c.DOM.SetFileInputFiles(ctx, args)
	if err != nil {
		return fmt.Errorf("set file input files: %w", err)
	}

	return nil
}
