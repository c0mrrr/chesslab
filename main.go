package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/notnil/chess"
)

var (
	totalCores       = runtime.NumCPU()
	threadsPerEngine = max((totalCores-2)/2, 1)
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

type GameSnapshot struct {
	fen      string
	lastMove *chess.Move
}

type RightClickButton struct {
	widget.Button
	OnRightTapped func()
}

func NewRightClickButton(text string, tapped func(), rightTapped func()) *RightClickButton {
	b := &RightClickButton{
		OnRightTapped: rightTapped,
	}
	b.Text = text
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *RightClickButton) TappedSecondary(e *fyne.PointEvent) {
	if b.OnRightTapped != nil {
		b.OnRightTapped()
	}
}

type ChessApp struct {
	fyneApp fyne.App
	window  fyne.Window
	game    *chess.Game

	rootStack        *fyne.Container
	normalContent    *fyne.Container
	boardPlaceholder *fyne.Container
	fsBoardStack     *fyne.Container
	boardContainer   *fyne.Container
	terminalPanel    *fyne.Container
	bottomArea       *fyne.Container
	boardLayoutObj   *boardLayout

	helpFromSq, helpToSq chess.Square
	helpActive           bool
	helpRect1            *canvas.Rectangle
	helpRect2            *canvas.Rectangle
	helpLine             *canvas.Line
	helpArrow1           *canvas.Line
	helpArrow2           *canvas.Line

	assistantThreats   map[chess.Square]bool
	assistantKingCheck bool

	hintRects       [64]*canvas.Rectangle
	safeHints       map[chess.Square]bool
	riskyHints      map[chess.Square]bool
	assistantKingSq chess.Square
	assistantRects  [64]*canvas.Rectangle
	currentSqSize   float32
	boardOffsetX    float32
	boardOffsetY    float32
	boardAbsPos     fyne.Position
	boardFlipped    bool

	bgSquares     [64]*boardSquare
	pieceImgs     [64]*canvas.Image
	flyImg        *canvas.Image
	deadWhiteImgs [16]*canvas.Image
	deadBlackImgs [16]*canvas.Image

	topmostOverlay          *fyne.Container
	loadingOverlayContainer *fyne.Container
	dragCheat               *canvas.Image
	cheatPopup              *widget.PopUp
	cheatsToggle            *widget.Button
	cheatsPanel             *fyne.Container
	cheatBtn                *widget.Button
	copyLogBtn              *widget.Button
	reverseBtn              *widget.Button
	passBtn                 *widget.Button
	helpBtn                 *RightClickButton
	assistantCheck          *widget.Check

	selectedSq  chess.Square
	humanMoveCh chan *chess.Move

	statusLabel   *widget.Label
	moveLabel     *widget.Label
	warnLabel     *canvas.Text
	warnContainer *fyne.Container
	turnWidget    *TurnIndicator

	whiteEngine     *StockfishEngine
	blackEngine     *StockfishEngine
	assistantEngine *StockfishEngine
	whiteLog        *terminalWidget
	blackLog        *terminalWidget

	lastWIsHuman     bool
	lastBIsHuman     bool
	lastWSkill       int
	lastBSkill       int
	speedrun         atomic.Bool
	animate          atomic.Bool
	autoLoop         atomic.Bool
	autoHelpActive   atomic.Bool
	autoHelpBtnFlash atomic.Bool
	lastHelpClick    time.Time
	assistantActive  atomic.Bool
	running          atomic.Bool
	stopCh           chan struct{}
	mu               sync.Mutex
	history          []GameSnapshot
}

// board custom widgets & layout lol

type boardSquare struct {
	widget.BaseWidget
	ca *ChessApp
	sq chess.Square
	bg *canvas.Rectangle
}

func newBoardSquare(ca *ChessApp, sq chess.Square, sqColor color.Color) *boardSquare {
	rect := canvas.NewRectangle(sqColor)
	rect.StrokeColor = color.NRGBA{0, 0, 0, 5}
	rect.StrokeWidth = 0.2
	s := &boardSquare{ca: ca, sq: sq, bg: rect}
	s.ExtendBaseWidget(s)
	return s
}

func (s *boardSquare) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.bg)
}

func (s *boardSquare) Tapped(e *fyne.PointEvent) {
	s.ca.handleSquareTapped(s.sq)
}

type boardWrapper struct {
	widget.BaseWidget
	ca      *ChessApp
	content *fyne.Container
}

func newBoardWrapper(ca *ChessApp, content *fyne.Container) *boardWrapper {
	w := &boardWrapper{ca: ca, content: content}
	w.ExtendBaseWidget(w)
	return w
}

func (w *boardWrapper) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.content)
}

func (w *boardWrapper) MouseMoved(e *desktop.MouseEvent) {
	w.ca.boardAbsPos = fyne.NewPos(e.AbsolutePosition.X-e.Position.X, e.AbsolutePosition.Y-e.Position.Y)
}
func (w *boardWrapper) MouseIn(e *desktop.MouseEvent) {}
func (w *boardWrapper) MouseOut()                     {}

type boardLayout struct {
	ca *ChessApp
}

func (l *boardLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	sideLength := minFloat(size.Width, size.Height)
	sqSize := float32(int(sideLength / 8))

	offsetX := float32(int((size.Width - sqSize*8) / 2))
	offsetY := float32(int((size.Height - sqSize*8) / 2))

	l.ca.currentSqSize = sqSize
	l.ca.boardOffsetX = offsetX
	l.ca.boardOffsetY = offsetY

	for i := 0; i < 64; i++ {
		row := 7 - (i / 8)
		col := i % 8

		pos := fyne.NewPos(offsetX+float32(col)*sqSize, offsetY+float32(row)*sqSize)
		sz := fyne.NewSize(sqSize, sqSize)

		if i < len(objects) {
			objects[i].Resize(sz)
			objects[i].Move(pos)
		}

		pIdx := i + 64
		if pIdx < len(objects) {
			objects[pIdx].Resize(sz)
			objects[pIdx].Move(pos)
		}
	}

	if len(objects) > 128 {
		objects[128].Resize(fyne.NewSize(sqSize, sqSize))
	}

	if l.ca.helpActive && l.ca.helpRect1 != nil {
		fromRow := 7 - int(l.ca.helpFromSq/8)
		fromCol := int(l.ca.helpFromSq % 8)
		toRow := 7 - int(l.ca.helpToSq/8)
		toCol := int(l.ca.helpToSq % 8)
		if l.ca.boardFlipped {
			fromRow = 7 - fromRow
			fromCol = 7 - fromCol
			toRow = 7 - toRow
			toCol = 7 - toCol
		}
		sz := fyne.NewSize(sqSize, sqSize)
		fromPos := fyne.NewPos(offsetX+float32(fromCol)*sqSize, offsetY+float32(fromRow)*sqSize)
		toPos := fyne.NewPos(offsetX+float32(toCol)*sqSize, offsetY+float32(toRow)*sqSize)

		l.ca.helpRect1.Resize(sz)
		l.ca.helpRect1.Move(fromPos)
		l.ca.helpRect1.Show()

		l.ca.helpRect2.Resize(sz)
		l.ca.helpRect2.Move(toPos)
		l.ca.helpRect2.Show()

		l.ca.helpLine.Position1 = fyne.NewPos(fromPos.X+sqSize/2, fromPos.Y+sqSize/2)
		l.ca.helpLine.Position2 = fyne.NewPos(toPos.X+sqSize/2, toPos.Y+sqSize/2)
		l.ca.helpLine.Show()

		dx := float64(l.ca.helpLine.Position2.X - l.ca.helpLine.Position1.X)
		dy := float64(l.ca.helpLine.Position2.Y - l.ca.helpLine.Position1.Y)
		angle := math.Atan2(dy, dx)
		arrowLen := float64(15.0)

		p1X := float64(l.ca.helpLine.Position2.X) - arrowLen*math.Cos(angle-math.Pi/6)
		p1Y := float64(l.ca.helpLine.Position2.Y) - arrowLen*math.Sin(angle-math.Pi/6)
		l.ca.helpArrow1.Position1 = l.ca.helpLine.Position2
		l.ca.helpArrow1.Position2 = fyne.NewPos(float32(p1X), float32(p1Y))
		l.ca.helpArrow1.Show()

		p2X := float64(l.ca.helpLine.Position2.X) - arrowLen*math.Cos(angle+math.Pi/6)
		p2Y := float64(l.ca.helpLine.Position2.Y) - arrowLen*math.Sin(angle+math.Pi/6)
		l.ca.helpArrow2.Position1 = l.ca.helpLine.Position2
		l.ca.helpArrow2.Position2 = fyne.NewPos(float32(p2X), float32(p2Y))
		l.ca.helpArrow2.Show()
	} else if l.ca.helpRect1 != nil {
		l.ca.helpRect1.Hide()
		l.ca.helpRect2.Hide()
		l.ca.helpLine.Hide()
		l.ca.helpArrow1.Hide()
		l.ca.helpArrow2.Hide()
	}

	for i := 0; i < 64; i++ {
		rect := l.ca.assistantRects[i]
		if rect == nil {
			continue
		}

		isActiveThreat := l.ca.assistantThreats != nil && l.ca.assistantThreats[chess.Square(i)]
		isKingCheck := l.ca.assistantKingCheck && chess.Square(i) == l.ca.assistantKingSq

		if l.ca.assistantActive.Load() && (isActiveThreat || isKingCheck) {
			row := 7 - (i / 8)
			col := i % 8
			if l.ca.boardFlipped {
				row = i / 8
				col = 7 - (i % 8)
			}
			pos := fyne.NewPos(offsetX+float32(col)*sqSize, offsetY+float32(row)*sqSize)
			sz := fyne.NewSize(sqSize, sqSize)

			rect.Resize(sz)
			rect.Move(pos)

			if isKingCheck {
				rect.StrokeColor = color.NRGBA{R: 200, G: 0, B: 0, A: 255}
				rect.FillColor = color.NRGBA{R: 200, G: 0, B: 0, A: 60}
			} else {
				rect.StrokeColor = color.NRGBA{R: 255, G: 140, B: 0, A: 255}
				rect.FillColor = color.NRGBA{R: 255, G: 140, B: 0, A: 60}
			}
			rect.Show()
		} else {
			rect.Hide()
		}
	}
	for i := 0; i < 64; i++ {
		rect := l.ca.hintRects[i]
		if rect == nil {
			continue
		}

		isSafe := l.ca.safeHints != nil && l.ca.safeHints[chess.Square(i)]
		isRisky := l.ca.riskyHints != nil && l.ca.riskyHints[chess.Square(i)]

		if l.ca.assistantActive.Load() && (isSafe || isRisky) {
			row := 7 - (i / 8)
			col := i % 8
			if l.ca.boardFlipped {
				row = i / 8
				col = 7 - (i % 8)
			}
			pos := fyne.NewPos(offsetX+float32(col)*sqSize, offsetY+float32(row)*sqSize)
			sz := fyne.NewSize(sqSize, sqSize)

			rect.Resize(sz)
			rect.Move(pos)

			if isRisky {
				rect.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
				rect.FillColor = color.NRGBA{R: 255, G: 0, B: 0, A: 60}
			} else {
				rect.StrokeColor = color.NRGBA{R: 0, G: 255, B: 0, A: 255}
				rect.FillColor = color.NRGBA{R: 0, G: 255, B: 0, A: 60}
			}
			rect.Show()
		} else {
			rect.Hide()
		}
	}
}

func (l *boardLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(320, 320)
}

// meatbag click to move

func (ca *ChessApp) handleSquareTapped(sq chess.Square) {
	if !ca.running.Load() {
		return
	}
	pos := ca.game.Position()
	isWhiteTurn := pos.Turn() == chess.White
	if (isWhiteTurn && !ca.lastWIsHuman) || (!isWhiteTurn && !ca.lastBIsHuman) {
		return
	}

	if ca.selectedSq == chess.NoSquare {
		p := pos.Board().Piece(sq)
		if p != chess.NoPiece && p.Color() == pos.Turn() {
			ca.selectedSq = sq
			ca.highlightSquare(sq)
			if ca.assistantActive.Load() {
				ca.showMoveHints(sq)
			}
		}
	} else {
		if ca.selectedSq == sq {
			ca.clearHighlight()
			ca.selectedSq = chess.NoSquare
			return
		}

		validMoves := ca.game.ValidMoves()
		var promoMoves []*chess.Move
		var chosenMove *chess.Move
		for _, m := range validMoves {
			if m.S1() == ca.selectedSq && m.S2() == sq {
				if m.Promo() != chess.NoPieceType {
					promoMoves = append(promoMoves, m)
				} else {
					chosenMove = m
					break
				}
			}
		}

		if len(promoMoves) > 0 {
			ca.showPromotionPopup(promoMoves)
			ca.clearHighlight()
			ca.selectedSq = chess.NoSquare
			return
		}

		if chosenMove != nil {
			ca.clearHighlight()
			ca.selectedSq = chess.NoSquare
			select {
			case ca.humanMoveCh <- chosenMove:
			default:
			}
		} else {
			p := pos.Board().Piece(sq)
			if p != chess.NoPiece && p.Color() == pos.Turn() {
				ca.clearHighlight()
				ca.selectedSq = sq
				ca.highlightSquare(sq)
				if ca.assistantActive.Load() {
					ca.showMoveHints(sq)
				}
			} else {
				ca.clearHighlight()
				ca.selectedSq = chess.NoSquare
			}
		}
	}
}

func (ca *ChessApp) highlightSquare(sq chess.Square) {
	ca.bgSquares[sq].bg.FillColor = color.NRGBA{R: 200, G: 200, B: 50, A: 150}
	ca.bgSquares[sq].Refresh()
}

func (ca *ChessApp) clearHighlight() {
	if ca.selectedSq != chess.NoSquare {
		row := 7 - (int(ca.selectedSq) / 8)
		col := int(ca.selectedSq) % 8
		var sqColor color.NRGBA
		if isLightSquare(row, col) {
			sqColor = color.NRGBA{R: lightSquareColor[0], G: lightSquareColor[1], B: lightSquareColor[2], A: 255}
		} else {
			sqColor = color.NRGBA{R: darkSquareColor[0], G: darkSquareColor[1], B: darkSquareColor[2], A: 255}
		}
		ca.bgSquares[ca.selectedSq].bg.FillColor = sqColor
		ca.bgSquares[ca.selectedSq].Refresh()
	}
	ca.safeHints = nil
	ca.riskyHints = nil
	if ca.boardContainer != nil {
		ca.boardContainer.Refresh()
	}
}

// Cheats in case you suck at chess

type CheatPieceWidget struct {
	widget.BaseWidget
	ca    *ChessApp
	piece chess.Piece
	img   *canvas.Image
}

func newCheatPieceWidget(ca *ChessApp, p chess.Piece) *CheatPieceWidget {
	c := &CheatPieceWidget{ca: ca, piece: p}
	c.img = canvas.NewImageFromResource(getPieceResource(p))
	c.img.FillMode = canvas.ImageFillContain
	c.img.SetMinSize(fyne.NewSize(50, 50))
	c.ExtendBaseWidget(c)
	return c
}

func (c *CheatPieceWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.img)
}

func (c *CheatPieceWidget) Dragged(e *fyne.DragEvent) {
	if c.ca.dragCheat == nil {
		c.ca.dragCheat = canvas.NewImageFromResource(getPieceResource(c.piece))
		c.ca.dragCheat.FillMode = canvas.ImageFillContain
		c.ca.dragCheat.Resize(fyne.NewSize(c.ca.currentSqSize, c.ca.currentSqSize))
		c.ca.topmostOverlay.Add(c.ca.dragCheat)
	}
	// center svg on cursor
	offset := c.ca.currentSqSize / 2
	c.ca.dragCheat.Move(fyne.NewPos(e.AbsolutePosition.X-offset, e.AbsolutePosition.Y-offset))
	c.ca.dragCheat.Show()
	c.ca.topmostOverlay.Refresh()
}
func (c *CheatPieceWidget) DragEnd() {
	if c.ca.dragCheat != nil {
		pos := c.ca.dragCheat.Position()
		c.ca.dragCheat.Hide()
		c.ca.topmostOverlay.RemoveAll()
		c.ca.dragCheat = nil

		// adjust top left to center of dropped piece
		centerPos := fyne.NewPos(pos.X+c.ca.currentSqSize/2, pos.Y+c.ca.currentSqSize/2)
		c.ca.handleCheatDrop(centerPos, c.piece)
	}
}

func (ca *ChessApp) showCheatMenu() {
	var cheatColor chess.Color
	if ca.lastWIsHuman {
		cheatColor = chess.White
	} else {
		cheatColor = chess.Black
	}

	pieces := []chess.PieceType{chess.Queen, chess.Rook, chess.Bishop, chess.Knight, chess.Pawn}

	grid := container.NewGridWithColumns(1)
	for _, pType := range pieces {
		grid.Add(newCheatPieceWidget(ca, chess.NewPiece(pType, cheatColor)))
	}

	ca.cheatPopup = widget.NewModalPopUp(container.NewPadded(grid), ca.window.Canvas())
	ca.cheatPopup.Show()
}

func (ca *ChessApp) handleCheatDrop(pos fyne.Position, p chess.Piece) {
	if ca.cheatPopup != nil {
		ca.cheatPopup.Hide()
	}

	bx := pos.X - ca.boardAbsPos.X - ca.boardOffsetX
	by := pos.Y - ca.boardAbsPos.Y - ca.boardOffsetY

	if bx < 0 || by < 0 || bx > ca.currentSqSize*8 || by > ca.currentSqSize*8 {
		return // cancel movement lol
	}

	col := int(bx / ca.currentSqSize)
	row := int(by / ca.currentSqSize)
	sqIdx := (7-row)*8 + col
	sq := chess.Square(sqIdx)

	ca.applyCheatPiece(sq, p)
}

func (ca *ChessApp) applyCheatPiece(sq chess.Square, p chess.Piece) {
	ca.mu.Lock()

	pos := ca.game.Position()
	sqMap := pos.Board().SquareMap()
	if sqMap[sq] != chess.NoPiece {
		ca.mu.Unlock()
		return // invalid pos
	}

	sqMap[sq] = p
	newBoard := chess.NewBoard(sqMap)

	oldFen := pos.String()
	parts := strings.Fields(oldFen)
	if len(parts) >= 1 {
		parts[0] = newBoard.String()
	}
	// sanitize fen: clear castling + en passant to prevent stockfish's brain from melting lol
	if len(parts) >= 3 {
		parts[2] = "-"
	}
	if len(parts) >= 4 {
		parts[3] = "-"
	}
	newFen := strings.Join(parts, " ")

	fenFunc, err := chess.FEN(newFen)
	if err == nil {
		ca.history = append(ca.history, GameSnapshot{fen: oldFen, lastMove: nil})
		ca.game = chess.NewGame(fenFunc)
		ca.mu.Unlock()
		ca.refreshBoard()
	} else {
		ca.mu.Unlock()
	}
}

// terminal widget lol
type terminalWidget struct {
	linesText []*canvas.Text
	bg        *canvas.Rectangle
	panel     *fyne.Container
	lines     []string
	maxLines  int
	// this rendering logic is absolute trash why did i build it like this
	mu    sync.Mutex
	dirty atomic.Bool
}

func newTerminalWidget() *terminalWidget {
	bg := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	maxLines := 20
	linesText := make([]*canvas.Text, maxLines)
	vbox := container.NewVBox()

	for i := 0; i < maxLines; i++ {
		txt := canvas.NewText("", color.White)
		txt.TextSize = 9
		txt.TextStyle = fyne.TextStyle{Monospace: true}
		linesText[i] = txt
		vbox.Add(txt)
	}

	tw := &terminalWidget{
		linesText: linesText,
		bg:        bg,
		panel:     container.NewStack(bg, vbox),
		lines:     make([]string, 0, maxLines),
		maxLines:  maxLines,
	}

	go func() {
		for {
			time.Sleep(33 * time.Millisecond)
			if tw.dirty.Load() {
				tw.dirty.Store(false)
				tw.mu.Lock()
				currentLines := make([]string, len(tw.lines))
				copy(currentLines, tw.lines)
				tw.mu.Unlock()

				for i := 0; i < tw.maxLines; i++ {
					if i < len(currentLines) {
						tw.linesText[i].Text = currentLines[i]
					} else {
						tw.linesText[i].Text = ""
					}
					tw.linesText[i].Refresh()
				}
			}
		}
	}()
	return tw
}

func (tw *terminalWidget) appendLine(line string) {
	const maxLen = 38
	if len(line) > maxLen {
		line = line[:maxLen-3] + "..."
	}
	tw.mu.Lock()
	tw.lines = append(tw.lines, line)
	if len(tw.lines) > tw.maxLines {
		tw.lines = tw.lines[len(tw.lines)-tw.maxLines:]
	}
	tw.mu.Unlock()
	tw.dirty.Store(true)
}

func (tw *terminalWidget) clear() {
	tw.mu.Lock()
	tw.lines = tw.lines[:0]
	tw.mu.Unlock()
	tw.dirty.Store(true)
}

// turn indicator logic

type TurnIndicator struct {
	whitePawnImg *canvas.Image
	blackPawnImg *canvas.Image
	slider       *canvas.Rectangle
	panel        *fyne.Container
	innerOverlay *fyne.Container
	isWhite      bool
}

const (
	pawnIconSize float32 = 52
	indicatorGap float32 = 8
	sliderBarH   float32 = 6
	sliderGapY   float32 = 4
)

func newTurnIndicator() *TurnIndicator {
	ti := &TurnIndicator{isWhite: true}

	panelW := pawnIconSize*2 + indicatorGap*3
	panelH := indicatorGap + pawnIconSize + sliderGapY + sliderBarH + indicatorGap

	bg := canvas.NewRectangle(color.NRGBA{R: 30, G: 30, B: 30, A: 220})
	bg.CornerRadius = 8
	bg.Resize(fyne.NewSize(panelW, panelH))
	bg.Move(fyne.Position{})

	ti.whitePawnImg = canvas.NewImageFromResource(resourcePawnWSvg)
	ti.whitePawnImg.FillMode = canvas.ImageFillContain
	ti.whitePawnImg.Resize(fyne.NewSize(pawnIconSize, pawnIconSize))
	ti.whitePawnImg.Move(fyne.NewPos(indicatorGap, indicatorGap))

	ti.blackPawnImg = canvas.NewImageFromResource(resourcePawnBSvg)
	ti.blackPawnImg.FillMode = canvas.ImageFillContain
	ti.blackPawnImg.Resize(fyne.NewSize(pawnIconSize, pawnIconSize))
	ti.blackPawnImg.Move(fyne.NewPos(pawnIconSize+indicatorGap*2, indicatorGap))

	sliderY := indicatorGap + pawnIconSize + sliderGapY
	ti.slider = canvas.NewRectangle(color.NRGBA{R: 80, G: 200, B: 80, A: 220})
	ti.slider.CornerRadius = 3
	ti.slider.Resize(fyne.NewSize(pawnIconSize, sliderBarH))
	ti.slider.Move(fyne.NewPos(indicatorGap, sliderY))

	ti.innerOverlay = container.NewWithoutLayout(bg, ti.slider, ti.whitePawnImg, ti.blackPawnImg)
	ti.panel = container.New(layout.NewGridWrapLayout(fyne.NewSize(panelW, panelH)), ti.innerOverlay)
	return ti
}

func (ti *TurnIndicator) setTurn(isWhite bool) {
	if ti.isWhite == isWhite {
		return
	}
	ti.isWhite = isWhite

	whiteX := indicatorGap
	blackX := pawnIconSize + indicatorGap*2

	var fromX, toX float32
	if isWhite {
		fromX, toX = blackX, whiteX
	} else {
		fromX, toX = whiteX, blackX
	}

	sliderAnim := canvas.NewPositionAnimation(
		fyne.NewPos(fromX, ti.slider.Position().Y),
		fyne.NewPos(toX, ti.slider.Position().Y),
		300*time.Millisecond,
		func(p fyne.Position) {
			ti.slider.Move(p)
			ti.innerOverlay.Refresh()
		},
	)
	sliderAnim.Curve = fyne.AnimationEaseInOut
	sliderAnim.Start()
}

// main fuckery

func main() {
	runtime.GOMAXPROCS(totalCores)

	a := app.New()
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("ChessLab")
	w.SetMaster()

	ca := &ChessApp{
		fyneApp:     a,
		window:      w,
		stopCh:      make(chan struct{}),
		humanMoveCh: make(chan *chess.Move, 1),
		selectedSq:  chess.NoSquare,
	}
	ca.animate.Store(true)

	content := ca.buildUI()
	w.SetContent(content)
	w.Resize(fyne.NewSize(1600, 1000))
	w.CenterOnScreen()

	go func() {
		time.Sleep(400 * time.Millisecond)
		ca.showNewGameWindow()
	}()

	w.ShowAndRun()
}

func (ca *ChessApp) buildUI() fyne.CanvasObject {
	ca.buildBoard()

	leftPanel := ca.buildDeadPiecesPanel(true)
	rightPanel := ca.buildDeadPiecesPanel(false)

	boardWithDead := container.NewBorder(
		nil, nil,
		container.NewVBox(leftPanel),
		container.NewVBox(rightPanel),
		ca.boardPlaceholder,
	)

	ca.statusLabel = widget.NewLabel("Ready to play")
	ca.statusLabel.Alignment = fyne.TextAlignCenter
	ca.moveLabel = widget.NewLabel("")
	ca.moveLabel.Alignment = fyne.TextAlignCenter

	newGameBtn := widget.NewButton("New Game", func() {
		ca.stopGame()
		ca.showNewGameWindow()
	})
	stopBtn := widget.NewButton("STOP", func() {
		ca.stopGame()
	})

	ca.cheatBtn = widget.NewButton("Add Piece", func() {
		ca.showCheatMenu()
	})

	ca.reverseBtn = widget.NewButton("Undo", func() {
		go ca.handleReverse()
	})

	ca.passBtn = widget.NewButton("Pass", func() {
		select {
		case ca.humanMoveCh <- nil:
		default:
		}
	})

	ca.copyLogBtn = widget.NewButton("Copy Engine Logs", func() {
		var b strings.Builder
		b.WriteString("=== WHITE ENGINE LOG ===\n")
		ca.whiteLog.mu.Lock()
		for _, l := range ca.whiteLog.lines {
			b.WriteString(l + "\n")
		}
		ca.whiteLog.mu.Unlock()
		b.WriteString("\n=== BLACK ENGINE LOG ===\n")
		ca.blackLog.mu.Lock()
		for _, l := range ca.blackLog.lines {
			b.WriteString(l + "\n")
		}
		ca.blackLog.mu.Unlock()
		ca.window.Clipboard().SetContent(b.String())
	})
	ca.copyLogBtn.Hide()

	uiControls := container.NewHBox(layout.NewSpacer(), newGameBtn, stopBtn, ca.copyLogBtn, layout.NewSpacer())
	statusPanel := container.NewVBox(uiControls)

	boardPanel := container.NewBorder(
		nil,
		statusPanel,
		nil, nil,
		boardWithDead,
	)

	ca.whiteLog = newTerminalWidget()
	ca.blackLog = newTerminalWidget()

	whiteTerminal := container.NewBorder(
		widget.NewLabelWithStyle("White Engine", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil, ca.whiteLog.panel,
	)
	blackTerminal := container.NewBorder(
		widget.NewLabelWithStyle("Black Engine", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		nil, nil, nil, ca.blackLog.panel,
	)

	ca.terminalPanel = container.NewGridWithRows(2, whiteTerminal, blackTerminal)

	split := container.NewHSplit(boardPanel, ca.terminalPanel)
	split.SetOffset(0.70)

	ca.turnWidget = newTurnIndicator()

	ca.cheatsToggle = widget.NewButton("Cheats", func() {
		if ca.cheatsPanel.Hidden {
			ca.cheatsPanel.Show()
		} else {
			ca.cheatsPanel.Hide()
		}
	})
	ca.cheatsToggle.Hide()

	ca.helpBtn = NewRightClickButton("Help", func() {
		ca.showHelpVisualization()
	}, func() {
		ca.toggleAutoHelp()
	})
	ca.assistantCheck = widget.NewCheck("Chess Assist", func(checked bool) {
		ca.assistantActive.Store(checked)
		if !checked && ca.warnContainer != nil {
			ca.warnLabel.Text = ""
			ca.warnContainer.Hide()
		}
	})

	ca.warnLabel = canvas.NewText("", color.NRGBA{0, 0, 0, 255})
	ca.warnLabel.TextStyle = fyne.TextStyle{Bold: true}
	ca.warnLabel.Alignment = fyne.TextAlignCenter

	warnBg := canvas.NewRectangle(color.NRGBA{255, 255, 0, 150})
	ca.warnContainer = container.NewStack(warnBg, container.NewPadded(ca.warnLabel))
	ca.warnContainer.Hide()

	ca.cheatsPanel = container.NewHBox(
		ca.cheatBtn, ca.reverseBtn, ca.passBtn, ca.helpBtn,
		ca.assistantCheck, ca.warnContainer,
	)
	ca.cheatsPanel.Hide()

	ca.bottomArea = container.NewHBox(
		ca.turnWidget.panel,
		container.NewVBox(layout.NewSpacer(), ca.cheatsToggle),
		container.NewVBox(layout.NewSpacer(), ca.cheatsPanel),
		layout.NewSpacer(),
		container.NewVBox(layout.NewSpacer(), ca.statusLabel),
		container.NewVBox(layout.NewSpacer(), ca.moveLabel),
	)

	ca.normalContent = container.NewBorder(
		nil,
		ca.bottomArea,
		nil, nil,
		split,
	)

	bg := canvas.NewRectangle(color.NRGBA{0, 0, 0, 180})
	label := widget.NewLabelWithStyle("Initializing Stockfish Engine...", fyne.TextAlignCenter, fyne.TextStyle{Bold: true, Italic: true})
	progress := widget.NewProgressBarInfinite()
	box := container.NewVBox(label, progress)
	ca.loadingOverlayContainer = container.NewStack(bg, container.NewCenter(box))
	ca.loadingOverlayContainer.Hide()

	ca.topmostOverlay = container.NewWithoutLayout()
	ca.rootStack = container.NewStack(ca.normalContent, ca.loadingOverlayContainer, ca.topmostOverlay)
	return ca.rootStack
}

func (ca *ChessApp) buildDeadPiecesPanel(isWhite bool) fyne.CanvasObject {
	grid := container.NewGridWithColumns(2)

	title := "Captured Black"
	if isWhite {
		title = "Captured White"
	}
	label := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	for i := 0; i < 16; i++ {
		// this hacky state check is literally making me lose my mind ngl
		img := canvas.NewImageFromResource(nil)
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(30, 30))
		img.Hide()

		if isWhite {
			ca.deadWhiteImgs[i] = img
		} else {
			ca.deadBlackImgs[i] = img
		}
		grid.Add(img)
	}

	content := container.NewVBox(label, grid)
	bg := canvas.NewRectangle(color.NRGBA{R: 40, G: 40, B: 40, A: 255})
	bg.CornerRadius = 8
	return container.NewStack(bg, container.NewPadded(content))
}

func (ca *ChessApp) buildBoard() {
	ca.boardLayoutObj = &boardLayout{ca: ca}
	objs := make([]fyne.CanvasObject, 0, 129)

	for i := 0; i < 64; i++ {
		row := 7 - (i / 8)
		col := i % 8
		if ca.boardFlipped {
			row = i / 8
			col = 7 - (i % 8)
		}
		var squareColor color.NRGBA
		if isLightSquare(row, col) {
			squareColor = color.NRGBA{R: lightSquareColor[0], G: lightSquareColor[1], B: lightSquareColor[2], A: 255}
		} else {
			squareColor = color.NRGBA{R: darkSquareColor[0], G: darkSquareColor[1], B: darkSquareColor[2], A: 255}
		}

		sq := newBoardSquare(ca, chess.Square(i), squareColor)
		ca.bgSquares[i] = sq
		objs = append(objs, sq)
	}

	for i := 0; i < 64; i++ {
		img := canvas.NewImageFromResource(nil)
		img.FillMode = canvas.ImageFillContain
		img.Hide()
		ca.pieceImgs[i] = img
		objs = append(objs, img)
	}

	ca.flyImg = canvas.NewImageFromResource(nil)
	ca.flyImg.FillMode = canvas.ImageFillContain
	ca.flyImg.Hide()
	objs = append(objs, ca.flyImg)

	ca.helpRect1 = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	ca.helpRect1.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
	ca.helpRect1.StrokeWidth = 3
	ca.helpRect1.Hide()
	objs = append(objs, ca.helpRect1)

	ca.helpRect2 = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
	ca.helpRect2.StrokeColor = color.NRGBA{R: 255, G: 0, B: 0, A: 200}
	ca.helpRect2.StrokeWidth = 3
	ca.helpRect2.Hide()
	objs = append(objs, ca.helpRect2)

	ca.helpLine = canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 150})
	ca.helpLine.StrokeWidth = 3
	ca.helpLine.Hide()
	objs = append(objs, ca.helpLine)

	ca.helpArrow1 = canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 150})
	ca.helpArrow1.StrokeWidth = 3
	ca.helpArrow1.Hide()
	objs = append(objs, ca.helpArrow1)

	ca.helpArrow2 = canvas.NewLine(color.NRGBA{R: 255, G: 0, B: 0, A: 150})
	ca.helpArrow2.StrokeWidth = 3
	ca.helpArrow2.Hide()
	objs = append(objs, ca.helpArrow2)

	for i := 0; i < 64; i++ {
		ca.assistantRects[i] = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
		ca.assistantRects[i].StrokeWidth = 3
		ca.assistantRects[i].Hide()
		objs = append(objs, ca.assistantRects[i])
	}
	for i := 0; i < 64; i++ {
		ca.hintRects[i] = canvas.NewRectangle(color.NRGBA{0, 0, 0, 0})
		ca.hintRects[i].StrokeWidth = 3
		ca.hintRects[i].Hide()
		objs = append(objs, ca.hintRects[i])
	}

	innerContainer := container.New(ca.boardLayoutObj, objs...)
	ca.boardContainer = container.NewStack(newBoardWrapper(ca, innerContainer))
	ca.boardPlaceholder = container.NewMax(ca.boardContainer)
	ca.fsBoardStack = container.NewMax()
}

func (ca *ChessApp) refreshBoard() {
	board := ca.game.Position().Board()
	for sq := 0; sq < 64; sq++ {
		piece := board.Piece(chess.Square(sq))
		res := getPieceResource(piece)
		img := ca.pieceImgs[sq]

		img.Resource = res
		if res == nil {
			img.Hide()
		} else {
			img.Show()
		}
		img.Refresh()
	}

	ca.updateDeadPieces(board)
}

func (ca *ChessApp) updateDeadPieces(board *chess.Board) {
	whiteCounts := make(map[chess.PieceType]int)
	blackCounts := make(map[chess.PieceType]int)

	for sq := 0; sq < 64; sq++ {
		p := board.Piece(chess.Square(sq))
		if p != chess.NoPiece {
			if p.Color() == chess.White {
				whiteCounts[p.Type()]++
			} else {
				blackCounts[p.Type()]++
			}
		}
	}

	initial := map[chess.PieceType]int{
		chess.Queen: 1, chess.Rook: 2, chess.Bishop: 2, chess.Knight: 2, chess.Pawn: 8,
	}
	order := []chess.PieceType{chess.Queen, chess.Rook, chess.Bishop, chess.Knight, chess.Pawn}

	wIdx := 0
	for _, pType := range order {
		missing := initial[pType] - whiteCounts[pType]
		for i := 0; i < missing; i++ {
			if wIdx < 16 {
				ca.deadWhiteImgs[wIdx].Resource = getPieceResource(chess.NewPiece(pType, chess.White))
				ca.deadWhiteImgs[wIdx].Show()
				ca.deadWhiteImgs[wIdx].Refresh()
				wIdx++
			}
		}
	}
	for i := wIdx; i < 16; i++ {
		ca.deadWhiteImgs[i].Resource = nil
		ca.deadWhiteImgs[i].Hide()
		ca.deadWhiteImgs[i].Refresh()
	}

	bIdx := 0
	for _, pType := range order {
		missing := initial[pType] - blackCounts[pType]
		for i := 0; i < missing; i++ {
			if bIdx < 16 {
				ca.deadBlackImgs[bIdx].Resource = getPieceResource(chess.NewPiece(pType, chess.Black))
				ca.deadBlackImgs[bIdx].Show()
				ca.deadBlackImgs[bIdx].Refresh()
				bIdx++
			}
		}
	}
	for i := bIdx; i < 16; i++ {
		ca.deadBlackImgs[i].Resource = nil
		ca.deadBlackImgs[i].Hide()
		ca.deadBlackImgs[i].Refresh()
	}
}

func (ca *ChessApp) animateMove(fromSq, toSq chess.Square) {
	img := ca.pieceImgs[fromSq]
	if img == nil || img.Resource == nil {
		return
	}

	movingResource := img.Resource

	// Hide any piece at the target square before animating
	if toSq != fromSq {
		if targetImg := ca.pieceImgs[toSq]; targetImg != nil && targetImg.Resource != nil {
			targetImg.Resource = nil
			targetImg.Hide()
			targetImg.Refresh()
		}
	}

	ca.flyImg.Resource = movingResource
	ca.flyImg.Resize(fyne.NewSize(ca.currentSqSize, ca.currentSqSize))
	ca.flyImg.Move(img.Position())
	ca.flyImg.Show()
	ca.flyImg.Refresh()

	img.Resource = nil
	img.Hide()
	img.Refresh()

	toCol := float32(toSq % 8)
	toRow := 7 - float32(toSq/8)
	targetPos := fyne.NewPos(ca.boardOffsetX+toCol*ca.currentSqSize, ca.boardOffsetY+toRow*ca.currentSqSize)

	done := make(chan struct{})
	anim := canvas.NewPositionAnimation(ca.flyImg.Position(), targetPos, 200*time.Millisecond, func(p fyne.Position) {
		ca.flyImg.Move(p)
		ca.flyImg.Refresh()
	})
	anim.Curve = fyne.AnimationLinear
	anim.Start()

	go func() {
		time.Sleep(210 * time.Millisecond)
		close(done)
	}()
	<-done

	// Pre-set destination image BEFORE hiding flyImg to prevent flash
	destImg := ca.pieceImgs[toSq]
	if destImg != nil {
		destImg.Resource = movingResource
		destImg.Move(targetPos)
		destImg.Show()
		destImg.Refresh()
	}

	ca.flyImg.Hide()
	ca.flyImg.Refresh()
}

func (ca *ChessApp) playReverseAnimation(m *chess.Move, targetFen string) {
	fenFunc, err := chess.FEN(targetFen)
	if err != nil {
		return
	}
	oldGame := chess.NewGame(fenFunc)
	oldBoard := oldGame.Position().Board()
	movingPiece := oldBoard.Piece(m.S1())
	capturedPiece := oldBoard.Piece(m.S2())

	movingResource := getPieceResource(movingPiece)

	fromCol := float32(m.S1() % 8)
	fromRow := 7 - float32(m.S1()/8)
	fromPos := fyne.NewPos(ca.boardOffsetX+fromCol*ca.currentSqSize, ca.boardOffsetY+fromRow*ca.currentSqSize)

	toCol := float32(m.S2() % 8)
	toRow := 7 - float32(m.S2()/8)
	toPos := fyne.NewPos(ca.boardOffsetX+toCol*ca.currentSqSize, ca.boardOffsetY+toRow*ca.currentSqSize)

	if img := ca.pieceImgs[m.S2()]; img != nil {
		img.Hide()
		img.Refresh()
	}

	if capturedPiece != chess.NoPiece && capturedPiece.Color() != movingPiece.Color() {
		ca.pieceImgs[m.S2()].Resource = getPieceResource(capturedPiece)
		ca.pieceImgs[m.S2()].Show()
		ca.pieceImgs[m.S2()].Refresh()
	}

	ca.flyImg.Resource = movingResource
	ca.flyImg.Resize(fyne.NewSize(ca.currentSqSize, ca.currentSqSize))
	ca.flyImg.Move(toPos)
	ca.flyImg.Show()
	ca.flyImg.Refresh()

	done := make(chan struct{})
	anim := canvas.NewPositionAnimation(toPos, fromPos, 200*time.Millisecond, func(p fyne.Position) {
		ca.flyImg.Move(p)
		ca.flyImg.Refresh()
	})
	anim.Curve = fyne.AnimationLinear
	anim.Start()

	go func() {
		time.Sleep(210 * time.Millisecond)
		close(done)
	}()
	<-done

	// Pre-set source image before hiding flyImg to prevent flash
	srcImg := ca.pieceImgs[m.S1()]
	if srcImg != nil {
		srcImg.Resource = movingResource
		srcImg.Move(fromPos)
		srcImg.Show()
		srcImg.Refresh()
	}

	ca.flyImg.Hide()
	ca.flyImg.Refresh()
}

func (ca *ChessApp) handleReverse() {
	ca.mu.Lock()
	if !ca.running.Load() || len(ca.history) == 0 {
		ca.mu.Unlock()
		return
	}

	var snapsToReverse []GameSnapshot
	for len(ca.history) > 0 {
		snap := ca.history[len(ca.history)-1]
		ca.history = ca.history[:len(ca.history)-1]
		snapsToReverse = append(snapsToReverse, snap)

		fenFunc, _ := chess.FEN(snap.fen)
		g := chess.NewGame(fenFunc)
		isWhiteTurn := g.Position().Turn() == chess.White

		// Stop popping if we reached meatbag's turn lol
		if (isWhiteTurn && ca.lastWIsHuman) || (!isWhiteTurn && ca.lastBIsHuman) {
			break
		}
	}
	ca.mu.Unlock()

	ca.stopGame()
	time.Sleep(100 * time.Millisecond)

	// Clear all overlays immediately: selection, help hints, assistant threats
	ca.mu.Lock()
	ca.clearHighlight()
	ca.selectedSq = chess.NoSquare
	ca.helpActive = false
	ca.assistantThreats = nil
	ca.assistantKingCheck = false
	ca.safeHints = nil
	ca.riskyHints = nil
	if ca.warnContainer != nil {
		ca.warnContainer.Hide()
	}
	if ca.boardContainer != nil {
		ca.boardContainer.Refresh()
	}
	ca.mu.Unlock()

	// Play animations and restore board state without holding the lock
	for _, snap := range snapsToReverse {
		if snap.lastMove != nil && ca.animate.Load() {
			ca.playReverseAnimation(snap.lastMove, snap.fen)
		}
		ca.mu.Lock()
		fenFunc, _ := chess.FEN(snap.fen)
		ca.game = chess.NewGame(fenFunc)
		ca.mu.Unlock()
		ca.refreshBoard()
	}

	ca.mu.Lock()
	ca.running.Store(true)
	ca.stopCh = make(chan struct{})
	ca.statusLabel.SetText("")
	ca.mu.Unlock()

	ca.showLoadingOverlay("Initializing Stockfish Engine...")

	// spawn new engine processes lol
	go func() {
		var wEng, bEng, aEng *StockfishEngine
		if !ca.lastWIsHuman {
			wEng, _ = NewEngine("stockfish", ca.lastWSkill, threadsPerEngine, ca.getMovetime(ca.lastWSkill), ca.getDepth(ca.lastWSkill), func(l string) { ca.whiteLog.appendLine(l) })
		}
		if !ca.lastBIsHuman {
			bEng, _ = NewEngine("stockfish", ca.lastBSkill, threadsPerEngine, ca.getMovetime(ca.lastBSkill), ca.getDepth(ca.lastBSkill), func(l string) { ca.blackLog.appendLine(l) })
		}
		if ca.lastWIsHuman || ca.lastBIsHuman {
			aEng, _ = NewEngine("stockfish", 5, 2, 500, 15, func(l string) {})
		}

		ca.mu.Lock()
		ca.whiteEngine = wEng
		ca.blackEngine = bEng
		ca.assistantEngine = aEng
		ca.mu.Unlock()

		ca.hideLoadingOverlay()

		if aEng != nil {
			go ca.assistantLoop()
		}
		ca.gameLoop()
	}()
}

func (ca *ChessApp) getMovetime(skill int) int {
	switch skill {
	case 1:
		return 200
	case 2:
		return 500
	case 3:
		return 1000
	case 4:
		return 1500
	case 5:
		return 2000
	default:
		return 500
	}
}

func (ca *ChessApp) getDepth(skill int) int {
	switch skill {
	case 1:
		return 5
	case 2:
		return 10
	case 3:
		return 15
	case 4:
		return 20
	case 5:
		return 25
	default:
		return 10
	}
}

func (ca *ChessApp) showNewGameWindow() {
	w := ca.fyneApp.NewWindow("Configure Battle")
	w.Resize(fyne.NewSize(400, 350))
	w.CenterOnScreen()
	w.SetFixedSize(true)

	whiteSkill := widget.NewEntry()
	whiteSkill.SetText(fmt.Sprintf("%d", ca.lastWSkill))
	if whiteSkill.Text == "0" {
		whiteSkill.SetText("2")
	}
	wType := widget.NewSelect([]string{"Stockfish", "Human"}, func(s string) {
		if s == "Human" {
			whiteSkill.Disable()
		} else {
			whiteSkill.Enable()
		}
	})
	if ca.lastWIsHuman {
		wType.SetSelected("Human")
	} else {
		wType.SetSelected("Stockfish")
	}

	blackSkill := widget.NewEntry()
	blackSkill.SetText(fmt.Sprintf("%d", ca.lastBSkill))
	if blackSkill.Text == "0" {
		blackSkill.SetText("2")
	}
	bType := widget.NewSelect([]string{"Stockfish", "Human"}, func(s string) {
		if s == "Human" {
			blackSkill.Disable()
		} else {
			blackSkill.Enable()
		}
	})
	if ca.lastBIsHuman {
		bType.SetSelected("Human")
	} else {
		bType.SetSelected("Stockfish")
	}

	speedrunCheck := widget.NewCheck("", nil)
	speedrunCheck.Checked = ca.speedrun.Load()

	animCheck := widget.NewCheck("", nil)
	animCheck.Checked = ca.animate.Load()

	autoLoopCheck := widget.NewCheck("", nil)
	autoLoopCheck.Checked = ca.autoLoop.Load()

	sysInfo := widget.NewLabel(fmt.Sprintf("%d cores — %d threads/engine", totalCores, threadsPerEngine))

	form := widget.NewForm(
		widget.NewFormItem("White", wType),
		widget.NewFormItem("White Difficulty (1-5)", whiteSkill),
		widget.NewFormItem("Black", bType),
		widget.NewFormItem("Black Difficulty (1-5)", blackSkill),
		widget.NewFormItem("Speedrun Mode", speedrunCheck),
		widget.NewFormItem("Move Animations", animCheck),
		widget.NewFormItem("Auto-Loop Games", autoLoopCheck),
		widget.NewFormItem("System", sysInfo),
	)

	errLabel := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	startBtn := widget.NewButton("Start Game", func() {
		if wType.Selected == "Human" && bType.Selected == "Human" {
			errLabel.SetText("Error: Both can't be Human!")
			return
		}

		wSkill := ca.parseSkillInput(whiteSkill.Text)
		bSkill := ca.parseSkillInput(blackSkill.Text)

		ca.lastWSkill = wSkill
		ca.lastBSkill = bSkill
		ca.lastWIsHuman = wType.Selected == "Human"
		ca.lastBIsHuman = bType.Selected == "Human"

		ca.speedrun.Store(speedrunCheck.Checked)
		ca.animate.Store(animCheck.Checked)
		ca.autoLoop.Store(autoLoopCheck.Checked)
		w.Hide()
		w.Close()
		go ca.startGame(wSkill, bSkill, ca.lastWIsHuman, ca.lastBIsHuman)
	})

	cancelBtn := widget.NewButton("Cancel", func() {
		w.Hide()
		w.Close()
	})

	buttons := container.NewHBox(layout.NewSpacer(), cancelBtn, startBtn)
	content := container.NewVBox(form, errLabel, layout.NewSpacer(), buttons)
	w.SetContent(container.NewPadded(content))
	w.Show()
}

func (ca *ChessApp) parseSkillInput(in string) int {
	switch strings.TrimSpace(in) {
	case "1":
		return 1
	case "3":
		return 3
	case "4":
		return 4
	case "5":
		return 5
	default:
		return 2
	}
}

type Difficulty struct {
	Skill    int
	Movetime int
	Depth    int
}

func (ca *ChessApp) getDifficulty(level int) Difficulty {
	switch level {
	case 1:
		return Difficulty{Skill: 0, Movetime: 50, Depth: 1}
	case 2:
		return Difficulty{Skill: 5, Movetime: 100, Depth: 3}
	case 3:
		return Difficulty{Skill: 10, Movetime: 300, Depth: 5}
	case 4:
		return Difficulty{Skill: 15, Movetime: 800, Depth: 10}
	case 5:
		return Difficulty{Skill: 20, Movetime: 2000, Depth: 20}
	default:
		return Difficulty{Skill: 5, Movetime: 100, Depth: 3}
	}
}

func (ca *ChessApp) startGame(whiteSkillLevel, blackSkillLevel int, wIsHuman, bIsHuman bool) {
	ca.mu.Lock()
	if ca.running.Load() {
		ca.mu.Unlock()
		return
	}
	ca.game = chess.NewGame()
	ca.stopCh = make(chan struct{})
	ca.running.Store(true)
	ca.whiteLog.clear()
	ca.blackLog.clear()
	ca.mu.Unlock()

	ca.refreshBoard()
	ca.turnWidget.isWhite = false
	ca.turnWidget.setTurn(true)
	ca.statusLabel.SetText("Starting engines...")
	ca.copyLogBtn.Hide()
	ca.history = nil

	if wIsHuman || bIsHuman {
		ca.cheatsToggle.Show()
	} else {
		ca.cheatsToggle.Hide()
		ca.cheatsPanel.Hide()
	}

	stockfishPath := "stockfish"
	wDiff := ca.getDifficulty(whiteSkillLevel)
	bDiff := ca.getDifficulty(blackSkillLevel)

	var whiteEng, blackEng *StockfishEngine
	var err error

	if !wIsHuman {
		whiteEng, err = NewEngine(stockfishPath, wDiff.Skill, threadsPerEngine, wDiff.Movetime, wDiff.Depth, func(line string) {
			ca.whiteLog.appendLine(line)
		})
		if err != nil {
			ca.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
			ca.running.Store(false)
			return
		}
	} else {
		ca.whiteLog.appendLine("HUMAN PLAYER")
	}

	if !bIsHuman {
		blackEng, err = NewEngine(stockfishPath, bDiff.Skill, threadsPerEngine, bDiff.Movetime, bDiff.Depth, func(line string) {
			ca.blackLog.appendLine(line)
		})
		if err != nil {
			if whiteEng != nil {
				whiteEng.Close()
			}
			ca.statusLabel.SetText(fmt.Sprintf("Error: %v", err))
			ca.running.Store(false)
			return
		}
	} else {
		ca.blackLog.appendLine("HUMAN PLAYER")
	}

	var aEng *StockfishEngine
	if wIsHuman || bIsHuman {
		aEng, err = NewEngine(stockfishPath, 5, 2, 500, 15, func(l string) {})
		if err != nil {
			ca.statusLabel.SetText(fmt.Sprintf("Assistant Error: %v", err))
		}
	}
	// why the hell is this function so long, clean code my ass pls

	ca.mu.Lock()
	ca.whiteEngine = whiteEng
	ca.blackEngine = blackEng
	ca.assistantEngine = aEng
	ca.lastWIsHuman = wIsHuman
	ca.lastBIsHuman = bIsHuman
	ca.mu.Unlock()

	if aEng != nil {
		go ca.assistantLoop()
	}

	ca.statusLabel.SetText("Game in progress")
	go ca.gameLoop()
}

func (ca *ChessApp) gameLoop() {
	defer func() {
		ca.mu.Lock()
		if ca.whiteEngine != nil {
			ca.whiteEngine.Close()
			ca.whiteEngine = nil
		}
		if ca.blackEngine != nil {
			ca.blackEngine.Close()
			ca.blackEngine = nil
		}
		if ca.assistantEngine != nil {
			ca.assistantEngine.Close()
			ca.assistantEngine = nil
		}
		ca.running.Store(false)
		ca.mu.Unlock()
	}()

	for {
		select {
		case <-ca.stopCh:
			ca.statusLabel.SetText("Game stopped")
			return
		default:
		}

		if ca.game.Outcome() != chess.NoOutcome {
			ca.handleGameOver()
			return
		}

		pos := ca.game.Position()
		fen := pos.String()
		isWhiteTurn := pos.Turn() == chess.White

		ca.turnWidget.setTurn(isWhiteTurn)

		turnStr := "WHITE"
		if !isWhiteTurn {
			turnStr = "BLACK"
		}
		ca.moveLabel.SetText(fmt.Sprintf("Move %d — %s to play", len(ca.game.Moves())/2+1, turnStr))

		var m *chess.Move

		if (isWhiteTurn && ca.lastWIsHuman) || (!isWhiteTurn && ca.lastBIsHuman) {
			select {
			case m = <-ca.humanMoveCh:
			case <-ca.stopCh:
				ca.statusLabel.SetText("Game stopped")
				return
			}

			if m == nil { // pass logic
				ca.mu.Lock()
				parts := strings.Fields(fen)
				if len(parts) >= 2 {
					if parts[1] == "w" {
						parts[1] = "b"
					} else {
						parts[1] = "w"
					}
				}
				if len(parts) >= 4 {
					parts[3] = "-"
				}
				newFen := strings.Join(parts, " ")
				fenFunc, _ := chess.FEN(newFen)
				ca.history = append(ca.history, GameSnapshot{fen: fen, lastMove: nil})
				ca.game = chess.NewGame(fenFunc)
				ca.refreshBoard()
				ca.mu.Unlock()
				continue
			}
		} else {
			var engine *StockfishEngine
			if isWhiteTurn {
				engine = ca.whiteEngine
			} else {
				engine = ca.blackEngine
			}

			moveStr, err := engine.GetBestMove(fen)
			if err != nil {
				errMsg := fmt.Sprintf("Rock brain died before answering: %v. (Invalid FEN?)", err)
				ca.statusLabel.SetText(errMsg)
				ca.copyLogBtn.Show()
				return
			}

			m, err = ca.parseUCIMove(moveStr)
			if err != nil {
				ca.statusLabel.SetText(fmt.Sprintf("Move error: %s — %v", moveStr, err))
				return
			}
		}

		if ca.animate.Load() {
			ca.animateMove(m.S1(), m.S2())
		}

		ca.mu.Lock()
		ca.helpActive = false
		ca.assistantThreats = nil
		ca.assistantKingCheck = false
		if ca.boardContainer != nil {
			ca.boardContainer.Refresh()
		}
		ca.history = append(ca.history, GameSnapshot{fen: fen, lastMove: m})
		if err := ca.game.Move(m); err != nil {
			ca.statusLabel.SetText(fmt.Sprintf("Move error: %v", err))
			ca.mu.Unlock()
			return
		}
		ca.refreshBoard()
		ca.mu.Unlock()

		if !ca.speedrun.Load() && ((!isWhiteTurn && !ca.lastBIsHuman) || (isWhiteTurn && !ca.lastWIsHuman)) {
			delay := time.Duration(200+rand.Intn(600)) * time.Millisecond
			select {
			case <-time.After(delay):
			case <-ca.stopCh:
				ca.statusLabel.SetText("Game stopped")
				return
			}
		}
	}
}

func (ca *ChessApp) showPromotionPopup(promoMoves []*chess.Move) {
	var popup *widget.PopUp

	vbox := container.NewVBox(widget.NewLabelWithStyle("Promote Pawn", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))

	for _, m := range promoMoves {
		move := m // capture
		var name string
		switch move.Promo() {
		case chess.Queen:
			name = "Queen"
		case chess.Rook:
			name = "Rook"
		case chess.Bishop:
			name = "Bishop"
		case chess.Knight:
			name = "Knight"
		default:
			name = "Piece"
		}

		btn := widget.NewButton(name, func() {
			popup.Hide()
			select {
			case ca.humanMoveCh <- move:
			default:
			}
		})
		vbox.Add(btn)
	}

	popup = widget.NewModalPopUp(container.NewPadded(vbox), ca.window.Canvas())
	popup.Show()
}

func (ca *ChessApp) showMoveHints(sq chess.Square) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	validMoves := ca.game.ValidMoves()
	safeHints := make(map[chess.Square]bool)
	riskyHints := make(map[chess.Square]bool)

	for _, m := range validMoves {
		if m.S1() == sq {
			clone := ca.game.Clone()
			_ = clone.Move(m)

			oppMoves := clone.ValidMoves()
			isRisky := false
			for _, oppM := range oppMoves {
				if oppM.S2() == m.S2() && oppM.HasTag(chess.Capture) {
					isRisky = true
					break
				}
			}

			if isRisky {
				riskyHints[m.S2()] = true
			} else {
				safeHints[m.S2()] = true
			}
		}
	}

	ca.safeHints = safeHints
	ca.riskyHints = riskyHints
	if ca.boardContainer != nil {
		ca.boardContainer.Refresh()
	}
}

func (ca *ChessApp) parseUCIMove(uciMove string) (*chess.Move, error) {
	validMoves := ca.game.ValidMoves()
	for _, m := range validMoves {
		if strings.EqualFold(m.String(), uciMove) {
			return m, nil
		}
	}
	return nil, fmt.Errorf("legal move not found '%s'", uciMove)
}

func (ca *ChessApp) handleGameOver() {
	outcome := ca.game.Outcome()
	method := ca.game.Method()

	var resultText string
	switch outcome {
	case chess.WhiteWon:
		resultText = "White wins!"
	case chess.BlackWon:
		resultText = "Black wins!"
	case chess.Draw:
		resultText = "Draw!"
	default:
		resultText = "Game over"
	}

	methodStr := ""
	switch method {
	case chess.Checkmate:
		methodStr = "by checkmate"
	case chess.Stalemate:
		methodStr = "by stalemate"
	case chess.InsufficientMaterial:
		methodStr = "insufficient material"
	case chess.ThreefoldRepetition:
		methodStr = "threefold repetition"
	case chess.FiftyMoveRule:
		methodStr = "fifty-move rule"
	}

	if methodStr != "" {
		resultText = fmt.Sprintf("%s (%s)", resultText, methodStr)
	}

	ca.statusLabel.SetText(resultText)
	totalMoves := len(ca.game.Moves())
	ca.moveLabel.SetText(fmt.Sprintf("Game ended after %d moves", totalMoves))

	if ca.autoLoop.Load() {
		ca.statusLabel.SetText(fmt.Sprintf("%s (Auto-restarting in 3s...)", resultText))
		go func() {
			select {
			case <-time.After(3 * time.Second):
				ca.startGame(ca.lastWSkill, ca.lastBSkill, ca.lastWIsHuman, ca.lastBIsHuman)
			case <-ca.stopCh:
			}
		}()
	}
}

func (ca *ChessApp) stopGame() {
	if ca.running.Load() {
		close(ca.stopCh)
		time.Sleep(100 * time.Millisecond)
	}
}

func (ca *ChessApp) showLoadingOverlay(text string) {
	if ca.loadingOverlayContainer != nil {
		ca.loadingOverlayContainer.Show()
		ca.loadingOverlayContainer.Refresh()
	}
}
func (ca *ChessApp) hideLoadingOverlay() {
	if ca.loadingOverlayContainer != nil {
		ca.loadingOverlayContainer.Hide()
		ca.loadingOverlayContainer.Refresh()
	}
}

func (ca *ChessApp) toggleAutoHelp() {
	if ca.autoHelpActive.Load() {
		ca.autoHelpActive.Store(false)
		ca.helpBtn.Importance = widget.MediumImportance
		ca.helpBtn.Refresh()
		ca.mu.Lock()
		ca.helpActive = false
		if ca.boardContainer != nil {
			ca.boardContainer.Refresh()
		}
		ca.mu.Unlock()
	} else {
		ca.autoHelpActive.Store(true)
		go ca.autoHelpLoop()
		go func() {
			for ca.autoHelpActive.Load() {
				if ca.autoHelpBtnFlash.Load() {
					ca.helpBtn.Importance = widget.WarningImportance
				} else {
					ca.helpBtn.Importance = widget.MediumImportance
				}
				ca.helpBtn.Refresh()
				ca.autoHelpBtnFlash.Store(!ca.autoHelpBtnFlash.Load())
				time.Sleep(500 * time.Millisecond)
			}
			ca.helpBtn.Importance = widget.MediumImportance
			ca.helpBtn.Refresh()
		}()
	}
}

func (ca *ChessApp) showHelpVisualization() {
	ca.mu.Lock()
	if !ca.running.Load() || ca.helpBtn.Text == "Thinking..." {
		ca.mu.Unlock()
		return
	}
	ca.helpBtn.SetText("Thinking...")
	ca.helpBtn.Disable()
	fen := ca.game.Position().String()
	ca.mu.Unlock()

	go func() {
		defer func() {
			ca.helpBtn.SetText("Help")
			ca.helpBtn.Enable()
		}()

		helpEng, err := NewEngine("stockfish", 5, 4, 5000, 24, func(l string) {})
		if err != nil {
			return
		}
		defer helpEng.Close()

		moveStr, err := helpEng.GetBestMove(fen)
		if err == nil && moveStr != "(none)" {
			m, parseErr := ca.parseUCIMove(moveStr)
			if parseErr == nil && m != nil {
				ca.mu.Lock()
				ca.helpFromSq = m.S1()
				ca.helpToSq = m.S2()
				ca.helpActive = true
				if ca.boardContainer != nil {
					ca.boardContainer.Refresh()
				}
				ca.mu.Unlock()

				go func() {
					time.Sleep(3 * time.Second)
					ca.mu.Lock()
					if !ca.autoHelpActive.Load() {
						ca.helpActive = false
						if ca.boardContainer != nil {
							ca.boardContainer.Refresh()
						}
					}
					ca.mu.Unlock()
				}()
			}
		}
	}()
}

func (ca *ChessApp) autoHelpLoop() {
	helpEng, err := NewEngine("stockfish", 5, 4, 1000, 15, func(l string) {})
	if err != nil {
		ca.autoHelpActive.Store(false)
		return
	}
	defer helpEng.Close()

	var lastFen string

	for ca.running.Load() && ca.autoHelpActive.Load() {
		ca.mu.Lock()
		isWhiteTurn := ca.game.Position().Turn() == chess.White
		humanTurn := (isWhiteTurn && ca.lastWIsHuman) || (!isWhiteTurn && ca.lastBIsHuman)
		fen := ca.game.Position().String()
		ca.mu.Unlock()

		if humanTurn {
			if fen != lastFen {
				moveStr, err := helpEng.GetBestMove(fen)
				if err == nil && moveStr != "(none)" {
					m, parseErr := ca.parseUCIMove(moveStr)
					if parseErr == nil && m != nil {
						ca.mu.Lock()
						ca.helpFromSq = m.S1()
						ca.helpToSq = m.S2()
						ca.helpActive = true
						if ca.boardContainer != nil {
							ca.boardContainer.Refresh()
						}
						ca.mu.Unlock()
						lastFen = fen
					}
				}
			}
		} else {
			ca.mu.Lock()
			if ca.helpActive {
				ca.helpActive = false
				if ca.boardContainer != nil {
					ca.boardContainer.Refresh()
				}
			}
			ca.mu.Unlock()
			lastFen = "" // reset so it calculates immediately when it's our turn again
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// why's this code so long
