package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/bjornpagen/goplay/chrome"
	"github.com/bjornpagen/goplay/sites/github"

	"github.com/go-vgo/robotgo"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
	// Block for 10 seconds.
	time.Sleep(10 * time.Second)
}

func run() error {
	// Create a new context that is cancelled when Chrome exits.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start Chrome.
	c, err := chrome.New(ctx)
	if err != nil {
		return err
	}

	// Navigate to GitHub.
	err = c.Navigate("https://github.com")
	if err != nil {
		return err
	}

	// Sleep for a random duration between 2 and 4 seconds.
	time.Sleep(time.Duration(2+rand.Intn(2)) * time.Second)

	// Get the DomRect for the header.
	rect, err := c.GetBoundingClientRect(github.CSSSelectors["header"])
	if err != nil {
		return err
	}

	// Get the center of the header.
	x, y, err := c.GetIntCoordinates(rect)
	if err != nil {
		return err
	}

	// Move the cursor to the center of the header.
	res := robotgo.MoveSmooth(x, y)
	if !res {
		return fmt.Errorf("failed to move cursor")
	}

	// Move the cursor to the bottom of the browsing window.
	res = robotgo.MoveSmooth(x, int(c.Bottom))
	if !res {
		return fmt.Errorf("failed to move cursor")
	}

	// Move the cursor to the center of the browsing window.
	res = robotgo.MoveSmooth(int(c.Left+c.Right)/2, int(c.Top+c.Bottom)/2)
	if !res {
		return fmt.Errorf("failed to move cursor")
	}

	return nil
}
