// This file is part of Ringot.
/*
Copyright 2016 tSU-RooT <tsu.root@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"errors"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

func byteSliceRemove(bytes []byte, from int, to int) []byte {
	copy(bytes[from:], bytes[to:])
	return bytes[:len(bytes)+from-to]
}

func byteSliceInsert(dst []byte, src []byte, pos int) []byte {
	length := len(dst) + len(src)
	if cap(dst) < length {
		s := make([]byte, len(dst), length)
		copy(s, dst)
		dst = s
	}
	dst = dst[:length]
	copy(dst[pos+len(src):], dst[pos:])
	copy(dst[pos:], src)
	return dst

}

func centeringStr(str string, width int) string {
	sub := width - len(str)
	if sub <= 0 {
		return str
	}
	val := ""
	if sub%2 == 0 {
		for i := 0; i < (sub / 2); i++ {
			val += " "
		}
	} else {
		for i := 0; i < (sub/2)+1; i++ {
			val += " "
		}
	}
	val += str

	for i := 0; i < (sub / 2); i++ {
		val += " "
	}
	return val
}

func drawText(str string, x int, y int, fg termbox.Attribute, bg termbox.Attribute) {
	i := 0
	for _, c := range str {
		termbox.SetCell(x+i, y, c, fg, bg)
		i += runewidth.RuneWidth(c)
	}
}

func drawTextWithAutoNotice(str string, x int, y int, fg termbox.Attribute, bg termbox.Attribute) {
	pos := 0
	foreColor := fg
	backColor := bg
	fgChanging := false
	bgChanging := false
	t := []byte(str)
	for {
		if len(t) == 0 {
			break
		}
		c, s := utf8.DecodeRune(t)
		if !(bgChanging || fgChanging) && len(t) > s {
			if c == '@' {
				tc, _ := utf8.DecodeRune(t[s:])
				if isScreenNameUsable(tc) {
					backColor = ColorLowlight
					bgChanging = true
				}
			} else {
				found := false
				s2 := 0
				if c == ' ' {
					var tc rune
					tc, s2 = utf8.DecodeRune(t[s:])
					if tc == '#' {
						found = true
					}
				} else if c == '#' && pos == 0 {
					found = true
				}
				if found {
					tc, _ := utf8.DecodeRune(t[s+s2:])
					if tc != ' ' {
						foreColor = ColorBlue
						fgChanging = true
					}
				}
			}
		} else {
			if bgChanging && !isScreenNameUsable(c) {
				backColor = bg
				bgChanging = false
			} else if fgChanging && c == ' ' {
				tc, _ := utf8.DecodeRune(t[s:])
				if tc != '#' {
					foreColor = fg
					fgChanging = false
				}
			}
		}

		termbox.SetCell(x+pos, y, c, foreColor, backColor)
		pos += runewidth.RuneWidth(c)
		t = t[s:]
	}
}

func isScreenNameUsable(r rune) bool {
	if r >= 'a' && r <= 'z' {
		return true
	} else if r >= 'A' && r <= 'Z' {
		return true
	} else if r >= '0' && r <= '9' {
		return true
	} else if r == '_' {
		return true
	}
	return false
}

func isScreenNameUsableStr(s string) bool {
	for _, r := range s {
		if !isScreenNameUsable(r) {
			return false
		}
	}
	return true
}

func fillLine(offset int, y int, bg termbox.Attribute) {
	width, _ := getTermSize()
	x := offset
	for {
		if x >= width {
			break
		}
		termbox.SetCell(x, y, ' ', ColorBackground, bg)
		x++
	}
}

func generateLabelColorByUserID(id int64) termbox.Attribute {
	if val, ok := LabelColorMap[id]; ok {
		return LabelColors[val]
	}

	rand.Seed(id)
	val := rand.Intn(len(LabelColors))
	LabelColorMap[id] = val
	return LabelColors[val]
}

var (
	replacer = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">")
)

func wrapTweets(tweets []anaconda.Tweet) []tweetstatus {
	result := make([]tweetstatus, len(tweets))
	for i := 0; i < len(tweets); i++ {
		tweet := &tweets[i]
		for {
			if tweet.RetweetedStatus != nil {
				tweet = tweet.RetweetedStatus
			} else {
				break
			}
		}
		tweet.Text = replacer.Replace(tweet.Text)
		for _, url := range tweet.Entities.Urls {
			tweet.Text = strings.Replace(tweet.Text, url.Url, url.Display_url, -1)
		}
		for _, media := range tweet.ExtendedEntities.Media {
			tweet.Text = strings.Replace(tweet.Text, media.Url, media.Display_url, -1)
		}
		result[i] = tweetstatus{Content: &tweets[i]}
	}
	return result
}

func wrapTweet(t *anaconda.Tweet) tweetstatus {
	tweet := t
	for {
		if tweet.RetweetedStatus != nil {
			tweet = tweet.RetweetedStatus
		} else {
			break
		}
	}
	tweet.Text = replacer.Replace(tweet.Text)
	for _, url := range tweet.Entities.Urls {
		tweet.Text = strings.Replace(tweet.Text, url.Url, url.Display_url, -1)
	}
	for _, media := range tweet.ExtendedEntities.Media {
		tweet.Text = strings.Replace(tweet.Text, media.Url, media.Display_url, -1)
	}
	return tweetstatus{Content: t}
}

func sumTweetLines(tweetsStatusSlice []tweetstatus) int {
	sum := 0
	tweets := tweetsStatusSlice
	for _, t := range tweets {
		sum += t.countLines()
	}
	return sum
}

func openCommand(path string) {
	var commandName string
	switch runtime.GOOS {
	case "linux":
		commandName = "xdg-open"
	case "darwin":
		commandName = "open"
	default:
		return

	}
	exec.Command(commandName, path).Run()
}

const (
	tempDir = "ringot"
)

func downloadMedia(url string, filefullpath string, wg *sync.WaitGroup, mes chan int) {
	defer func() {
		if err := recover(); err != nil {
			mes <- -1
		} else {
			mes <- 1
		}
		wg.Done()
	}()
	if _, err := os.Stat(filefullpath); err == nil {
		return
	}

	res, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	if res.StatusCode != 200 {
		panic(errors.New(res.Status))
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	tempdir := filepath.Join(os.TempDir(), tempDir)
	if _, err := os.Stat(tempdir); err != nil {
		err := os.Mkdir(tempdir, 0775)
		if err != nil {
			panic(err)
		}
	}
	file, err := os.OpenFile(filefullpath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0664)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	file.Write(body)
}

func openMedia(urls []string) {
	wg := new(sync.WaitGroup)
	mes := make(chan int, len(urls))
	fileps := make([]string, 0)
	changeBufferState(fmt.Sprintf("Downloading...(0/%d)", len(urls)))
	for _, url := range urls {
		_, filename := path.Split(url)
		filefullpath := filepath.Join(os.TempDir(), tempDir, filename)
		fileps = append(fileps, filefullpath)
		wg.Add(1)
		go downloadMedia(url, filefullpath, wg, mes)
	}
	receiveErrors := 0
	go func() {
		count := 0
		for {
			rec := <-mes
			if rec == -1 {
				receiveErrors++
				continue
			}
			count++
			changeBufferState(fmt.Sprintf("Downloading...(%d/%d)", count, len(urls)))
			if count == len(urls) {
				break
			}
		}
	}()
	wg.Wait()
	// Reverse
	for i := len(fileps) - 1; i >= 0; i-- {
		if _, err := os.Stat(fileps[i]); err == nil {
			openCommand(fileps[i])
		}
		time.Sleep(time.Millisecond)
	}

	if receiveErrors > 0 {
		if receiveErrors == 1 {
			changeBufferState("Err:media downloading was failed")
		} else if receiveErrors == len(urls) {
			changeBufferState("Err:all of media downloading were failed")
		} else {
			changeBufferState("Err:some of media downloading were failed")
		}
	} else {
		changeBufferState("")
	}
}

func favoriteTweet(id int64) {
	_, err := api.Favorite(id)
	if err != nil {
		changeBufferState("Err:Favorite")
		return
	}
}

func unfavoriteTweet(id int64) {
	_, err := api.Unfavorite(id)
	if err != nil {
		changeBufferState("Err:Unfavorite")
		return
	}
}

func retweet(id int64) {
	_, err := api.Retweet(id, false)
	if err != nil {
		changeBufferState("Err:Retweet")
		return
	}
}

func changeBufferState(state string) {
	go func() {
		stateCh <- state
		if state != "" {
			stateClearCh <- 30
		}
	}()
}

func getTermSize() (int, int) {
	return termWidth, termHeight
}

func setTermSize(w, h int) {
	termWidth, termHeight = w, h
}

type lock struct {
	mutex   sync.Mutex
	locking uint32
}

// Errors
var (
	ErrAlreayLocking = errors.New("already locking")
)

func (l *lock) lock() error {
	if atomic.LoadUint32(&l.locking) == 1 {
		return ErrAlreayLocking
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.locking == 0 {
		atomic.StoreUint32(&l.locking, 1)
	}
	return nil
}

func (l *lock) unlock() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.locking == 1 {
		atomic.StoreUint32(&l.locking, 0)
	}
}

func (l *lock) isLocking() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return atomic.LoadUint32(&l.locking) == 1
}
