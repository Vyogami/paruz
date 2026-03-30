# Paruz

Paruz is a fast, Terminal User Interface (TUI) for Arch Linux that acts as a visual wrapper for the `paru` AUR helper.

It allows you to:
- **Search:** Quickly search your local pacman sync databases and AUR (via paru -Qs / paru -Ss).
- **Inspect:** View rich package information (`paru -Si`).
- **Install:** Trigger installations directly from the UI, with standard output and prompts embedded into the TUI.
- **Mirrors:** Update your pacman mirrorlist using `rate-mirrors`.

## Installation

Ensure you have Go, `paru`, and (optionally) `rate-mirrors` installed on your system.

```bash
git clone https://github.com/vyogami/paruz
cd paruz
go build -o paruz ./cmd/paruz
```

## Usage

Run the compiled binary:
```bash
./paruz
```

### Keybindings
- **`/`**: Start filtering/searching packages
- **`Enter`**: Install the selected package
- **`u`**: Update mirrorlist (requires `rate-mirrors` and `sudo`)
- **`Esc`**: Exit terminal execution or clear search
- **`q`**: Quit the application

## Technical Details

Paruz is built in Go using the incredible [Charmbracelet](https://charm.sh/) ecosystem (`bubbletea`, `bubbles`, `lipgloss`). It integrates with `creack/pty` to provide a pseudo-terminal specifically for rendering `paru` output inside the TUI viewport while correctly forwarding user inputs (like typing `y` for `PKGBUILD` reviews).
