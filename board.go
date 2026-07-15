package main

// Caveman brain remember where all rock-pieces sit on flat stone board.
// This file hold board wisdom — what piece live where, what color square be.

import (
	"github.com/notnil/chess"
	"fyne.io/fyne/v2"
)

// Caveman paint board with green grass and pale sand colors.
// Light square like dry bone, dark square like fresh leaf.
var (
	lightSquareColor = [3]uint8{238, 238, 210} // #EEEED2 — bone white
	darkSquareColor  = [3]uint8{118, 150, 86}  // #769656 — leaf green
)

// Caveman map each chess tribe member to their cave painting (SVG icon).
// This how caveman know which rock picture to show on board.
func getPieceResource(p chess.Piece) fyne.Resource {
	// Caveman look at piece and find matching cave painting.
	switch p {
	case chess.WhiteKing:
		return resourceKingWSvg
	case chess.WhiteQueen:
		return resourceQueenWSvg
	case chess.WhiteRook:
		return resourceRookWSvg
	case chess.WhiteBishop:
		return resourceBishopWSvg
	case chess.WhiteKnight:
		return resourceKnightWSvg
	case chess.WhitePawn:
		return resourcePawnWSvg
	case chess.BlackKing:
		return resourceKingBSvg
	case chess.BlackQueen:
		return resourceQueenBSvg
	case chess.BlackRook:
		return resourceRookBSvg
	case chess.BlackBishop:
		return resourceBishopBSvg
	case chess.BlackKnight:
		return resourceKnightBSvg
	case chess.BlackPawn:
		return resourcePawnBSvg
	default:
		// Caveman see empty square. No rock here. Return nothing.
		return nil
	}
}

// Caveman check if square dark or light. Even caveman know checkerboard pattern.
// Row plus column — if sum even, square light like sun. If odd, square dark like shadow.
func isLightSquare(row, col int) bool {
	return (row+col)%2 == 0
}
