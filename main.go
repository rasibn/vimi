package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "vimi",
		Usage: "Fuzzy-find and open files in Neovim",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "preview",
				Aliases: []string{"p"},
				Usage:   "Enable preview",
			},
			&cli.BoolFlag{
				Name:    "files",
				Aliases: []string{"f"},
				Usage:   "Search directories instead of files",
			},
			&cli.IntFlag{
				Name:    "depth",
				Aliases: []string{"d"},
				Usage:   "Search depth (1 = non-recursive, 0 = unlimited)",
				Value:   0,
			},
		},
		Action: func(c *cli.Context) error {
			usePreview := c.Bool("preview")
			searchDirs := c.Bool("files")
			depth := c.Int("depth")

			// Collect CLI args first; only fall back to env if none provided
			searchPaths := c.Args().Slice()
			if len(searchPaths) == 0 {
				for _, env := range []string{"PROJECT_DIR", "WORK_DIR", "ASSET_DIR"} {
					if val := os.Getenv(env); val != "" {
						searchPaths = append(searchPaths, val)
					}
				}
			}
			if len(searchPaths) == 0 {
				searchPaths = []string{"."}
			}

			// Ensure fd is present
			if !commandExists("fd") {
				return fmt.Errorf("'fd' is required but not found in PATH")
			}

			if searchDirs {
				selectedDir, err := findDirectory(searchPaths, usePreview, depth)
				if err != nil {
					return fmt.Errorf("failed to find directory: %w", err)
				}
				if selectedDir == "" {
					return nil
				}
				fmt.Println(selectedDir)
				return nil
			}

			selectedFile, err := findFile(searchPaths, usePreview, depth)
			if err != nil {
				return fmt.Errorf("failed to find file: %w", err)
			}
			if selectedFile == "" {
				fmt.Fprintln(os.Stderr, "No file selected.")
				return nil
			}

			return openInNvim(selectedFile)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func findFile(paths []string, usePreview bool, depth int) (string, error) {
	return findItems(paths, usePreview, "f", depth)
}

func findDirectory(paths []string, usePreview bool, depth int) (string, error) {
	return findItems(paths, usePreview, "d", depth)
}

func findItems(paths []string, usePreview bool, itemType string, depth int) (string, error) {
	fzfArgs := []string{"fzf"}
	if usePreview {
		if itemType == "f" && commandExists("bat") {
			fzfArgs = append(fzfArgs,
				"--ansi",
				"--preview-window=right:45%",
				"--preview=bat --color=always --style=header,grid --line-range :300 {}",
			)
		} else if itemType == "d" {
			fzfArgs = append(fzfArgs,
				"--ansi",
				"--preview-window=right:45%",
				"--preview=ls -la {}",
			)
		}
	}

	// Build fd args: options first, then search paths via --search-path, then the match-all pattern "."
	args := []string{
		"--type", itemType, // "f" or "d"
		"--hidden",
		"--follow",
		"--exclude", ".git",
	}
	if depth > 0 {
		args = append(args, "--max-depth", fmt.Sprint(depth))
	}
	for _, p := range paths {
		args = append(args, "--search-path", p)
	}
	// Explicit pattern so fd doesn't treat the first path as the pattern.
	args = append(args, ".")

	findCmd := exec.Command("fd", args...)
	// Capture stderr too so errors are informative
	outBytes, err := findCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("fd search failed: %v\n%s", err, string(outBytes))
	}

	fzfCmd := exec.Command(fzfArgs[0], fzfArgs[1:]...)
	fzfCmd.Stdin = strings.NewReader(string(outBytes))
	out, err := fzfCmd.Output()
	if err != nil {
		// Treat cancel/no-selection as empty result
		if exitErr, ok := err.(*exec.ExitError); ok && (exitErr.ExitCode() == 130 || exitErr.ExitCode() == 1) {
			return "", nil
		}
		return "", fmt.Errorf("fzf failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func openInNvim(file string) error {
	cmd := exec.Command("nvim", file)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
