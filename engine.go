package main

// Caveman talk to magic rock brain (Stockfish) through special grunt protocol (UCI).
// Caveman send position, rock brain think, rock brain grunt back best move.
// All grunt-reading happen in separate cave thread so main cave not freeze.

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// StockfishEngine — caveman's tamed rock brain. It think about chess so caveman not have to.
// Has pipe for talking (stdin), pipe for listening (stdout), and magic signal fire (channel) for moves.
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

// NewEngine — caveman summon new rock brain from deep cave.
// Give it skill level (how smart rock brain be), thread count, movetime, depth,
// and log callback.
func NewEngine(path string, skillLevel int, threads int, movetime int, depth int, logCb func(string)) (*StockfishEngine, error) {
	// Caveman create new rock brain process. Like hatching smart egg.
	cmd := exec.Command(path)

	// Caveman attach talking pipe to rock brain mouth.
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("caveman fail attach talking pipe: %w", err)
	}

	// Caveman attach listening pipe to rock brain ear.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("caveman fail attach listening pipe: %w", err)
	}

	// Caveman wake up rock brain. RISE!
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("caveman fail wake rock brain: %w", err)
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

	// Caveman teach rock brain the UCI protocol. "You speak UCI now, rock brain!"
	eng.sendCommand("uci")
	
	// Caveman wait synchronously for 'uciok' so no data race happen!
	for eng.scanner.Scan() {
		line := eng.scanner.Text()
		if eng.logCb != nil {
			eng.logCb(line)
		}
		if strings.TrimSpace(line) == "uciok" {
			break
		}
	}

	// Caveman set how smart rock brain should be. Low number = dumb rock. High number = genius boulder.
	eng.sendCommand(fmt.Sprintf("setoption name Skill Level value %d", skillLevel))

	// Caveman give rock brain many cave workers (threads) for faster thinking.
	eng.sendCommand(fmt.Sprintf("setoption name Threads value %d", threads))

	// Caveman give rock brain memory cave (hash table). 128 megabyte of remember-space.
	eng.sendCommand("setoption name Hash value 128")

	// Caveman ask "you ready, rock brain?" and wait for grunt of confirmation.
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

	// Caveman NOW start background cave worker to listen to rock brain grunts during game.
	go eng.readLoop()

	return eng, nil
}

// readLoop — background cave worker that never sleep. Always listening to rock brain output.
// Every grunt from rock brain get sent to cave wall terminal AND checked for bestmove signal.
func (e *StockfishEngine) readLoop() {
	defer close(e.done)
	// Caveman sit by pipe and listen. Every line rock brain grunt, caveman hear.
	for e.scanner.Scan() {
		line := e.scanner.Text()

		// Caveman show rock brain grunt on cave wall terminal for tribe to see.
		if e.logCb != nil {
			e.logCb(line)
		}

		// Caveman check if rock brain say magic words "bestmove". That mean rock brain done thinking!
		if strings.HasPrefix(line, "bestmove") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Caveman extract the move and put in signal fire. Non-blocking so cave not freeze.
				select {
				case e.bestMove <- parts[1]:
				default:
					// Caveman drop old move if channel full. Should never happen but caveman careful.
				}
			}
		}
	}
}

// sendCommand — caveman grunt command to rock brain through talking pipe.
// Must lock so two cave workers not grunt at same time and confuse rock brain.
func (e *StockfishEngine) sendCommand(cmd string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// Caveman write grunt to pipe, then flush so rock brain hear it NOW.
	fmt.Fprintln(e.stdin, cmd)
	e.stdin.Flush()
}

// (Caveman removed waitFor because it cause brain damage data race!)

// GetBestMove — caveman ask rock brain "what best move for this position?"
// Send board position (FEN string) and tell rock brain to think fast.
// Rock brain grunt back bestmove, caveman catch it from signal fire channel.
func (e *StockfishEngine) GetBestMove(fen string) (string, error) {
	// Caveman tell rock brain where all pieces are on board right now.
	e.sendCommand("position fen " + fen)

	// Caveman tell rock brain to think based on difficulty settings!
	// Extra depth and movetime for high levels.
	goCmd := fmt.Sprintf("go movetime %d depth %d", e.movetime, e.depth)
	e.sendCommand(goCmd)

	// Caveman wait by signal fire for rock brain to grunt bestmove.
	select {
	case move := <-e.bestMove:
		return move, nil
	case <-e.done:
		return "", fmt.Errorf("rock brain die before answering")
	}
}

// Close — caveman put rock brain back to sleep. Send "quit" grunt and wait for process to end.
// Clean up all pipes and channels. Caveman tidy cave.
func (e *StockfishEngine) Close() {
	e.sendCommand("quit")
	// Caveman wait for rock brain process to finish dying.
	e.cmd.Wait()
}
