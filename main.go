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
		},
		Action: func(c *cli.Context) error {
			usePreview := c.Bool("preview")
			searchDirs := c.Bool("files")

			// Collect args and fallback to env vars
			searchPaths := c.Args().Slice()
			envVars := []string{"PROJECT_DIR", "WORK_DIR", "ASSET_DIR"}
			for _, env := range envVars {
				if val := os.Getenv(env); val != "" {
					searchPaths = append(searchPaths, val)
				}
			}
			if len(searchPaths) == 0 {
				searchPaths = append(searchPaths, ".")
			}

			if searchDirs {
				selectedDir, err := findDirectory(searchPaths, usePreview)
				if err != nil {
					return fmt.Errorf("failed to find directory: %w", err)
				}
				if selectedDir == "" {
					return nil
				}
				fmt.Println(selectedDir)
				return nil
			}

			selectedFile, err := findFile(searchPaths, usePreview)
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

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func findFile(paths []string, usePreview bool) (string, error) {
	return findItems(paths, usePreview, "f")
}

func findDirectory(paths []string, usePreview bool) (string, error) {
	return findItems(paths, usePreview, "d")
}

func findItems(paths []string, usePreview bool, itemType string) (string, error) {
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

	var findCmd *exec.Cmd
	if commandExists("fd") {
		args := []string{".", "--type", itemType, "--hidden", "--follow", "--exclude", ".git"}
		args = append(args, paths...)
		findCmd = exec.Command("fd", args...)
	} else if commandExists("find") {
		findCmd = exec.Command("find", append(paths, "-type", itemType)...)
	} else {
		return "", fmt.Errorf("neither fd nor find is installed")
	}

	findOut, err := findCmd.Output()
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	fzfCmd := exec.Command(fzfArgs[0], fzfArgs[1:]...)
	fzfCmd.Stdin = strings.NewReader(string(findOut))
	out, err := fzfCmd.Output()
	if err != nil {
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
