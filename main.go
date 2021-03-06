package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/caseymrm/menuet"
	"github.com/ncruces/zenity"
)

// timerFinishedAudioFile specifies the audio file which is played once the timer is finished.
//
// This file should be in the same directory as this executable.
//
// The file is played by invoking 'ffplay'.
const timerFinishedAudioFile = "you-can-heal.mp3"

const (
	secondsInMinute = 60
	timeStep        = time.Millisecond * 999

	// timerFile is used to display the timers inside Emacs.
	timerFile = "/Users/Gira/.tim/timers.org"
)

// caffeinatePID is apparently required because one can't pass arguments to properQuitMenuItem().
var caffeinatePID = 0

// lastCurrentTime is used to only write to timerFile when the display has changed.
var lastCurrentTime = ""
var informedEmacsOnLaunch = false

type countdown struct {
	minutes int
	seconds int
}

func totalSecondsToString(totalSeconds int) string {
	in := nearestDisplayFine(totalSeconds)
	m := in / secondsInMinute
	s := in % secondsInMinute

	out := fmt.Sprintf("%d%0.2d", m, s)
	if m > 9 {
		mm := nearestFineDown(m)
		if mm <= 9 {
			mm = 12
		}
		return fmt.Sprintf("%d°", mm)
	}
	if len(out) < 3 {
		return "0" + out
	}
	return out
}

func toString(minutes, seconds int) string {
	return totalSecondsToString(minutes*secondsInMinute + seconds)
}

func (c countdown) isOverTime() bool {
	return c.minutes <= 0 && c.seconds <= 0
}

func (c *countdown) flipForOverTime() {
	if c.minutes < 0 {
		c.minutes = -c.minutes
	}

	if c.seconds < 0 {
		c.seconds = -c.seconds
	}
}

func sumDigits(number int) int {
	sumResult := 0
	for number != 0 {
		remainder := number % 10
		sumResult += remainder
		number = number / 10
	}
	if sumResult > 9 {
		return sumDigits(sumResult)
	}
	return sumResult
}

func nearestDisplayFine(totalSeconds int) int {
	current := totalSeconds
	for {
		m := current / secondsInMinute
		s := current % secondsInMinute

		test := sumDigits(m) + sumDigits(s)
		if isFine(test) {
			return m*secondsInMinute + s
		}

		current += 1
	}
}

func nearestFineDown(inp int) int {
	curr := inp
	for !isFine(curr) {
		curr -= 1
	}
	return curr
}

func isFine(inp int) bool {
	return inp%3 == 0
}

func getRemainingTime(endTime time.Time) countdown {
	now := time.Now()
	difference := endTime.Sub(now)

	total := int64(difference.Seconds())
	minutes := total / secondsInMinute
	seconds := total % secondsInMinute

	return countdown{
		minutes: int(minutes),
		seconds: int(seconds),
	}
}

func properQuitMenuItem() []menuet.MenuItem {
	return []menuet.MenuItem{
		{
			Text: "Proper Quit",
			Clicked: func() {
				exitAndKillCaffeinate(0)
			},
		},
	}
}

func getNewTimersString(current string, pid int, currentTime string, writeOwnPid bool) string {
	output := ""
	pidS := strconv.Itoa(pid)
	lines := strings.Split(strings.TrimSpace(current), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, pidS+" ") {
			linePid := strings.Split(line, " ")[0]
			if isPidRunning(linePid) {
				output += line + "\n"
			}
		}
	}
	if writeOwnPid {
		output += pidS + " " + currentTime
	}
	return strings.TrimSpace(output)
}

func isPidRunning(pid string) bool {
	cmd := exec.Command("ps", "-p", pid)
	err := cmd.Run()
	return err == nil
}

// writeToTimersFile fills timerFile with this layout:
//
// {pid} {time-to-display}
// {pid} {time-to-display}
// ...
//
// Every PID can only appear once in the file.
func writeToTimersFile(currentTime string, writeOwnPid bool) {
	data, err := ioutil.ReadFile(timerFile)
	if err != nil {
		panic(err)
	}

	initial := string(data)
	pid := os.Getpid()

	f, err := os.OpenFile(timerFile, os.O_WRONLY|os.O_TRUNC, 0x666)
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(getNewTimersString(initial, pid, currentTime, writeOwnPid))
	if err != nil {
		err2 := f.Close()
		if err2 != nil {
			panic(err2)
		}
		panic(err)
	}
	err = f.Close()
	if err != nil {
		panic(err)
	}

	// if writeOwnPid is false, the tim process is always about to quit
	if !writeOwnPid {
		informEmacsToCheckModeLine()
	} else {
		if !informedEmacsOnLaunch {
			informEmacsToCheckModeLine()
			informedEmacsOnLaunch = true
		}
	}
}

func countDown(startTime time.Time, timerName string, totalCount int) {
	menuet.App().Label = fmt.Sprintf("%d", caffeinatePID)
	menuet.App().Children = properQuitMenuItem

	countDown := time.Duration(totalCount) * time.Second
	doneOn := startTime.Add(countDown)

	isOverTime := false

	for {
		remaining := getRemainingTime(doneOn)
		menuString := ""

		if isOverTime {
			remaining.flipForOverTime()
			str := toString(remaining.minutes, remaining.seconds)

			if remaining.seconds >= 1 {
				menuString = "-" + str
			} else {
				// to not display a minus for the zero time
				menuString = str
			}
		} else {
			str := toString(remaining.minutes, remaining.seconds)
			menuString = str
		}

		title := menuString
		if timerName != "" {
			title = timerName + " " + title
		}

		if lastCurrentTime != title {
			writeToTimersFile(title, true)
		}
		lastCurrentTime = title

		menuet.App().SetMenuState(&menuet.MenuState{
			Title: title,
		})

		if remaining.isOverTime() && !isOverTime {
			isOverTime = true

			go timerIsUp(totalCount)
		}

		time.Sleep(timeStep)
	}
}

func playFinishedSound() {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}

	path := filepath.Dir(exe)

	// #nosec
	err = exec.Command("afplay", path+"/"+timerFinishedAudioFile).Run()
	if err != nil {
		panic(err)
	}
}

func timerIsUp(totalCount int) {
	killCaffeinate()

	forHuman := totalSecondsToString(totalCount)
	text := fmt.Sprintf("%s passed.", forHuman)

	err := zenity.Notify(text,
		zenity.Title("Timer is finished"),
		zenity.Icon(zenity.InfoIcon))
	if err != nil {
		panic(err)
	}

	go playFinishedSound()

	_, err = zenity.Info(text,
		zenity.Title("Timer is finished"),
		zenity.Icon(zenity.InfoIcon))
	if err != nil {
		panic(err)
	}

	writeToTimersFile("", false)
	os.Exit(0)
}

func safeAtoi(s string) int {
	if s == "" {
		return 0
	}

	parsed, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}

	return parsed
}

func parseStringCountToSeconds(s string) int {
	if strings.Contains(s, ",") {
		const inMinutesSecondsFormat = 2

		parts := strings.Split(s, ",")

		switch len(parts) {
		case inMinutesSecondsFormat:
			return nearestDisplayFine(safeAtoi(parts[0])*secondsInMinute + safeAtoi(parts[1]))
		}
	} else {
		// just minutes
		return nearestDisplayFine(safeAtoi(s) * secondsInMinute)
	}

	println(fmt.Sprintf("Problematic time format: %s\n", s))
	printUsage()
	os.Exit(1)

	// the return value here is really not important
	return -1
}

func printUsage() {
	println("Usage:\n" +
		"  countdown {time option} {optional timer name}\n\n" +
		"Valid time options are:\n" +
		" ,15      (15 seconds)\n" +
		"  30      (30 minutes)\n" +
		"  30,45   (30 minutes and 45 seconds)")
}

// waitForStdinToQuit queries stdin for an Enter to abort the program.
//
// Using a signal notifier like: signal.Notify(c, os.Interrupt, syscall.SIGTERM)
// causes an internal crash with the menu bar C code and is not fixable.
func waitForStdinToQuit(startTime time.Time, totalSeconds int) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Hit Enter to cancel > ")

	_, err := reader.ReadString('\n')
	if err != nil {
		panic(err)
	}

	doneOn := startTime.Add(time.Second * time.Duration(totalSeconds))
	remaining := getRemainingTime(doneOn)

	if remaining.isOverTime() {
		remaining.flipForOverTime()
		str := toString(remaining.minutes, remaining.seconds)
		fmt.Printf("\n%s over time...\n", str)
	} else {
		str := toString(remaining.minutes, remaining.seconds)
		fmt.Printf("\n%s left...\n", str)
	}

	exitAndKillCaffeinate(0)
}

func killCaffeinate() {
	// #nosec
	cmd := exec.Command("kill", strconv.Itoa(caffeinatePID))

	if err := cmd.Start(); err != nil {
		panic(err)
	}

	// we do not check for errors here because the timer might have already been killed
	_ = cmd.Wait()
}

func informEmacsToCheckModeLine() {
	_ = exec.Command("timeout", "0.06", "emacsclient", "--eval", "(tim-mode-line-check)").Run()
}

func exitAndKillCaffeinate(exitCode int) {
	writeToTimersFile("", false)
	killCaffeinate()
	os.Exit(exitCode)
}

// preventSystemSleepViaCaffeinate makes sure that the system does not
// idle sleep to keep the timer running correctly.
//
// This still allows the display to turn off.
func preventSystemSleepViaCaffeinate() {
	cmd := exec.Command("caffeinate", "-i")

	err := cmd.Start()
	if err != nil {
		panic(err)
	}

	pid := cmd.Process.Pid

	go func() {
		// when the timer is up, the caffeinate process is killed, so we do not check for errors here
		_ = cmd.Wait()
	}()

	caffeinatePID = pid
}

func main() {
	count := ""

	const hasArg = 2
	if len(os.Args) >= hasArg {
		count = os.Args[1]
	} else {
		printUsage()
		os.Exit(1)
	}

	startTime := time.Now()
	countInSeconds := parseStringCountToSeconds(count)

	preventSystemSleepViaCaffeinate()

	go waitForStdinToQuit(startTime, countInSeconds)

	timerName := ""

	const hasTimerName = 3
	if len(os.Args) >= hasTimerName {
		timerName = os.Args[2]
	}

	go countDown(startTime, timerName, countInSeconds)

	menuet.App().RunApplication()
}
