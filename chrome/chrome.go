package chrome

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	goruntime "runtime"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
)

// Browser is a struct that contains all the top level variables.
type Browser struct {
	Top     float64
	Bottom  float64
	Left    float64
	Right   float64
	Client  *cdp.Client
	Context context.Context
}

// DOMRect is a struct representing a DOMRect.
type DOMRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// WindowSize is a struct representing the viewport size.
type WindowSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// ScreenSize is a struct representing the screen size.
type ScreenSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// New creates a new browser instance with the given context.
func New(ctx context.Context) (*Browser, error) {
	b := &Browser{
		Top:     0,
		Bottom:  0,
		Left:    0,
		Right:   0,
		Client:  &cdp.Client{},
		Context: ctx,
	}
	err := b.startChrome()
	if err != nil {
		return nil, err
	}

	return b, nil
}

// startChrome starts a new Chrome instance and returns a cdp.Client.
func (b *Browser) startChrome() error {
	// Execute the following command to start Chrome with the default arguments:
	// google-chrome --remote-debugging-port=9222 --disable-notifications --kiosk
	var startArgs []string = []string{"--remote-debugging-port=9222", "--disable-notifications", "--kiosk"}

	var chromeBinary string = "google-chrome"

	// If we're on macOS, use the default Chrome.app.
	if goruntime.GOOS == "darwin" {
		chromeBinary = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}

	// If the environment variable CHROME_BINARY is set, use that instead.
	if os.Getenv("CHROME_BINARY") != "" {
		chromeBinary = os.Getenv("CHROME_BINARY")
	}

	// Start Chrome with b.Context.
	cmd := exec.CommandContext(b.Context, chromeBinary, startArgs...)
	// Send Chrome stdout and stderr to file descriptor 2 (stderr).
	cmd.Stdout = os.NewFile(2, "/dev/stderr")
	cmd.Stderr = os.NewFile(2, "/dev/stderr")
	// Start Chrome.
	err := cmd.Start()
	if err != nil {
		return err
	}

	// Wait for Chrome to start.
	time.Sleep(2 * time.Second)

	// Connect to Chrome.
	devt := devtool.New("http://localhost:9222")
	pageTarget, err := devt.Get(b.Context, devtool.Page)
	if err != nil {
		return err
	}
	conn, err := rpcc.DialContext(b.Context, pageTarget.WebSocketDebuggerURL)
	if err != nil {
		return err
	}

	// Create a new cdp.Client.
	b.Client = cdp.NewClient(conn)

	// Enable the Page domain.
	err = b.Client.Page.Enable(b.Context)
	if err != nil {
		return err
	}

	// Enable the Runtime domain.
	err = b.Client.Runtime.Enable(b.Context)
	if err != nil {
		return err
	}

	// Navigate to about:blank to remove the bookmarks bar from window and screen size.
	err = b.Navigate("about:blank")
	if err != nil {
		return err
	}

	// Get the window size.
	w, err := b.GetWindowSize()
	if err != nil {
		return err
	}

	// Get the screen size.
	s, err := b.GetScreenSize()
	if err != nil {
		return err
	}

	// Calculate the Top coordinate of the viewport.
	b.Top = s.Height - w.Height

	// Calculate the Bottom coordinate of the viewport.
	b.Bottom = s.Height

	// Calculate the Left coordinate of the viewport.
	b.Left = 0

	// Calculate the Right coordinate of the viewport.
	b.Right = s.Width

	return nil
}

// GetBoundingClientRect returns an DOMRect struct for the given CSS selector.
func (b *Browser) GetBoundingClientRect(selector string) (*DOMRect, error) {
	// Get the bounding box of the given selector.
	s, err := b.Evaluate(fmt.Sprintf(`
	(() => {
		var rect = document.querySelector("%s").getBoundingClientRect();
		return JSON.stringify({
			width: rect.width,
			height: rect.height,
			x: rect.x,
			y: rect.y
		});
	})()
`, selector))
	if err != nil {
		return nil, err
	}

	// Unmarshal the result into a DOMRect struct.
	var rect DOMRect
	err = json.Unmarshal([]byte(s), &rect)
	if err != nil {
		return nil, err
	}

	return &rect, nil
}

// GetWindowSize returns the window size.
func (b *Browser) GetWindowSize() (*WindowSize, error) {
	s, err := b.Evaluate(`JSON.stringify({width: window.innerWidth, height: window.innerHeight});`)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result into a WindowSize struct.
	var size WindowSize
	err = json.Unmarshal([]byte(s), &size)
	if err != nil {
		return nil, err
	}

	return &size, nil
}

// GetScreenSize returns the screen size.
func (b *Browser) GetScreenSize() (*ScreenSize, error) {
	s, err := b.Evaluate(`JSON.stringify({width: window.screen.width, height: window.screen.height});`)
	if err != nil {
		return nil, err
	}

	// Unmarshal the result into a ScreenSize struct.
	var size ScreenSize
	err = json.Unmarshal([]byte(s), &size)
	if err != nil {
		return nil, err
	}

	return &size, nil
}

// GetIntCoordinates returns the x and y coordinates of the given DOMRect.
func (b *Browser) GetIntCoordinates(rect *DOMRect) (int, int, error) {
	x, y := int(rect.X+(rect.Width/2)), int(rect.Y+(rect.Height/2)+b.Top)

	// Check if the y coordinate is between the top and bottom of the screen.
	if y < int(b.Top) || y > int(b.Bottom) {
		return 0, 0, fmt.Errorf("y coordinate is not between the top and bottom of the screen")
	}

	// Check if the x coordinate is between the left and right of the screen.
	if x < int(b.Left) || x > int(b.Right) {
		return 0, 0, fmt.Errorf("x coordinate is not between the left and right of the screen")
	}

	return x, y, nil
}

// Navigate navigates to the given URL.
func (b *Browser) Navigate(url string) error {
	// Navigate to the page, block until ready.
	loadEventFired, err := b.Client.Page.LoadEventFired(b.Context)
	if err != nil {
		return err
	}

	_, err = b.Client.Page.Navigate(b.Context, page.NewNavigateArgs(url))
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
func (b *Browser) Evaluate(exp string) (string, error) {
	// Evaluate the expression.
	res, err := b.Client.Runtime.Evaluate(b.Context, runtime.NewEvaluateArgs(exp))
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
