package main

import (
	"time"
)

func main() {
	time.Sleep(2 * time.Second) // give host time to reopen the USB serial port
	for i := 0; ; i++ {
		println("hello from TinyGo on M5Paper S3, tick", i)
		time.Sleep(time.Second)
	}
}
