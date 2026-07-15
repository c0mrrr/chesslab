# 🧪 ChessLab 🧪

**ChessLab** is a feature-rich, interactive chess analysis and sandbox desktop application built in Go using the **Fyne** GUI toolkit. Designed for **Linux** platforms, ChessLab lets you play against stockfish engines, watch engines battle each other, or tweak the game with a few "fun" options.

---

### Navigation
*   [How to Run](#-how-to-run)
*   [Build and Compile](#️-build-and-compile)
*   [Key Features](#-key-features)
*   [Requirements](#️-platform--system-requirements)
*   [Cheats](#-cheats-section)

---

## 📦 How to Run

To run ChessLab by downloading the latest pre-compiled binary:

1. Download the latest binary from the **Releases** section on GitHub.
2. Open your terminal and make the binary executable:
   ```bash
   chmod +x chesslab
   ```
3. Run the executable:
   ```bash
   ./chesslab
   ```

*Note: You still need to have Stockfish installed on your system (see [Build and Compile](#️-build-and-compile) for dependency commands).*

---

## ⚠️ Platform & System Requirements

ChessLab is designed to run on Linux systems. To run ChessLab, you need to have **Stockfish** installed and available in your system's `PATH`.

---

## 🚀 Key Features

*   **Engine-vs-Engine Battles**: Watch two Stockfish engines battle each other in real time.
*   **Human vs. Engine**: Test your skills against Stockfish with adjustable thread settings.
*   **Live Engine Terminals**: View the stdout logs of the White and Black engines directly inside the UI.
*   **Chess Assistant**: Turn on real-time hints including:
    *   **Safe Hints**: Highlights squares that are safe to move to.
    *   **Risky Hints**: Highlights squares that are under threat but still playable.
    *   **Threat Highlights & Warnings**: Visual indicators showing squares under attack.
*   **Auto-Loop Games**: Continuously start new engine battles automatically.

---

## 😈 Cheats Section

Click the **Cheats** button at the bottom of the board to toggle the cheat control panel.

| Cheat | Description |
| :--- | :--- |
| **Add Piece** | Click "Add Piece" to open a selector. Choose a Queen, Rook, Bishop, Knight, or Pawn of either color, then drag and drop it onto any square on the board. <br>⚠️ *Warning: If you add too many pieces at once, the Stockfish chess engine will likely get confused, leading to undefined behaviors or the game ending.* |
| **Undo** | Reverts the game state back to your last turn, undoing the engine's last move as well. |
| **Pass** | Skips your turn, passing the move action back to the opponent engine. |
| **Manual/Auto Help** | Get instant visual help, safe/risky square overlays, and threat indicators on demand. |

---

## 🛠️ Build and Compile

### Prerequisites

*   **Go Version**: `1.26.4` or higher.
*   **C Compiler & X11/OpenGL development libraries** (required by the Fyne GUI toolkit).

Follow the steps below to install all dependencies, compile, and run ChessLab:

#### 1. Install Dependencies & Stockfish

##### On Arch Linux

```bash
sudo pacman -Syu
sudo pacman -S go stockfish gcc pkgconf xorgproto libx11 libxrandr libxinerama libxcursor libxi mesa
```

> [NOTE]
> If installing `stockfish` through `pacman` does not work, you can install it from the AUR:
> ```bash
> yay -S stockfish
> ```

##### On Debian / Ubuntu

```bash
sudo apt update
sudo apt install -y golang stockfish gcc pkg-config libgl1-mesa-dev xorg-dev
```

---

#### 2. Compile and Run ChessLab

Navigate to the project root directory and build the binary, then execute it:

```bash
go build -o chesslab
./chesslab
```

Enjoy lol