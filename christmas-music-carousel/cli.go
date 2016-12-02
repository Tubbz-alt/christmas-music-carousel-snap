package main

import (
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const (
	mainPort   = "14:0"
	maxRestart = 5
)

type serviceFn func(port string, ready chan<- interface{}, quit <-chan interface{}) error

func main() {

	// alsa operations
	// bindmount in current snap namespace /usr/share and /usr/lib directory for alsa conf and plugin not being relocatable
	// TODO: extract in a function returning err
	if os.Getenv("SNAP") != "" {
		if os.Getenv("SUDO_UID") == "" {
			Error.Println("This program needs to run as root, under sudo to get access to alsa from the snap")
			os.Exit(1)
		}
		if err := syscall.Mount("/var/lib/snapd/hostfs/usr/lib", "/usr/lib", "", syscall.MS_BIND, ""); err == nil {
			Error.Printf("Couldn't mount alsa directory: %v", err)
			os.Exit(1)
		}
		if err := syscall.Mount("/var/lib/snapd/hostfs/usr/share", "/usr/share", "", syscall.MS_BIND, ""); err == nil {
			Error.Printf("Couldn't mount alsa directory: %v", err)
			os.Exit(1)
		}
		// TODO: Drop priviledges?
	}

	musics := []string{"Foo", "Bar", "Baz", "Tralala"}

	wg := &sync.WaitGroup{}
	rc := 0
	quit := make(chan interface{})

	// handle Ctrl + Ctrl properly
	userstop := make(chan os.Signal)
	signal.Notify(userstop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-userstop:
			Debug.Printf("Exit requested")
			close(quit)
		}
	}()

	// run listen to music event client
	pgready, epg := keepservicealive(func1, "Piglow Connector", mainPort, wg, quit)

	// run timidity

	// connect timitidy to main input

	// grab musics to play and shuffle them

	// run aplaymidi forever in a loop
	eplayer := playforever(mainPort, musics, wg, quit)

	Debug.Println("All services started")
mainloop:
	for {
		select {
		case err := <-etimidity:
			Error.Printf("Fatal error in midi timidity backend player: %v\n", err)
			rc = 1
			close(quit)
			break mainloop
		case err := <-eplayer:
			if err != nil {
				Error.Printf("Fatal error in midi player: %v\n", err)
				rc = 1
			}
			close(quit)
			break mainloop
		case <-quit:
			break mainloop
		}
	}

	wg.Wait()
	os.Exit(rc)
}

func keepservicealive(f serviceFn, name string, port string, wg *sync.WaitGroup, quit <-chan interface{}) (<-chan interface{}, <-chan error) {
	err := make(chan error)
	ready := make(chan interface{})
	Debug.Printf("Starting %s", name)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(err)

		n := 0
		for {
			start := time.Now()
			e := f(port, ready, quit)
			end := time.Now()

			// check for quitting request
			select {
			case <-quit:
				Debug.Printf("Quit %s watcher as requested", name)
				return
			default:
				if n > maxRestart-1 {
					Debug.Printf("We did fail starting %s many times, returning an error", name)
					// send a ready signal in case we never sent it on startup. We are the only goroutine accessing it
					// so it's safe to check if closed
					if _, active := <-ready; active {
						close(ready)
					}
					err <- e
					return
				}
			}

			if end.Sub(start) < time.Duration(10*time.Second) {
				n++
				Debug.Printf("%s failed to start, increasing number of restarts: %d.", name, n)
			} else {
				n = 0
				Debug.Printf("%s failed, but not immediately, considering as first restart.", name)
			}
		}
	}()
	return ready, err
}

func func1(port string, ready chan<- interface{}, quit <-chan interface{}) error {
	cmd := exec.Command("sleep", "30")
	// prevent Ctrl + C and other signals to get sent
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	err := cmd.Start()
	if err != nil {
		return err
	}

	// killer goroutine
	done := make(chan interface{})
	defer close(done)
	go func() {
		select {
		case <-quit:
			Debug.Println("Forcing func1 to stop")
			cmd.Process.Kill()
		case <-done:
		}
	}()

	return cmd.Wait()
}
