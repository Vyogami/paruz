# Paruz

![screenshot](./screenshot.png)

Paruz is a fast, Terminal User Interface (TUI) for Arch Linux that acts as a visual wrapper for the `paru` AUR helper.

It allows you to:
- **Search:** Quickly search your local pacman sync databases and AUR (via paru -Qs / paru -Ss).
- **Inspect:** View rich package information (`paru -Si`).
- **Install:** Trigger installations directly from the UI, with standard output and prompts embedded into the TUI.
- **Mirrors:** Update your pacman mirrorlist using `rate-mirrors`.
- **Self-update:** Check for and install new paruz releases from within the app (press `U`).

## Installation

### Arch Linux (AUR)

Paruz is available on the [AUR](https://aur.archlinux.org/packages/paruz). Install it with any AUR helper:

```bash
# Pre-built binary (recommended)
paru -S paruz

# Build the latest commit from source
paru -S paruz-git
```

### Quick Install

Install the latest pre-built binary to `/usr/local/bin`:

```bash
curl -sL https://raw.githubusercontent.com/vyogami/paruz/main/install.sh | sudo sh
```

### Build from Source

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

| Key | Action |
|-----|--------|
| `/` or `s` | Open the search bar |
| `j` / `↓`, `k` / `↑` | Move through the package list (`↑`/`k` at the top jumps back to the search bar) |
| `Enter` | Install the selected package |
| `u` | Update the pacman mirrorlist (requires `rate-mirrors` and `sudo`) |
| `r` | Refresh the package cache |
| `,` | Open the settings menu |
| `U` (Shift+U) | Check for a paruz update (and install it in-app) |
| `q` | Quit the application |

## Configuration

Paruz stores its configuration in `~/.config/paruz/config.toml`.

### `config.toml`
```toml
aur_helper = "paru"      # paru or yay
mirror_helper = "rate-mirrors" # rate-mirrors or reflector
theme = "ayu-dark"       # default theme name
```

### Custom Themes
You can add your own themes by creating `~/.config/paruz/themes.toml`:

```toml
[themes.my-cool-theme]
title_bg = "#ff00ff"
title_fg = "#ffffff"
border = "#6272a4"
info_title = "#ff79c6"
info_key = "#8be9fd"
error = "#ff5555"
status_bar = "#f8f8f2"
```

After adding a theme to `themes.toml`, it will appear in the settings menu (press `,`).

## Technical Details

Paruz is built in Go using the incredible [Charmbracelet](https://charm.sh/) ecosystem (`bubbletea`, `bubbles`, `lipgloss`). It integrates with `creack/pty` to provide a pseudo-terminal specifically for rendering `paru` output inside the TUI viewport.

## Inspiration

This project is inspired by the original [\<github\> paruz \[deleted\]](https://github.com/joehillen/paruz), [\<AUR\> paruz-git](https://aur.archlinux.org/packages/paruz-git) by Joe Hillen. However, that project has been abandoned and its source code is no longer available on GitHub. 

While the original `paruz` was a simple script that used `fzf` to provide a selection interface for `paru`, this version of **Paruz** is a complete, standalone TUI built from the ground up using **Bubble Tea**. It provides a richer, more interactive experience.
