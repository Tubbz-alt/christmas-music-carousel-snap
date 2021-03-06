package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"
	"syscall"
	"time"
)

// start and connect timidity daemon to port.
func startTimidity(port string, ready chan struct{}, quit <-chan struct{}) error {
	freepatsPath := "/usr/share/midi/freepats"
	if snapdir := os.Getenv("SNAP"); snapdir != "" {
		freepatsPath = path.Join(snapdir, freepatsPath)
	}
	cmd := exec.Command("timidity", "-Os", "-iA", "-c", path.Join(rootdir, "timidity-snap.cfg"),
		"-L", freepatsPath)
	var errbuf bytes.Buffer
	cmd.Stderr = &errbuf
	// prevent Ctrl + C and other signals to get sent
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	e := cmd.Start()
	if e != nil {
		return e
	}

	wg := sync.WaitGroup{}

	// killer goroutine
	done := make(chan bool)
	defer close(done)
	go func() {
		select {
		case <-quit:
			Debug.Println("Forcing timidity to stop")
		case <-done:
		}
		cmd.Process.Kill()
		cmd = nil
	}()

	// we have 2 goroutines which can send to err
	// if we stop the connect goroutine, the timidity .Wait() will try to send there
	err := make(chan error, 1)
	defer close(err)

	// Timitidy process
	wg.Add(1)
	go func() {
		defer wg.Done()
		Debug.Println("Starting timidity")
		e := cmd.Wait()
		err <- fmt.Errorf("%s: %v", errbuf.String(), e)
		Debug.Println("Timidity stopped")
	}()

	// connector goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		Debug.Println("Starting timidity connector")
		connectTimitidy(port, ready, done, err)
		Debug.Println("Timidity connector ended")
	}()

	e = <-err
	// signal to kill timidity if still running
	if cmd != nil {
		done <- true
	}
	wg.Wait()
	return e
}

// connect timidity to port, send a ready signal once connected.
func connectTimitidy(port string, ready chan struct{}, done <-chan bool, err chan<- error) {

	n := 0
	for {
		// get alsa ports
		out, e := exec.Command("aconnect", "-l").CombinedOutput()
		if e != nil {
			if n > 4 {
				err <- errors.New(string(out))
				return
			}
			n++
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// get timidity port
		end := bytes.LastIndex(out, []byte(": 'TiMidity'"))
		if end < 0 {
			if n > 4 {
				err <- errors.New("No TiMitidy alsa port found")
				return
			}
			n++
			time.Sleep(time.Second)
			continue
		}
		start := bytes.LastIndexByte(out[:end], ' ')
		tport := string(out[start+1 : end])

		// connect timitity to main port
		out, e = exec.Command("aconnect", port, tport).CombinedOutput()
		if e != nil {
			if n > 4 {
				err <- errors.New(string(out))
				return
			}
			n++
			time.Sleep(500 * time.Millisecond)
			continue
		}

		Debug.Printf("Signaling timidity is connected")
		// we only signal it once, if timidity fails and restarts, aplaymidi is already reading music
		signalOnce(ready)
		return
	}

}
