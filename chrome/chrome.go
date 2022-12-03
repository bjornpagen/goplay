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

var Deadzone float64 = 0

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

// getBoundingClientRect returns an DOMRect struct for the given CSS selector.
func GetBoundingClientRect(ctx context.Context, c *cdp.Client, selector string) (*DOMRect, error) {
	// Get the bounding box of the given selector.
	s, err := Evaluate(ctx, c, fmt.Sprintf(`
		(function() {
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
func GetWindowSize(ctx context.Context, c *cdp.Client) (*WindowSize, error) {
	s, err := Evaluate(ctx, c, `
		(function() {
			return JSON.stringify({
				width: window.innerWidth,
				height: window.innerHeight
			});
		})()
	`)
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
func GetScreenSize(ctx context.Context, c *cdp.Client) (*ScreenSize, error) {
	eval, err := c.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(`
		(function() {
			return JSON.stringify({
				width: window.screen.width,
				height: window.screen.height
			});
		})()
	`))
	if err != nil {
		return nil, err
	}

	var s string
	err = json.Unmarshal(eval.Result.Value, &s)
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
func GetIntCoordinates(rect *DOMRect) (int, int, error) {
	// Check if deadzone is set.
	if Deadzone == 0 {
		return 0, 0, fmt.Errorf("deadzone is not set")
	}
	return int(rect.X + (rect.Width / 2)), int(rect.Y + (rect.Height / 2) + Deadzone), nil
}

// startChrome starts a new Chrome instance and returns a cdp.Client.
func StartChrome(ctx context.Context, cancel context.CancelFunc) (*cdp.Client, error) {
	// Execute the following command to start Chrome with the default arguments:
	// google-chrome --remote-debugging-port=9222 --disable-notifications --kiosk
	var startArgs []string = []string{"--remote-debugging-port=9222", "--disable-notification", "--kiosk"}

	var chromeBinary string = "google-chrome"

	// If we're on macOS, use the default Chrome.app.
	if goruntime.GOOS == "darwin" {
		chromeBinary = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}

	// If the environment variable CHROME_BINARY is set, use that instead.
	if os.Getenv("CHROME_BINARY") != "" {
		chromeBinary = os.Getenv("CHROME_BINARY")
	}

	var cmd = exec.CommandContext(ctx, chromeBinary, startArgs...)
	// Send Chrome stdout and stderr to file descriptor 2 (stderr).
	cmd.Stdout = os.NewFile(2, "/dev/stderr")
	cmd.Stderr = os.NewFile(2, "/dev/stderr")
	// Start Chrome.
	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	// Wait for Chrome to start.
	time.Sleep(2 * time.Second)

	// Connect to Chrome.
	devt := devtool.New("http://localhost:9222")
	pageTarget, err := devt.Get(ctx, devtool.Page)
	if err != nil {
		return nil, err
	}
	conn, err := rpcc.DialContext(ctx, pageTarget.WebSocketDebuggerURL)
	if err != nil {
		return nil, err
	}

	// Create a new cdp.Client.
	c := cdp.NewClient(conn)

	// Enable the Page domain.
	err = c.Page.Enable(ctx)
	if err != nil {
		return nil, err
	}

	// Enable the Runtime domain.
	err = c.Runtime.Enable(ctx)
	if err != nil {
		return nil, err
	}

	// Navigate to about:blank to remove the bookmarks bar from window and screen size.
	err = Navigate(ctx, c, "about:blank")
	if err != nil {
		return nil, err
	}

	// Get the window size.
	w, err := GetWindowSize(ctx, c)
	if err != nil {
		return nil, err
	}

	// Get the screen size.
	s, err := GetScreenSize(ctx, c)
	if err != nil {
		return nil, err
	}

	// Calculate the deadzone at the top of the screen.
	// This is the area where we don't want to move the mouse.
	Deadzone = s.Height - w.Height

	return c, nil
}

// navigate navigates to the given URL.
func Navigate(ctx context.Context, c *cdp.Client, url string) error {
	// Navigate to the page, block until ready.
	loadEventFired, err := c.Page.LoadEventFired(ctx)
	if err != nil {
		return err
	}

	_, err = c.Page.Navigate(ctx, page.NewNavigateArgs(url))
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

func Evaluate(ctx context.Context, c *cdp.Client, exp string) (string, error) {
	// Evaluate the JavaScript expression exp.
	eval, err := c.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(exp))
	if err != nil {
		return "", err
	}

	// Unmarshal the result into a string.
	var result string
	err = json.Unmarshal(eval.Result.Value, &result)
	if err != nil {
		return "", err
	}

	return result, nil
}
