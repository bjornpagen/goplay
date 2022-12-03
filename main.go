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
	c, err := chrome.StartChrome(ctx, cancel)
	if err != nil {
		return err
	}

	// Navigate to GitHub.
	err = chrome.Navigate(ctx, c, "https://github.com")
	if err != nil {
		return err
	}

	// Sleep for a random duration between 2 and 4 seconds.
	time.Sleep(time.Duration(2+rand.Intn(2)) * time.Second)

	// Evaluate GitHub's title.
	title, err := chrome.Evaluate(ctx, c, "document.title")
	if err != nil {
		return err
	}

	// Print the title.
	fmt.Printf("Title: %s\n", title)

	// Get the DomRect for the header.
	rect, err := chrome.GetBoundingClientRect(ctx, c, github.CSSSelectors["header"])
	if err != nil {
		return err
	}

	// Get the center of the header.
	x, y, err := chrome.GetIntCoordinates(rect)
	if err != nil {
		return err
	}

	// Move the cursor to the center of the header.
	res := robotgo.MoveSmooth(x, y)
	if !res {
		return fmt.Errorf("failed to move cursor")
	}

	// Move the cursor to the top of the browsing window.
	res = robotgo.MoveSmooth(x, int(chrome.Deadzone))
	if !res {
		return fmt.Errorf("failed to move cursor")
	}

	return nil
}
