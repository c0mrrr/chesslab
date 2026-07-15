package main

// engine communication

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// engine struct
type StockfishEngine struct {
	cmd      *exec.Cmd
	stdin    *bufio.Writer
	scanner  *bufio.Scanner
	bestMove chan string
	logCb    func(string)
	movetime int
	depth    int
	mu       sync.Mutex
	done     chan struct{}
}

// make engine
func NewEngine(path string, skillLevel int, threads int, movetime int, depth int, logCb func(string)) (*StockfishEngine, error) {
	// start proc
	cmd := exec.Command(path)
	// stdin
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("fail attach talking pipe: %w", err)
	}
	// stdout
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("fail attach listening pipe: %w", err)
	}
	// run cmd
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fail to wake rock brain: %w", err)
	}
	eng := &StockfishEngine{
		cmd:      cmd,
		stdin:    bufio.NewWriter(stdinPipe),
		scanner:  bufio.NewScanner(stdoutPipe),
		bestMove: make(chan string, 1),
		logCb:    logCb,
		movetime: movetime,
		depth:    depth,
		done:     make(chan struct{}),
	}

	// uci init
	eng.sendCommand("uci")

	// wait uciok lol
	for eng.scanner.Scan() {
		line := eng.scanner.Text()
		if eng.logCb != nil {
			eng.logCb(line)
		}
		if strings.TrimSpace(line) == "uciok" {
			break
		}
	}

	// set skill
	eng.sendCommand(fmt.Sprintf("setoption name Skill Level value %d", skillLevel))

	// set threads
	eng.sendCommand(fmt.Sprintf("setoption name Threads value %d", threads))

	// set hash
	eng.sendCommand("setoption name Hash value 128")

	// wait ready
	eng.sendCommand("isready")
	for eng.scanner.Scan() {
		line := eng.scanner.Text()
		if eng.logCb != nil {
			eng.logCb(line)
		}
		if strings.TrimSpace(line) == "readyok" {
			break
		}
	}

	// read thread lol
	go eng.readLoop()

	return eng, nil
}

// read loop
func (e *StockfishEngine) readLoop() {
	defer close(e.done)
	// scanning output lol
	for e.scanner.Scan() {
		line := e.scanner.Text()

		// log output
		if e.logCb != nil {
			e.logCb(line)
		}

		// look for bestmove
		if strings.HasPrefix(line, "bestmove") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// push move lol
				select {
				case e.bestMove <- parts[1]:
				default:
					// full channel lol
				}
			}
		}
	}
}

// send command
func (e *StockfishEngine) sendCommand(cmd string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// write and flush
	fmt.Fprintln(e.stdin, cmd)
	e.stdin.Flush()
}

// waitfor removed

// get best move — cancelCh lets callers abort if game stops or user undoes lol
func (e *StockfishEngine) GetBestMove(fen string, cancelCh <-chan struct{}) (string, error) {
	// drain stale results before sending new position
	select {
	case <-e.bestMove:
	default:
	}

	// set fen lol
	e.sendCommand("position fen " + fen)

	// start engine search
	goCmd := fmt.Sprintf("go movetime %d depth %d", e.movetime, e.depth)
	e.sendCommand(goCmd)

	// wait for result — bail out if cancelled or engine dies lol
	select {
	case move := <-e.bestMove:
		return move, nil
	case <-e.done:
		return "", fmt.Errorf("rock brain die before answering")
	case <-cancelCh:
		// tell engine to stop thinking so we don't get stale bestmove later
		e.sendCommand("stop")
		// drain the move that stop will produce
		select {
		case <-e.bestMove:
		case <-e.done:
		}
		return "", fmt.Errorf("cancelled")
	}
}

// close engine
func (e *StockfishEngine) Close() {
	e.sendCommand("quit")
	// wait for end
	e.cmd.Wait()
}
