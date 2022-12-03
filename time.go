package main

import (
	"math/rand"
	"time"
)

// sleepJitter sleeps for a random duration between 5 and 10 seconds.
func sleepJitter() {
	time.Sleep(time.Duration(5+rand.Intn(5)) * time.Second)
}
