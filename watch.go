// Copyright 2016 HenryLee. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/henrylee2cn/fay/fsnotify"
	"github.com/henrylee2cn/faygo"
)

var (
	cmd          *exec.Cmd
	state        sync.Mutex
	eventTime    = make(map[string]int64)
	scheduleTime time.Time
)
var started = make(chan bool)

func newWatcher() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		faygo.Errorf("[fay] Fail to create new Watcher[ %s ]", err)
		os.Exit(2)
	}

	go func() {
		for {
			select {
			case e := <-watcher.Event:
				isbuild := true

				// Skip TMP files for Sublime Text.
				if checkTMPFile(e.Name) {
					continue
				}
				if !checkIfWatchExt(e.Name) {
					continue
				}

				mt := getFileModTime(e.Name)
				if t := eventTime[e.Name]; mt == t {
					faygo.Printf("[fay] # %s #", e.String())
					isbuild = false
				}

				eventTime[e.Name] = mt

				if isbuild {
					faygo.Printf("%s", e)
					go func() {
						// Wait 1s before autobuild util there is no file change.
						scheduleTime = time.Now().Add(1 * time.Second)
						for {
							time.Sleep(scheduleTime.Sub(time.Now()))
							if time.Now().After(scheduleTime) {
								break
							}
							return
						}

						autobuild()
					}()
				}
			case err := <-watcher.Error:
				faygo.Warningf("[fay] %s", err.Error()) // No need to exit here
			}
		}
	}()

	faygo.Printf("[fay] Initializing watcher...")
	var paths []string
	readAppDirectories(crupath, &paths)
	for _, path := range paths {
		faygo.Printf("[fay] Directory( %s )", path)
		err = watcher.Watch(path)
		if err != nil {
			faygo.Errorf("[fay] Fail to watch crupathectory[ %s ]", err)
			os.Exit(2)
		}
	}
}

// getFileModTime retuens unix timestamp of `os.File.ModTime` by given path.
func getFileModTime(path string) int64 {
	path = strings.Replace(path, "\\", "/", -1)
	f, err := os.Open(path)
	if err != nil {
		faygo.Errorf("[fay] Fail to open file[ %s ]", err)
		return time.Now().Unix()
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		faygo.Errorf("[fay] Fail to get file information[ %s ]", err)
		return time.Now().Unix()
	}

	return fi.ModTime().Unix()
}

func autobuild() {
	state.Lock()
	defer state.Unlock()
	faygo.Printf("[fay] Start build...")
	appName := appname
	if runtime.GOOS == "windows" {
		appName += ".exe"
	}
	n := strings.LastIndex(crupath, "/src/")
	if n == -1 {
		faygo.Fatalf("[fay] The project is not under src, can not run: %s", crupath)
	}
	cmd := exec.Command("go", "build", "-o", appName)
	cmd.Env = append([]string{"GOPATH=" + crupath[:n]}, os.Environ()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		faygo.Errorf("[fay] ============== Build failed ===================")
		return
	}
	faygo.Printf("[fay] Build was successful")
	Restart(appname)
}

func Kill() {
	defer func() {
		if e := recover(); e != nil {
			fmt.Println("[fay] Kill.recover -> ", e)
		}
	}()
	if cmd != nil && cmd.Process != nil {
		err := cmd.Process.Kill()
		if err != nil {
			fmt.Println("[fay] Kill -> ", err)
		}
	}
}

func Restart(appname string) {
	faygo.Printf("[fay] Kill running process")
	Kill()
	go Start(appname)
}

func Start(appname string) {
	faygo.Printf("[fay] Restarting %s...", appname)
	if strings.Index(appname, "./") == -1 {
		appname = "./" + appname
	}

	cmd = exec.Command(appname)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	go cmd.Run()
	faygo.Printf("[fay] %s is running...", appname)
	started <- true
}

// checkTMPFile returns true if the event was for TMP files.
func checkTMPFile(name string) bool {
	if strings.HasSuffix(strings.ToLower(name), ".tmp") {
		return true
	}
	return false
}

var watchExts = []string{".go"}

// checkIfWatchExt returns true if the name HasSuffix <watch_ext>.
func checkIfWatchExt(name string) bool {
	for _, s := range watchExts {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

func readAppDirectories(crupathectory string, paths *[]string) {
	fileInfos, err := ioutil.ReadDir(crupathectory)
	if err != nil {
		return
	}

	useDirectory := false
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() == true && fileInfo.Name()[0] != '.' {
			readAppDirectories(crupathectory+fileInfo.Name(), paths)
			continue
		}

		if useDirectory == true {
			continue
		}

		if filepath.Ext(fileInfo.Name()) == ".go" {
			*paths = append(*paths, crupathectory)
			useDirectory = true
		}
	}

	return
}
