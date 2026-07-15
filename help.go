package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/notnil/chess"
)

func (ca *ChessApp) assistantLoop() {
	for ca.running.Load() {
		if !ca.assistantActive.Load() {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		ca.mu.Lock()
		isWhiteTurn := ca.game.Position().Turn() == chess.White
		humanTurn := (isWhiteTurn && ca.lastWIsHuman) || (!isWhiteTurn && ca.lastBIsHuman)
		fen := ca.game.Position().String()
		ca.mu.Unlock()

		if !humanTurn {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		ca.mu.Lock()
		eng := ca.assistantEngine
		ca.mu.Unlock()
		
		if eng != nil {
		parts := strings.Fields(fen)
		if len(parts) >= 2 {
			if parts[1] == "w" { parts[1] = "b" } else { parts[1] = "w" }
		}
		if len(parts) >= 4 { parts[3] = "-" }
		flippedFen := strings.Join(parts, " ")
		ff, _ := chess.FEN(flippedFen)
		fg := chess.NewGame(ff)

		threats := make(map[chess.Square]bool)
		var threatenedPieces []string

		ca.mu.Lock()
		myTurnColor := ca.game.Position().Turn()
		ca.mu.Unlock()

		for _, m := range fg.ValidMoves() {
			if m.HasTag(chess.Capture) {
				ca.mu.Lock()
				targetPiece := ca.game.Position().Board().Piece(m.S2())
				ca.mu.Unlock()
				if targetPiece != chess.NoPiece && targetPiece.Color() == myTurnColor {
					if !threats[m.S2()] {
						threats[m.S2()] = true
						threatenedPieces = append(threatenedPieces, pieceName(targetPiece))
					}
				}
			}
		}

		ca.mu.Lock()
		moves := ca.game.Moves()
		kingCheck := false
		var kingSq chess.Square
		if len(moves) > 0 && moves[len(moves)-1].HasTag(chess.Check) {
			kingCheck = true
			for sq, p := range ca.game.Position().Board().SquareMap() {
				if p.Type() == chess.King && p.Color() == myTurnColor {
					kingSq = sq
					break
				}
			}
		}

		ca.assistantThreats = threats
		ca.assistantKingCheck = kingCheck
		ca.assistantKingSq = kingSq

		if len(threats) > 0 || kingCheck {
			var msg string
			if kingCheck {
				msg = "Warning: King is attacked!"
			} else if len(threatenedPieces) > 0 {
				if len(threatenedPieces) == 1 {
					msg = fmt.Sprintf("Warning: %s is at risk!", threatenedPieces[0])
				} else if len(threatenedPieces) == 2 {
					msg = fmt.Sprintf("Warning: %s and %s are at risk!", threatenedPieces[0], threatenedPieces[1])
				} else {
					msg = fmt.Sprintf("Warning: %d pieces are at risk!", len(threatenedPieces))
				}
			}
			if ca.warnLabel != nil {
				ca.warnLabel.Text = msg
				ca.warnLabel.Refresh()
				ca.warnContainer.Show()
			}
		} else {
			if ca.warnContainer != nil {
				ca.warnContainer.Hide()
			}
		}

		if ca.boardContainer != nil {
			ca.boardContainer.Refresh()
		}
		ca.mu.Unlock()
	}

		time.Sleep(2 * time.Second)
	}
}

func parseFlippedUCIMove(uci string, originalFen string) (*chess.Move, error) {
	if len(uci) < 4 { return nil, fmt.Errorf("invalid uci") }
	parts := strings.Fields(originalFen)
	if len(parts) >= 2 {
		if parts[1] == "w" { parts[1] = "b" } else { parts[1] = "w" }
	}
	if len(parts) >= 4 { parts[3] = "-" }
	flippedFen := strings.Join(parts, " ")
	ff, _ := chess.FEN(flippedFen)
	fg := chess.NewGame(ff)
	
	for _, m := range fg.ValidMoves() {
		if strings.EqualFold(m.String(), uci) {
			return m, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func pieceName(p chess.Piece) string {
	switch p.Type() {
	case chess.Pawn: return "Pawn"
	case chess.Knight: return "Knight"
	case chess.Bishop: return "Bishop"
	case chess.Rook: return "Rook"
	case chess.Queen: return "Queen"
	case chess.King: return "King"
	default: return "Piece"
	}
}
