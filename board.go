package main

// board state

import (
	"fyne.io/fyne/v2"
	"github.com/notnil/chess"
)

// colors
var (
	lightSquareColor = [3]uint8{238, 238, 210} // bone white
	darkSquareColor  = [3]uint8{118, 150, 86}  // leaf green
)

// get piece images lol
func getPieceResource(p chess.Piece) fyne.Resource {
	// match pic
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
		// empty lol
		return nil
	}
}

// check square color idk this code sucks but ok.
func isLightSquare(row, col int) bool {
	return (row+col)%2 == 0
}
