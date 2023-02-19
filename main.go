package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/danicat/simpleansi"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

var (
	themeFile = flag.String("theme-file", "theme/emoji.json", "path to custom theme file")
	mazeFile  = flag.String("maze-file", "maze01.txt", "path to custom maze file")
)

type GhostStatus string

const (
	GhostStatusNormal GhostStatus = "normal"
	GhostStatusBlue   GhostStatus = "blue"
)

type sprite struct {
	row      int
	col      int
	startRow int
	startCol int
}

type ghost struct {
	position sprite
	status   GhostStatus
}

type Theme struct {
	Player           string        `json:"player"`
	Ghost            string        `json:"ghost"`
	Wall             string        `json:"wall"`
	Dot              string        `json:"dot"`
	Pill             string        `json:"pill"`
	Death            string        `json:"death"`
	Space            string        `json:"space"`
	UseEmoji         bool          `json:"use_emoji"`
	GhostBlue        string        `json:"ghost_blue"`
	PillDurationSecs time.Duration `json:"pill_duration_secs"`
}

var theme Theme
var maze []string
var player sprite
var ghosts []*ghost
var score int
var numDots int
var lives = 3

func main() {
	flag.Parse()

	// initialise game
	initialise()
	defer cleanup()

	// load resources
	err := loadMaze(*mazeFile)
	if err != nil {
		log.Println("failed to load maze:", err)
		return
	}
	err = loadTheme(*themeFile)
	if err != nil {
		log.Println("failed to load configuration:", err)
		return
	}

	// process input
	inputCh := make(chan string)
	go func(ch chan<- string) {
		for {
			input, err := readInput()
			if err != nil {
				log.Println("error reading input:", err)
				ch <- "ESC"
			}
			ch <- input
		}
	}(inputCh)

	for {
		// update screen
		printScreen()

		// process movement
		select {
		case input := <-inputCh:
			if input == "ESC" {
				lives = 0
			}
			movePlayer(input)
		default:
		}
		moveGhost()

		// 遇到怪则减一条命
		for _, g := range ghosts {
			if g.position.row == player.row && g.position.col == player.col {
				lives--
				if lives != 0 {
					moveCursor(player.row, player.col)
					fmt.Print(theme.Death)
					moveCursor(len(maze)+2, 0)
					time.Sleep(1000 * time.Millisecond) // 重置玩家位置前暂停下
					player.row, player.col = player.startRow, player.startCol
				}
			}
		}

		// check game over
		if numDots == 0 || lives <= 0 {
			if lives <= 0 {
				moveCursor(player.row, player.col)
				fmt.Print(theme.Death)
				moveCursor(len(maze)+2, 0)
			}
			break
		}

		// repeat
		time.Sleep(200 * time.Millisecond)
	}
}

func loadTheme(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	err = decoder.Decode(&theme)
	if err != nil {
		return err
	}

	return nil
}

// -----------------------------------
// - # represents a wall
// - . represents a dot
// - P represents the player
// - G represents the ghosts (enemies)
// - X represents the power up pills
// -----------------------------------
func loadMaze(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		maze = append(maze, line)
	}

	for row, line := range maze {
		for col, char := range line {
			switch char {
			case 'P':
				player = sprite{row, col, row, col}
			case 'G':
				ghosts = append(ghosts, &ghost{sprite{row, col, row, col}, GhostStatusNormal})
			case '.':
				numDots++
			}
		}
	}
	return nil
}

func moveCursor(row, col int) {
	if theme.UseEmoji {
		simpleansi.MoveCursor(row, col*2)
	} else {
		simpleansi.MoveCursor(row, col)
	}
}

func printScreen() {
	simpleansi.ClearScreen()
	for _, line := range maze {
		for _, char := range line {
			switch char {
			case '#':
				fmt.Print(simpleansi.WithBlueBackground(theme.Wall))
			case '.':
				fmt.Print(theme.Dot)
			case 'X':
				fmt.Print(theme.Pill)
			default:
				fmt.Print(theme.Space)
			}
		}
		fmt.Println()
	}

	// set player
	moveCursor(player.row, player.col)
	fmt.Print(theme.Player)

	// set ghosts
	for _, g := range ghosts {
		moveCursor(g.position.row, g.position.col)
		ghostStatusMux.RLock()
		ghostStatus := g.status
		ghostStatusMux.RUnlock()
		if ghostStatus == GhostStatusNormal {
			fmt.Print(theme.Ghost)
		} else {
			fmt.Print(theme.GhostBlue)
		}
	}

	// print score
	moveCursor(len(maze)+1, 0)
	livesRemaining := strconv.Itoa(lives)
	if theme.UseEmoji {
		// 使用 buffer，只在初始化的时候分配内存，更高效
		// 使用 + 的话，每次循环都要分配内存
		buffer := bytes.Buffer{}
		for i := 0; i < lives; i++ {
			buffer.WriteString(theme.Player)
		}
		livesRemaining = buffer.String()
	}
	fmt.Println("Score:", score, "\tLives:", livesRemaining)
}

func initialise() {
	cbTerm := exec.Command("stty", "cbreak", "-echo")
	cbTerm.Stdin = os.Stdin

	err := cbTerm.Run()
	if err != nil {
		log.Fatalln("failed to active cbreak mode:", err)
	}
}

func cleanup() {
	cookedTerm := exec.Command("stty", "-cbreak", "echo")
	cookedTerm.Stdin = os.Stdin

	err := cookedTerm.Run()
	if err != nil {
		log.Fatalln("failed to restore cooked mode:", err)
	}
}

func readInput() (string, error) {
	buffer := make([]byte, 100)
	cnt, err := os.Stdin.Read(buffer)
	if err != nil {
		return "", err
	}
	if cnt == 1 && buffer[0] == 0x1b { // 0x1b 是 esc 的16进制表示
		return "ESC", nil
	} else if cnt >= 3 {
		if buffer[0] == 0x1b && buffer[1] == '[' {
			switch buffer[2] {
			case 'A':
				return "UP", nil
			case 'B':
				return "DOWN", nil
			case 'C':
				return "RIGHT", nil
			case 'D':
				return "LEFT", nil
			}
		}
	}
	return "", nil
}

func makeMove(oldRow, oldCol int, dir string) (newRow, newCol int) {
	newRow, newCol = oldRow, oldCol
	switch dir {
	case "UP":
		newRow--
		if newRow < 0 {
			newRow = len(maze) - 1
		}
	case "DOWN":
		newRow++
		if newRow == len(maze) {
			newRow = 0
		}
	case "RIGHT":
		newCol++
		if newCol == len(maze[0]) {
			newCol = 0
		}
	case "LEFT":
		newCol--
		if newCol < 0 {
			newCol = len(maze[0]) - 1
		}
	}

	if maze[newRow][newCol] == '#' { // 新坐标如果是墙，则返回老坐标
		newRow, newCol = oldRow, oldCol
	}
	return
}

func movePlayer(dir string) {
	player.row, player.col = makeMove(player.row, player.col, dir)
	removeDot := func(row, col int) {
		maze[row] = maze[row][:col] + " " + maze[row][col+1:]
	}
	switch maze[player.row][player.col] {
	case '.':
		numDots--
		score++
		removeDot(player.row, player.col)
	case 'X':
		score += 10
		removeDot(player.row, player.col)
		go processPill()
	}
}

func moveGhost() {
	for _, g := range ghosts {
		g.position.row, g.position.col = makeMove(g.position.row, g.position.col, drawDirection())
	}
}

func drawDirection() string {
	dir := rand.Intn(4)
	move := map[int]string{
		0: "UP",
		1: "DOWN",
		2: "RIGHT",
		3: "LEFT",
	}
	return move[dir]
}

var pillTimer *time.Timer
var pillMux sync.Mutex

func processPill() {
	pillMux.Lock()
	updateGhosts(ghosts, GhostStatusBlue)
	if pillTimer != nil {
		pillTimer.Stop()
	}
	pillTimer = time.NewTimer(time.Second * theme.PillDurationSecs)
	<-pillTimer.C
	pillTimer.Stop()
	updateGhosts(ghosts, GhostStatusNormal)
	pillMux.Unlock()
}

var ghostStatusMux sync.RWMutex

func updateGhosts(ghosts []*ghost, ghostStatus GhostStatus) {
	ghostStatusMux.Lock()
	defer ghostStatusMux.Unlock()
	for _, g := range ghosts {
		g.status = ghostStatus
	}
}
